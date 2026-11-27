package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// 讯飞在线语音合成（流式版 WebAPI）：
// wss://tts-api.xfyun.cn/v2/tts，HMAC-SHA256 握手签名，单次文本 < 8000 字节，
// 请求 aue=lame&sfl=1 时返回 MP3 片段（base64），收到 data.status=2 表示结束。
const (
	xfyunDefaultWSURL = "wss://tts-api.xfyun.cn/v2/tts"
	xfyunMaxTextBytes = 7800 // 官方上限 8000 字节，留少量余量
	xfyunDefaultVoice = "xiaoyan"
)

type xfyunCredentials struct {
	AppID       string
	APIKey      string
	APISecret   string
	APIPassword string
	WSURL       string
}

func xfyunConfigFromEnv() xfyunCredentials {
	c := xfyunCredentials{
		AppID:       strings.TrimSpace(os.Getenv("XF_APP_ID")),
		APIKey:      strings.TrimSpace(os.Getenv("XF_API_KEY")),
		APISecret:   strings.TrimSpace(os.Getenv("XF_API_SECRET")),
		APIPassword: strings.TrimSpace(os.Getenv("XF_API_PASSWORD")),
		WSURL:       strings.TrimSpace(os.Getenv("XF_TTS_WS_URL")),
	}
	if c.WSURL == "" {
		c.WSURL = xfyunDefaultWSURL
	}
	return c
}

// xfyunConfigFromConfig 使用当前用户的设置（GetTTSConfig 已回退到环境变量）。
func xfyunConfigFromConfig(cfg TTSConfig) xfyunCredentials {
	c := xfyunConfigFromEnv()
	if cfg.XfAppID != "" {
		c.AppID = cfg.XfAppID
	}
	if cfg.XfAPIKey != "" {
		c.APIKey = cfg.XfAPIKey
	}
	if cfg.XfAPISecret != "" {
		c.APISecret = cfg.XfAPISecret
	}
	if cfg.XfAPIPassword != "" {
		c.APIPassword = cfg.XfAPIPassword
	}
	return c
}

func (c xfyunCredentials) configured() bool {
	if c.AppID == "" {
		return false
	}
	// 鉴权方式一：控制台 APIPassword（x-api-key 头）；方式二：APIKey + APISecret 签名
	return c.APIPassword != "" || (c.APIKey != "" && c.APISecret != "")
}

// xfyunAuthURL 生成带 host/date/authorization 查询参数的握手 URL。
// date 必须为 GMT 的 RFC1123 格式（允许与服务端相差 5 分钟以内）。
func xfyunAuthURL(hostURL, apiKey, apiSecret string) (string, error) {
	u, err := url.Parse(hostURL)
	if err != nil {
		return "", err
	}
	date := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	signOrigin := strings.Join([]string{
		"host: " + u.Host,
		"date: " + date,
		"GET " + u.Path + " HTTP/1.1",
	}, "\n")
	mac := hmac.New(sha256.New, []byte(apiSecret))
	_, _ = mac.Write([]byte(signOrigin))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	authOrigin := fmt.Sprintf(`api_key="%s", algorithm="hmac-sha256", headers="host date request-line", signature="%s"`,
		apiKey, signature)
	q := u.Query()
	q.Set("host", u.Host)
	q.Set("date", date)
	q.Set("authorization", base64.StdEncoding.EncodeToString([]byte(authOrigin)))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// xfyunSpeed 将倍速系数映射到讯飞 speed（0-100，50 为默认）。不支持时保持 50。
func xfyunSpeed(speedFactor float64) int {
	if speedFactor <= 0 {
		return 50
	}
	v := int(math.Round(50 + (speedFactor-1)*50))
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// xfyunVoice 过滤其它引擎的音色名（如 Edge 的 zh-CN-XiaoxiaoNeural），回退到默认发音人。
func xfyunVoice(voice string) string {
	v := strings.TrimSpace(voice)
	if v == "" || strings.Contains(v, "-") {
		if def := strings.TrimSpace(os.Getenv("XF_TTS_VOICE")); def != "" {
			return def
		}
		return xfyunDefaultVoice
	}
	return v
}

// XunfeiVoices 常用中文发音人列表（需在讯飞控制台开通对应音库）。
func XunfeiVoices() []map[string]interface{} {
	return []map[string]interface{}{
		{"short_name": "xiaoyan", "friendly_name": "晓燕（通用女声）", "gender": "Female", "engine": EngineXunfei},
		{"short_name": "xiaoyuan", "friendly_name": "晓媛（女声）", "gender": "Female", "engine": EngineXunfei},
		{"short_name": "xiaofeng", "friendly_name": "晓峰（男声）", "gender": "Male", "engine": EngineXunfei},
		{"short_name": "xiaoli", "friendly_name": "晓莉（女声）", "gender": "Female", "engine": EngineXunfei},
		{"short_name": "xiaoyu", "friendly_name": "晓宇（男声）", "gender": "Male", "engine": EngineXunfei},
	}
}

func synthesizeXunfei(cfg TTSConfig, text string, speedFactor float64) ([]byte, string, error) {
	creds := xfyunConfigFromConfig(cfg)
	if !creds.configured() {
		return nil, "", fmt.Errorf("讯飞 TTS 未配置：请在「设置 → 语音合成」填写 AppID 与 APIKey/APISecret（或 APIPassword）")
	}
	voice := xfyunVoice(cfg.Voice)
	speed := xfyunSpeed(speedFactor)
	var audio []byte
	for _, chunk := range splitXunfeiText(text) {
		part, err := xfyunSynthesizeOnce(creds, chunk, voice, speed)
		if err != nil {
			return nil, "", err
		}
		audio = append(audio, part...)
	}
	return audio, "audio/mpeg", nil
}

// splitXunfeiText 按字节上限切分文本，优先在句末标点处断开（MP3 帧流可直接拼接）。
func splitXunfeiText(text string) []string {
	runes := []rune(text)
	var chunks []string
	for start := 0; start < len(runes); {
		end, used, lastBreak := start, 0, -1
		for end < len(runes) {
			n := len(string(runes[end]))
			if used+n > xfyunMaxTextBytes {
				break
			}
			used += n
			end++
			if strings.ContainsRune("。！？!?；;\n", runes[end-1]) {
				lastBreak = end
			}
		}
		if end == start { // 单字符超过上限（UTF-8 不可能，防御性处理）
			end = start + 1
		}
		if lastBreak > start {
			end = lastBreak
		}
		chunks = append(chunks, string(runes[start:end]))
		start = end
	}
	return chunks
}

func xfyunSynthesizeOnce(creds xfyunCredentials, text, voice string, speed int) ([]byte, error) {
	headers := http.Header{}
	dialURL := creds.WSURL
	if creds.APIPassword != "" {
		headers.Set("x-api-key", creds.APIPassword)
	} else {
		var err error
		dialURL, err = xfyunAuthURL(creds.WSURL, creds.APIKey, creds.APISecret)
		if err != nil {
			return nil, fmt.Errorf("讯飞 TTS 鉴权 URL 构造失败：%w", err)
		}
	}

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := dialer.Dial(dialURL, headers)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return nil, fmt.Errorf("讯飞 TTS 握手失败（HTTP %d）：%s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("讯飞 TTS 连接失败：%w", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(120 * time.Second))

	payload := map[string]interface{}{
		"common": map[string]interface{}{
			"app_id": creds.AppID,
			"uid":    "moyue-books",
		},
		"business": map[string]interface{}{
			"aue":   "lame", // MP3
			"sfl":   1,      // aue=lame 时必须开启流式返回
			"vcn":   voice,
			"tte":   "utf8",
			"speed": speed,
		},
		"data": map[string]interface{}{
			"status": 2,
			"text":   base64.StdEncoding.EncodeToString([]byte(text)),
		},
	}
	if err := conn.WriteJSON(payload); err != nil {
		return nil, fmt.Errorf("讯飞 TTS 发送请求失败：%w", err)
	}

	var audio []byte
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("讯飞 TTS 读取响应失败：%w", err)
		}
		if msgType != websocket.TextMessage {
			continue
		}
		var frame struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    *struct {
				Audio  string `json:"audio"`
				Status int    `json:"status"`
			} `json:"data"`
		}
		if json.Unmarshal(msg, &frame) != nil {
			continue
		}
		if frame.Code != 0 {
			return nil, fmt.Errorf("讯飞 TTS 合成失败（code=%d）：%s", frame.Code, frame.Message)
		}
		if frame.Data == nil {
			continue
		}
		if frame.Data.Audio != "" {
			part, err := base64.StdEncoding.DecodeString(frame.Data.Audio)
			if err != nil {
				return nil, fmt.Errorf("讯飞 TTS 音频解码失败：%w", err)
			}
			audio = append(audio, part...)
		}
		if frame.Data.Status == 2 {
			break
		}
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("讯飞 TTS 未返回音频数据")
	}
	_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	return audio, nil
}
