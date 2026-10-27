package services

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// EdgeTTSDefaultVoice 默认中文女声（晓晓），Microsoft Edge 朗读在线音色
	// （长格式名，仅作兼容保留，Go 直连合成请使用 EdgeTTSDefaultVoiceShort）
	EdgeTTSDefaultVoice = "Microsoft Server Speech Text to Speech Voice (zh-CN, XiaoxiaoNeural)"
	// EdgeTTSDefaultVoiceShort 默认音色的 short name（用于设置项存储、前端下拉与 Go 直连合成）
	EdgeTTSDefaultVoiceShort = "zh-CN-XiaoxiaoNeural"
)

// edgeTTSMaxConcurrency 同时进行的 Edge TTS 网络连接（WebSocket）上限。
// 微软在线 TTS 是共享免费服务，对单 IP 的并发连接容忍度极低——过多并发会直接拒绝
// 或重置 WS 连接（表现为 websocket: bad handshake / connection reset），导致有声书
// 并发合成失败。因此默认串行（1）以保证稳定；如需更高吞吐，可经环境变量
// EDGE_TTS_MAX_CONCURRENCY 放宽（建议 ≤4，过大仍会被上游掐断）。
// 该闸门在 Edge TTS 网络层生效，无论有声书多少任务、每个任务多少路段，到微软的实际
// 并发连接数都被硬性限制，等价于"Edge TTS 不使用并发"。
var edgeTTSMaxConcurrency = func() int {
	n, _ := strconv.Atoi(os.Getenv("EDGE_TTS_MAX_CONCURRENCY"))
	if n <= 0 {
		n = 1
	}
	if n > 8 {
		n = 8
	}
	return n
}()

// edgeTTSConcurrencySem 全局 Edge TTS 并发信号量：每个到微软的合成调用（含长文本
// 内部重连、拆半重试）都必须先占用一个名额，释放前其余调用排队。
var edgeTTSConcurrencySem = make(chan struct{}, edgeTTSMaxConcurrency)

// EdgeTTSProxy 返回 edge-tts 专用代理地址：优先 EDGE_TTS_PROXY，回退通用
// HTTPS_PROXY / https_proxy。受限网络（国内/部分云主机）下微软端点会拒绝握手（403），
// 经代理出网可绕过；未配置时返回空（edge-tts 直连）。
// difyz9/edge-tts-go 在收到 403 时会依据服务端 Date 头自动校正本地时钟偏移并重试，
// 可规避因系统时钟偏差导致的 Sec-MS-GEC 令牌被拒（这正是旧实现偶发 403 的根因）。
func EdgeTTSProxy() string {
	for _, k := range []string{"EDGE_TTS_PROXY", "HTTPS_PROXY", "https_proxy"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// EdgeTTSVoice 微软 Edge TTS 可用音色（下发给前端展示）
type EdgeTTSVoice struct {
	ShortName    string `json:"short_name"`
	FriendlyName string `json:"friendly_name"`
	Gender       string `json:"gender"`
	Locale       string `json:"locale"`
}

// difyz9/edge-tts-go v0.0.3 的 voices.ListVoices 返回的 FriendlyName 为空，
// 需要基于 ShortName/Name 自行生成可读名称，否则前端下拉选项会显示空白。
var edgeTTSLocaleNames = map[string]string{
	"zh-CN":          "普通话",
	"zh-CN-liaoning": "东北话",
	"zh-CN-shaanxi":  "陕西话",
	"zh-CN-sichuan":  "四川话",
	"zh-CN-henan":    "河南话",
}

// normalizeEdgeTTSVoice 把旧版长格式音色名（如 Microsoft Server Speech Text to
// Speech Voice (zh-CN, XiaoxiaoNeural)）转成 difyz9 需要的 short name；已是 short
// name 则原样返回。用于兼容旧设置/任务中保存的长格式默认值。
func normalizeEdgeTTSVoice(voice string) string {
	voice = strings.TrimSpace(voice)
	if voice == "" {
		return EdgeTTSDefaultVoiceShort
	}
	prefix := "Microsoft Server Speech Text to Speech Voice ("
	if strings.HasPrefix(voice, prefix) && strings.HasSuffix(voice, ")") {
		inner := strings.TrimSuffix(strings.TrimPrefix(voice, prefix), ")")
		parts := strings.Split(inner, ", ")
		if len(parts) == 2 {
			return fmt.Sprintf("%s-%s", strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}
	return voice
}

// edgeTTSFriendlyName 为音色生成友好显示名（定义在 edgetts_ws.go，接收原始
// types.Voice；在 edgeTTSListVoices 中已完成计算，本处仅直接采用）。

var (
	edgeTTSCache     []EdgeTTSVoice
	edgeTTSCacheTime time.Time
)

// ListEdgeTTSVoices 返回中文音色列表（带 1 小时缓存）。
// 失败时返回 nil，由调用方静默降级（前端显示空列表）。
func ListEdgeTTSVoices() []EdgeTTSVoice {
	if len(edgeTTSCache) > 0 && time.Since(edgeTTSCacheTime) < time.Hour {
		return edgeTTSCache
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vlist, err := edgeTTSListVoices(ctx, EdgeTTSProxy())
	if err != nil {
		return nil
	}
	list := make([]EdgeTTSVoice, 0, len(vlist))
	for _, v := range vlist {
		// 只保留简体中文（含东北/四川/陕西/河南等方言音色）
		if !strings.HasPrefix(v.Locale, "zh-CN") {
			continue
		}
		list = append(list, EdgeTTSVoice{
			ShortName:    v.ShortName,
			FriendlyName: v.FriendlyName, // 已在 edgeTTSListVoices 内计算
			Gender:       v.Gender,
			Locale:       v.Locale,
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].FriendlyName < list[j].FriendlyName })
	edgeTTSCache = list
	edgeTTSCacheTime = time.Now()
	return list
}

// EdgeTTSSpeedToRate 语速倍率（0.5~2.0）转 edge-tts rate 参数（+20% / -30%）
func EdgeTTSSpeedToRate(speed float64) string {
	if speed <= 0 {
		speed = 1.0
	}
	pct := int((speed - 1.0) * 100)
	if pct >= 0 {
		return fmt.Sprintf("+%d%%", pct)
	}
	return fmt.Sprintf("%d%%", pct)
}

// EdgeTTSEmotion 情感对应的音色近似参数。
// Edge TTS 无真实情绪音色，只能通过语速 + 音量 + 音调组合近似模拟。
type EdgeTTSEmotion struct {
	SpeedFactor float64 // 相对用户语速的倍率（>1 更快，<1 更慢）
	Volume      string  // edge-tts volume 参数，如 "+5%"
	Pitch       string  // edge-tts pitch 参数，如 "+12Hz"
}

// EdgeTTSEmotionParams 返回情感对应的 Edge TTS 参数；未知情感按「平静」处理
func EdgeTTSEmotionParams(emotion string) EdgeTTSEmotion {
	switch emotion {
	case "愉悦":
		return EdgeTTSEmotion{1.10, "+5%", "+8Hz"} // 轻快、明亮
	case "悲伤":
		return EdgeTTSEmotion{0.90, "-5%", "-8Hz"} // 低沉、缓慢
	case "激昂":
		return EdgeTTSEmotion{1.15, "+8%", "+12Hz"} // 高亢、有力
	case "温柔":
		return EdgeTTSEmotion{0.95, "-3%", "+2Hz"} // 柔和
	case "自信":
		return EdgeTTSEmotion{1.05, "+3%", "+5Hz"}
	case "庄重":
		return EdgeTTSEmotion{0.90, "-3%", "-10Hz"} // 沉稳
	default: // 平静 / 未知
		return EdgeTTSEmotion{1.00, "+0%", "+0Hz"}
	}
}

// SynthesizeEdgeTTS 使用微软 Edge TTS 在线合成语音（Go 直连，自实现 websocket 客户端，
// 无需本地 Python tts-service 也不依赖 difyz9 的网络层），返回 MP3 音频字节。
// voice 为空时使用默认中文女声 short name（zh-CN-XiaoxiaoNeural）。
// 受限网络下微软端点可能 403/握手失败：设置 EDGE_TTS_PROXY（或 HTTPS_PROXY）后自动经
// 代理出网，支持 http/https/socks5 三类代理；未配置代理时强制直连（不再盲从环境中
// 的 HTTP_PROXY，避免被不可达系统代理连累导致 websocket: bad handshake）。
// 收到 403 时依据服务端 Date 头自动校正本地时钟偏移并重试，规避时钟偏差导致的
// Sec-MS-GEC 令牌被拒。文本内部的 XML 特殊字符由内部转义，超长文本按字节自动切分。
func SynthesizeEdgeTTS(text, voice, rate, volume, pitch string) ([]byte, error) {
	voice = normalizeEdgeTTSVoice(voice)
	if rate == "" {
		rate = "+0%"
	}
	if volume == "" {
		volume = "+0%"
	}
	if pitch == "" {
		pitch = "+0Hz"
	}

	// 占用 Edge TTS 全局并发名额：硬性限制到微软的并发连接数（默认 1），
	// 避免并发过高被上游拒绝/重置（websocket: bad handshake）。其余调用排队。
	edgeTTSConcurrencySem <- struct{}{}
	defer func() { <-edgeTTSConcurrencySem }()

	// 整体超时，避免网络静默丢包时 websocket 握手/读取无限挂起；
	// 自实现客户端内部已对读消息做 ctx 约束，并设了 15s 握手超时。
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	data, err := edgeTTSSynthesize(ctx, text, voice, rate, volume, pitch, EdgeTTSProxy())
	if err != nil {
		return nil, fmt.Errorf("Edge TTS 合成失败：%v", err)
	}
	return data, nil
}
