package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DeepSeek 语音合成：POST {base}/v1/tools/tts，Bearer 鉴权，响应体直接返回 WAV 音频。
// 不支持语速调节与 model 参数；text 最多 10000 个计费字符、请求体 64 KiB。
const (
	deepSeekDefaultBaseURL = "https://api.deepseek.com/v1"
	deepSeekMaxTextBytes   = 60000 // 请求体 64 KiB，预留 JSON 开销
)

// DeepSeekVoices DeepSeek TTS 可选音色（语种由文本自动识别）。
func DeepSeekVoices() []map[string]interface{} {
	return []map[string]interface{}{
		{"short_name": "stella", "friendly_name": "海星（女声）", "gender": "Female", "engine": EngineDeepSeek},
		{"short_name": "tide", "friendly_name": "暗潮（男声）", "gender": "Male", "engine": EngineDeepSeek},
	}
}

// deepSeekEndpoint 归一化 API 地址，兼容三种填写方式：
//   - https://api.deepseek.com
//   - https://api.deepseek.com/v1
//   - https://api.deepseek.com/v1/tools/tts
//
// 均得到官方端点 https://api.deepseek.com/v1/tools/tts（避免漏写 /v1 导致 404）。
func deepSeekEndpoint(override string) string {
	base := strings.TrimSpace(override)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("DEEPSEEK_TTS_BASE_URL"))
	}
	if base == "" {
		base = deepSeekDefaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/tools/tts") {
		return base
	}
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/tools/tts"
}

func deepSeekVoice(voice string) string {
	v := strings.TrimSpace(voice)
	if v == "stella" || v == "tide" {
		return v
	}
	return "stella"
}

func synthesizeDeepSeek(cfg TTSConfig, text string) ([]byte, string, error) {
	apiKey := strings.TrimSpace(cfg.DeepSeekAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	}
	if apiKey == "" {
		return nil, "", fmt.Errorf("DeepSeek TTS 未配置：请在「设置 → 语音合成」填写 API Key")
	}
	if len(text) > deepSeekMaxTextBytes {
		return nil, "", fmt.Errorf("DeepSeek TTS 文本过长（%d 字节，上限约 %d 字节）", len(text), deepSeekMaxTextBytes)
	}

	payload, err := json.Marshal(map[string]interface{}{
		"text":            text,
		"voice_id":        deepSeekVoice(cfg.Voice),
		"format":          "wav",
		"filter_markdown": true,
	})
	if err != nil {
		return nil, "", err
	}
	endpoint := deepSeekEndpoint(cfg.DeepSeekBase)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("DeepSeek TTS 请求失败：%w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("DeepSeek TTS 读取音频失败：%w", err)
	}
	if resp.StatusCode != http.StatusOK {
		extra := ""
		if resp.StatusCode == http.StatusNotFound {
			extra = fmt.Sprintf("；请求地址=%s（正确端点应为 https://api.deepseek.com/v1/tools/tts）", endpoint)
		}
		return nil, "", fmt.Errorf("DeepSeek TTS 合成失败（%d）%s：%s", resp.StatusCode, extra, truncate(string(data), 300))
	}
	// 文档约定 format=wav 返回 RIFF/WAVE；提前返回的 JSON 错误体也在这里拦住。
	if len(data) < 12 || !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WAVE")) {
		return nil, "", fmt.Errorf("DeepSeek TTS 返回非 WAV 音频：%s", truncate(string(data), 200))
	}
	return data, "audio/wav", nil
}
