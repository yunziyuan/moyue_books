package services

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// TestEdgeTTSGeCV2MatchesOfficial 用官方 Python edge_tts 库对固定时间戳生成 v2 令牌，
// 与本项目 Go 实现交叉验证，确保改版 SHA256/CS64 移植正确。
// 若本地不存在该 venv 则跳过（不影响 CI）。
func TestEdgeTTSGeCV2MatchesOfficial(t *testing.T) {
	venvPython := "/Users/apple/Documents/my/books/tts-service/.venv/bin/python"
	if _, err := exec.LookPath(venvPython); err != nil {
		t.Skip("venv python not found, skip cross-check")
	}
	ts := 1761550000.0
	cmd := exec.Command(venvPython, "-c",
		"from edge_tts.drm import generate_sec_ms_gec_v2; print(generate_sec_ms_gec_v2("+
			strconv.FormatFloat(ts, 'f', -1, 64)+"))")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("python exec failed: %v", err)
	}
	want := strings.TrimSpace(string(out))
	got := gecGenerateSecMSGECSpecific(ts)
	if want != got {
		t.Fatalf("v2 token mismatch:\n want=%s\n got =%s", want, got)
	}
}
