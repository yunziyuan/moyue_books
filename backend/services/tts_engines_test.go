package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestXfyunAuthURL(t *testing.T) {
	authURL, err := xfyunAuthURL("wss://tts-api.xfyun.cn/v2/tts", "key123", "secret456")
	if err != nil {
		t.Fatalf("xfyunAuthURL: %v", err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	q := u.Query()
	if q.Get("host") != "tts-api.xfyun.cn" {
		t.Fatalf("host = %q", q.Get("host"))
	}
	date := q.Get("date")
	if !strings.HasSuffix(date, " GMT") {
		t.Fatalf("date must be RFC1123 GMT, got %q", date)
	}
	auth, err := base64.StdEncoding.DecodeString(q.Get("authorization"))
	if err != nil {
		t.Fatalf("decode authorization: %v", err)
	}
	signOrigin := "host: tts-api.xfyun.cn\ndate: " + date + "\nGET /v2/tts HTTP/1.1"
	mac := hmac.New(sha256.New, []byte("secret456"))
	_, _ = mac.Write([]byte(signOrigin))
	want := "api_key=\"key123\", algorithm=\"hmac-sha256\", headers=\"host date request-line\", signature=\"" +
		base64.StdEncoding.EncodeToString(mac.Sum(nil)) + "\""
	if string(auth) != want {
		t.Fatalf("authorization mismatch\ngot:  %s\nwant: %s", auth, want)
	}
}

func TestXfyunSpeed(t *testing.T) {
	cases := []struct {
		factor float64
		want   int
	}{
		{0, 50}, {1.0, 50}, {0.5, 25}, {1.5, 75}, {2.0, 100}, {5.0, 100},
	}
	for _, c := range cases {
		if got := xfyunSpeed(c.factor); got != c.want {
			t.Errorf("xfyunSpeed(%v) = %d, want %d", c.factor, got, c.want)
		}
	}
}

func TestSplitXunfeiText(t *testing.T) {
	text := strings.Repeat("讯飞语音合成测试。", 2000)
	chunks := splitXunfeiText(text)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	var joined strings.Builder
	for _, c := range chunks {
		if len(c) > xfyunMaxTextBytes {
			t.Fatalf("chunk too long: %d bytes", len(c))
		}
		joined.WriteString(c)
	}
	if joined.String() != text {
		t.Fatal("split chunks lost text")
	}
}

func TestSynthesizeXunfeiWithLocalServer(t *testing.T) {
	var received map[string]interface{}
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if err := conn.ReadJSON(&received); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		for i, chunk := range []string{"abc", "def"} {
			status := 1
			if i == 1 {
				status = 2
			}
			_ = conn.WriteJSON(map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"audio":  base64.StdEncoding.EncodeToString([]byte(chunk)),
					"status": status,
				},
			})
		}
	}))
	defer srv.Close()

	// 凭据来自用户配置（非环境变量），WSURL 仍走环境变量默认端点
	t.Setenv("XF_TTS_WS_URL", "ws"+strings.TrimPrefix(srv.URL, "http")+"/v2/tts")
	cfg := TTSConfig{
		Voice:       "xiaoyan",
		XfAppID:     "app123",
		XfAPIKey:    "key123",
		XfAPISecret: "secret456",
	}

	data, ct, err := synthesizeXunfei(cfg, "你好，世界", 1.5)
	if err != nil {
		t.Fatalf("synthesizeXunfei: %v", err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("audio = %q, want abcdef", data)
	}
	if ct != "audio/mpeg" {
		t.Fatalf("content type = %q", ct)
	}
	common, ok := received["common"].(map[string]interface{})
	if !ok || common["app_id"] != "app123" {
		t.Fatalf("unexpected common params: %v", received["common"])
	}
	business, ok := received["business"].(map[string]interface{})
	if !ok || business["aue"] != "lame" || business["sfl"] != float64(1) || business["vcn"] != "xiaoyan" {
		t.Fatalf("unexpected business params: %v", received["business"])
	}
	if business["speed"] != float64(75) {
		t.Fatalf("speed = %v, want 75", business["speed"])
	}
}

func TestDeepSeekEndpointNormalization(t *testing.T) {
	t.Setenv("DEEPSEEK_TTS_BASE_URL", "")
	cases := map[string]string{
		"":                                      "https://api.deepseek.com/v1/tools/tts",
		"https://api.deepseek.com":              "https://api.deepseek.com/v1/tools/tts",
		"https://api.deepseek.com/":             "https://api.deepseek.com/v1/tools/tts",
		"https://api.deepseek.com/v1":           "https://api.deepseek.com/v1/tools/tts",
		"https://api.deepseek.com/v1/":          "https://api.deepseek.com/v1/tools/tts",
		"https://api.deepseek.com/v1/tools/tts": "https://api.deepseek.com/v1/tools/tts",
		"https://proxy.example.com/deepseek":    "https://proxy.example.com/deepseek/v1/tools/tts",
	}
	for input, want := range cases {
		if got := deepSeekEndpoint(input); got != want {
			t.Errorf("deepSeekEndpoint(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSynthesizeDeepSeekWithLocalServer(t *testing.T) {
	wav := append([]byte("RIFF\x00\x00\x00\x00WAVE"), []byte("mock audio")...)
	var gotPath, gotAuth string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wav)
	}))
	defer srv.Close()

	// 环境变量故意设为不可用值，验证用户配置优先
	t.Setenv("DEEPSEEK_API_KEY", "env-key-should-not-be-used")
	t.Setenv("DEEPSEEK_TTS_BASE_URL", "http://127.0.0.1:1")
	data, ct, err := synthesizeDeepSeek(TTSConfig{
		Voice:          "tide",
		DeepSeekAPIKey: "test-key",
		DeepSeekBase:   srv.URL,
	}, "你好")
	if err != nil {
		t.Fatalf("synthesizeDeepSeek: %v", err)
	}
	if string(data) != string(wav) || ct != "audio/wav" {
		t.Fatalf("unexpected audio: %q / %q", data, ct)
	}
	if gotPath != "/v1/tools/tts" || gotAuth != "Bearer test-key" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	if gotBody["voice_id"] != "tide" || gotBody["format"] != "wav" || gotBody["text"] != "你好" {
		t.Fatalf("unexpected body: %v", gotBody)
	}
}

func TestAudiobookSegmentExt(t *testing.T) {
	cases := map[string]string{
		EngineEdgeTTS:  "mp3",
		EngineXunfei:   "mp3",
		EngineDeepSeek: "wav",
		EngineAuto:     "mp3",
		"":             "mp3",
	}
	for engine, want := range cases {
		if got := AudiobookSegmentExt(engine); got != want {
			t.Errorf("AudiobookSegmentExt(%q) = %q, want %q", engine, got, want)
		}
	}
}
