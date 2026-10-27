package services

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/difyz9/edge-tts-go/pkg/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/net/proxy"
)

// 以下常量对齐官方 Python edge_tts 库（constants.py）：端点、TrustedClientToken、
// Chromium 版本与 GEC 版本。微软已轮换 GEC 校验，必须使用 v2 令牌（见 edgetts_gec.go）
// 与 "2-xxx" 版本前缀，否则握手被拒（websocket: bad handshake）。
const (
	edgeTTSTrustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	// edgeTTSBaseURL 为官方 readaloud WebSocket 端点（旧 api.msedgeservices.com 端点
	// 搭配 "1-xxx" GEC 已失效）。
	edgeTTSBaseURL         = "speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	edgeTTSVoiceListURL    = "https://speech.platform.bing.com/consumer/speech/synthesize/readaloud/voices/list?trustedclienttoken=" + edgeTTSTrustedClientToken
	edgeTTSWinEpoch        = 11644473600.0 // Windows 文件时间纪元（1601-01-01 UTC，秒）
	edgeTTSSToNS           = 1e9
	edgeTTSChromiumVersion = "143"
	edgeTTSSecMSGECVersion = "2-143.0.3650.75"
)

// edgeTTSWSSHeaders WebSocket 握手时使用的请求头（对齐微软 Edge TTS 协议）。
var edgeTTSWSSHeaders = map[string]string{
	"Pragma":                 "no-cache",
	"Cache-Control":          "no-cache",
	"Origin":                 "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold",
	"Sec-WebSocket-Protocol": "synthesize",
	"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36" +
		" (KHTML, like Gecko) Chrome/" + edgeTTSChromiumVersion + ".0.0.0 Safari/537.36" +
		" Edg/" + edgeTTSChromiumVersion + ".0.0.0",
	"Accept-Encoding": "gzip, deflate, br",
	"Accept-Language": "en-US,en;q=0.9",
}

// edgeTTSVoiceHeaders 拉取音色列表时的请求头。
var edgeTTSVoiceHeaders = map[string]string{
	"Authority": "speech.platform.bing.com",
	"Sec-CH-UA": "\" Not;A Brand\";v=\"99\", \"Microsoft Edge\";v=\"" + edgeTTSChromiumVersion + "\"," +
		" \"Chromium\";v=\"" + edgeTTSChromiumVersion + "\"",
	"Sec-CH-UA-Mobile": "?0",
	"Accept":           "*/*",
	"Sec-Fetch-Site":   "none",
	"Sec-Fetch-Mode":   "cors",
	"Sec-Fetch-Dest":   "empty",
	"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36" +
		" (KHTML, like Gecko) Chrome/" + edgeTTSChromiumVersion + ".0.0.0 Safari/537.36" +
		" Edg/" + edgeTTSChromiumVersion + ".0.0.0",
	"Accept-Encoding": "gzip, deflate, br",
	"Accept-Language": "en-US,en;q=0.9",
}

// ── 时钟偏移校正（DRM / Sec-MS-GEC 令牌）──────────────────────────────
// 微软用基于本地时钟的 Sec-MS-GEC 令牌做校验；系统时钟偏差会导致 403，
// 收到 403 时依据服务端 Date 头校正本地偏移并重试（与 difyz9 行为一致）。

var (
	edgeTTSClockSkewMu sync.RWMutex
	edgeTTSClockSkew   float64
)

func edgeTTSAdjClockSkew(skew float64) {
	edgeTTSClockSkewMu.Lock()
	defer edgeTTSClockSkewMu.Unlock()
	edgeTTSClockSkew += skew
}

func edgeTTSGetUnixTimestamp() float64 {
	edgeTTSClockSkewMu.RLock()
	defer edgeTTSClockSkewMu.RUnlock()
	return float64(time.Now().UTC().Unix()) + edgeTTSClockSkew
}

// edgeTTSGenerateSecMSGEC 生成 v2 形式的 Sec-MS-GEC 令牌（CS64-SHA256 两段式），
// 严格对齐官方 edge_tts 库，否则微软会拒绝握手。
func edgeTTSGenerateSecMSGEC() string {
	return gecGenerateSecMSGECSpecific(edgeTTSGetUnixTimestamp())
}

func edgeTTSHandle403(resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("no response")
	}
	serverDate := resp.Header.Get("Date")
	if serverDate == "" {
		return fmt.Errorf("no server date in headers")
	}
	t, err := time.Parse(time.RFC1123, serverDate)
	if err != nil {
		return fmt.Errorf("failed to parse server date: %s", serverDate)
	}
	edgeTTSAdjClockSkew(float64(t.UTC().Unix()) - edgeTTSGetUnixTimestamp())
	return nil
}

// ── 代理感知的拨号器 / HTTP Client ───────────────────────────────────

// edgeTTSDialer 返回支持 http/https/socks5 代理的 WebSocket 拨号器。
// proxyAddr 为空时强制直连（不继承环境里的 HTTP_PROXY/HTTPS_PROXY，
// 避免被不可达的系统代理连累导致 websocket: bad handshake）。
func edgeTTSDialer(proxyAddr string, handshakeTimeout time.Duration) *websocket.Dialer {
	d := &websocket.Dialer{
		HandshakeTimeout:  handshakeTimeout,
		EnableCompression: true,
	}
	switch {
	case proxyAddr == "":
		// 显式直连：覆盖默认的 ProxyFromEnvironment。
		d.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	case strings.HasPrefix(proxyAddr, "socks5"), strings.HasPrefix(proxyAddr, "socks4"):
		if u, err := url.Parse(proxyAddr); err == nil {
			var auth *proxy.Auth
			if u.User != nil {
				pw, _ := u.User.Password()
				auth = &proxy.Auth{User: u.User.Username(), Password: pw}
			}
			if sd, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct); err == nil {
				socks := sd
				d.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					if dc, ok := socks.(interface {
						DialContext(ctx context.Context, network, address string) (net.Conn, error)
					}); ok {
						return dc.DialContext(ctx, network, address)
					}
					return socks.Dial(network, address)
				}
				// 设置 NetDialContext 后须关闭 Proxy，让 gorilla 走直连分支再经 socks 出口。
				d.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
			}
		}
	default:
		if u, err := url.Parse(proxyAddr); err == nil {
			d.Proxy = http.ProxyURL(u)
		} else {
			d.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
		}
	}
	return d
}

// edgeTTSHTTPClient 返回支持 socks5/http/https 代理的 HTTP 客户端，用于拉取音色列表。
func edgeTTSHTTPClient(proxyAddr string) *http.Client {
	tr := &http.Transport{}
	switch {
	case proxyAddr == "":
		// 直连
	case strings.HasPrefix(proxyAddr, "socks5"), strings.HasPrefix(proxyAddr, "socks4"):
		if u, err := url.Parse(proxyAddr); err == nil {
			var auth *proxy.Auth
			if u.User != nil {
				pw, _ := u.User.Password()
				auth = &proxy.Auth{User: u.User.Username(), Password: pw}
			}
			if sd, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct); err == nil {
				socks := sd
				tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					if dc, ok := socks.(interface {
						DialContext(ctx context.Context, network, address string) (net.Conn, error)
					}); ok {
						return dc.DialContext(ctx, network, address)
					}
					return socks.Dial(network, address)
				}
			}
		}
	default:
		if u, err := url.Parse(proxyAddr); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

// ── 协议辅助 ────────────────────────────────────────────────────────

func edgeTTSConnectID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

func edgeTTSDateToString() string {
	return time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
}

func edgeTTSWSSURL() string {
	// 官方 readaloud 端点：用 TrustedClientToken 查询参数鉴权（非 Ocp-Apim-Subscription-Key）。
	return "wss://" + edgeTTSBaseURL + "?TrustedClientToken=" + edgeTTSTrustedClientToken +
		"&ConnectionId=" + edgeTTSConnectID() +
		"&Sec-MS-GEC=" + edgeTTSGenerateSecMSGEC() +
		"&Sec-MS-GEC-Version=" + edgeTTSSecMSGECVersion
}

// edgeTTSNormalizeVoice 把 short name（zh-CN-XiaoxiaoNeural）转成微软 SSML 需要的
// 长格式名（Microsoft Server Speech Text to Speech Voice (zh-CN, XiaoxiaoNeural)）。
func edgeTTSNormalizeVoice(voice string) string {
	re := regexp.MustCompile(`^([a-z]{2,})-([A-Z]{2,})-(.+Neural)$`)
	if m := re.FindStringSubmatch(voice); m != nil {
		lang, region, name := m[1], m[2], m[3]
		if i := strings.Index(name, "-"); i != -1 {
			region = region + "-" + name[:i]
			name = name[i+1:]
		}
		return fmt.Sprintf("Microsoft Server Speech Text to Speech Voice (%s-%s, %s)", lang, region, name)
	}
	return voice
}

func edgeTTSEscapeXML(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func edgeTTSRemoveIncompatible(s string) string {
	var b strings.Builder
	for _, r := range s {
		code := int(r)
		if (0 <= code && code <= 8) || (11 <= code && code <= 12) || (14 <= code && code <= 31) {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func edgeTTSMkSSML(voice, rate, volume, pitch, text string) string {
	return fmt.Sprintf(
		"<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'>"+
			"<voice name='%s'>"+
			"<prosody pitch='%s' rate='%s' volume='%s'>"+
			"%s"+
			"</prosody>"+
			"</voice>"+
			"</speak>",
		voice, pitch, rate, volume, text)
}

func edgeTTSSSMLHeadersPlusData(requestID, timestamp, ssml string) string {
	return fmt.Sprintf(
		"X-RequestId:%s\r\n"+
			"Content-Type:application/ssml+xml\r\n"+
			"X-Timestamp:%sZ\r\n"+ // 末尾 Z 是微软 Edge 的历史 bug，必须保留
			"Path:ssml\r\n\r\n"+
			"%s",
		requestID, timestamp, ssml)
}

func edgeTTSCalcMaxMsgSize(voice, rate, volume, pitch string) int {
	websocketMaxSize := 1 << 16
	overhead := len(edgeTTSSSMLHeadersPlusData(
		edgeTTSConnectID(),
		edgeTTSDateToString(),
		edgeTTSMkSSML(voice, rate, volume, pitch, ""),
	)) + 50
	return websocketMaxSize - overhead
}

// edgeTTSSplitByByteLength 按字节长度切分文本（对齐 difyz9 的 SplitTextByByteLength，
// 避免超长 SSML 超出 websocket 单帧上限）。
func edgeTTSSplitByByteLength(text string, byteLength int) [][]byte {
	if byteLength <= 0 {
		byteLength = 1
	}
	textBytes := []byte(text)
	var result [][]byte
	for len(textBytes) > byteLength {
		splitAt := bytes.LastIndex(textBytes[:byteLength], []byte(" "))
		if splitAt == -1 {
			splitAt = byteLength
		}
		for bytes.Contains(textBytes[:splitAt], []byte("&")) {
			amp := bytes.LastIndex(textBytes[:splitAt], []byte("&"))
			if bytes.Index(textBytes[amp:splitAt], []byte(";")) != -1 {
				break
			}
			splitAt = amp - 1
			if splitAt < 0 {
				splitAt = 0
				break
			}
		}
		newText := bytes.TrimSpace(textBytes[:splitAt])
		if len(newText) > 0 {
			result = append(result, newText)
		}
		if splitAt == 0 {
			splitAt = 1
		}
		textBytes = textBytes[splitAt:]
	}
	newText := bytes.TrimSpace(textBytes)
	if len(newText) > 0 {
		result = append(result, newText)
	}
	return result
}

func edgeTTSParseHeaders(data []byte) map[string]string {
	headers := make(map[string]string)
	for _, line := range bytes.Split(data, []byte("\r\n")) {
		if len(line) == 0 {
			continue
		}
		parts := bytes.SplitN(line, []byte(":"), 2)
		if len(parts) != 2 {
			continue
		}
		headers[string(bytes.TrimSpace(parts[0]))] = string(bytes.TrimSpace(parts[1]))
	}
	return headers
}

// edgeTTSReadMessage 在 ctx 约束下读取一条 websocket 消息，避免网络静默丢包时无限挂起。
func edgeTTSReadMessage(ctx context.Context, conn *websocket.Conn) (int, []byte, error) {
	type result struct {
		mt   int
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		mt, data, err := conn.ReadMessage()
		ch <- result{mt, data, err}
	}()
	select {
	case r := <-ch:
		return r.mt, r.data, r.err
	case <-ctx.Done():
		conn.Close()
		return 0, nil, ctx.Err()
	}
}

// ── 连接与合成 ──────────────────────────────────────────────────────

// edgeTTSConnect 建立 WebSocket 连接，处理 403 时钟偏移重试。
func edgeTTSConnect(ctx context.Context, proxyAddr string) (*websocket.Conn, error) {
	dialer := edgeTTSDialer(proxyAddr, 15*time.Second)
	header := http.Header{}
	for k, v := range edgeTTSWSSHeaders {
		header.Set(k, v)
	}
	// 官方库会附加一个随机 muid Cookie，部分校验路径依赖它。
	header.Set("Cookie", "muid="+edgeTTSConnectID()+";")
	conn, resp, err := dialer.DialContext(ctx, edgeTTSWSSURL(), header)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusForbidden {
			if herr := edgeTTSHandle403(resp); herr == nil {
				if c2, _, e2 := dialer.DialContext(ctx, edgeTTSWSSURL(), header); e2 == nil {
					c2.EnableWriteCompression(true)
					return c2, nil
				}
			}
		}
		return nil, fmt.Errorf("websocket 握手失败：%v", err)
	}
	conn.EnableWriteCompression(true)
	return conn, nil
}

func edgeTTSSendCommand(conn *websocket.Conn) error {
	msg := fmt.Sprintf(
		"X-Timestamp:%s\r\n"+
			"Content-Type:application/json; charset=utf-8\r\n"+
			"Path:speech.config\r\n\r\n"+
			`{"context":{"synthesis":{"audio":{"metadataoptions":{`+
			`"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"true"},`+
			`"outputFormat":"audio-24khz-48kbitrate-mono-mp3"`+
			`}}}}`,
		edgeTTSDateToString())
	return conn.WriteMessage(websocket.TextMessage, []byte(msg))
}

func edgeTTSSendSSML(conn *websocket.Conn, voice, rate, volume, pitch, text string) error {
	ssml := edgeTTSMkSSML(voice, rate, volume, pitch, text)
	msg := fmt.Sprintf(
		"X-RequestId:%s\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:%sZ\r\nPath:ssml\r\n\r\n%s",
		edgeTTSConnectID(), edgeTTSDateToString(), ssml)
	return conn.WriteMessage(websocket.TextMessage, []byte(msg))
}

// edgeTTSSynthesizeChunk 对单段文本完成一次「连接 → 配置 → SSML → 收取音频」流程。
func edgeTTSSynthesizeChunk(ctx context.Context, text, voice, rate, volume, pitch, proxyAddr string) ([]byte, error) {
	conn, err := edgeTTSConnect(ctx, proxyAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := edgeTTSSendCommand(conn); err != nil {
		return nil, fmt.Errorf("发送配置失败：%v", err)
	}
	if err := edgeTTSSendSSML(conn, voice, rate, volume, pitch, text); err != nil {
		return nil, fmt.Errorf("发送 SSML 失败：%v", err)
	}

	var audio []byte
	audioReceived := false
	for {
		mt, data, err := edgeTTSReadMessage(ctx, conn)
		if err != nil {
			return nil, fmt.Errorf("读取响应失败：%v", err)
		}
		if mt == websocket.TextMessage {
			headers := edgeTTSParseHeaders(data)
			switch headers["Path"] {
			case "turn.end":
				goto done
			case "audio.metadata", "response", "turn.start":
				// 忽略元数据与控制帧
			default:
				// 未知文本帧忽略，继续读取
			}
		} else if mt == websocket.BinaryMessage {
			if len(data) < 2 {
				continue
			}
			hl := int(binary.BigEndian.Uint16(data[:2]))
			if hl+2 > len(data) {
				continue
			}
			headers := edgeTTSParseHeaders(data[2 : 2+hl])
			audioData := data[2+hl:]
			if headers["Path"] == "audio" && headers["Content-Type"] == "audio/mpeg" && len(audioData) > 0 {
				audio = append(audio, audioData...)
				audioReceived = true
			}
		}
	}
done:
	if !audioReceived {
		return nil, fmt.Errorf("未收到音频数据")
	}
	return audio, nil
}

// edgeTTSSynthesize 自实现 Edge TTS 合成（MP3），支持代理与长文本自动切分。
func edgeTTSSynthesize(ctx context.Context, text, voice, rate, volume, pitch, proxyAddr string) ([]byte, error) {
	voice = edgeTTSNormalizeVoice(voice)
	clean := edgeTTSRemoveIncompatible(text)
	escaped := edgeTTSEscapeXML(clean)
	chunks := edgeTTSSplitByByteLength(escaped, edgeTTSCalcMaxMsgSize(voice, rate, volume, pitch))

	var out []byte
	for _, c := range chunks {
		audio, err := edgeTTSSynthesizeChunk(ctx, string(c), voice, rate, volume, pitch, proxyAddr)
		if err != nil {
			return nil, err
		}
		out = append(out, audio...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("未返回音频数据")
	}
	return out, nil
}

// edgeTTSListVoices 通过代理感知的 HTTP 客户端拉取简体中文音色列表。
func edgeTTSListVoices(ctx context.Context, proxyAddr string) ([]EdgeTTSVoice, error) {
	client := edgeTTSHTTPClient(proxyAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		edgeTTSVoiceListURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range edgeTTSVoiceHeaders {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("音色列表请求失败（%d）", resp.StatusCode)
	}
	var vlist []types.Voice
	if err := json.NewDecoder(resp.Body).Decode(&vlist); err != nil {
		return nil, err
	}
	list := make([]EdgeTTSVoice, 0, len(vlist))
	for _, v := range vlist {
		if !strings.HasPrefix(v.Locale, "zh-CN") {
			continue
		}
		list = append(list, EdgeTTSVoice{
			ShortName:    v.ShortName,
			FriendlyName: edgeTTSFriendlyName(v),
			Gender:       v.Gender,
			Locale:       v.Locale,
		})
	}
	return list, nil
}

// edgeTTSFriendlyName 为音色生成友好显示名（优先使用接口返回的 FriendlyName）。
// 接收 difyz9 的 types.Voice（原始解码结构），difyz9 v0.0.3 的 FriendlyName 为空，
// 需基于 ShortName 自行生成可读名称，否则前端下拉会显示空白。
func edgeTTSFriendlyName(v types.Voice) string {
	if v.FriendlyName != "" {
		return v.FriendlyName
	}
	// ShortName 形如 zh-CN-XiaoxiaoNeural 或 zh-CN-liaoning-XiaobeiNeural
	parts := strings.Split(v.ShortName, "-")
	if len(parts) >= 2 {
		voiceName := parts[len(parts)-1]
		locale := strings.Join(parts[:len(parts)-1], "-")
		localeName := locale
		if name, ok := edgeTTSLocaleNames[locale]; ok {
			localeName = name
		}
		return fmt.Sprintf("%s（%s）", voiceName, localeName)
	}
	if v.Name != "" {
		return v.Name
	}
	return v.ShortName
}
