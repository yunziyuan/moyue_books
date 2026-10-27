package services

import (
	"regexp"
	"strings"
)

// CleanConfig 有声书生成前文本预处理的开关。
// 两套清洗策略共用同一组正则：
//   - 结构化源（epub/kepub/pdf/md/txt）：先由各自适配器抽出纯文本，再走行级过滤；
//   - 低质量源（mobi/azw3 抽出的纯文本）：同样走行级过滤，容错强、适合标签残缺。
type CleanConfig struct {
	RemovePageNum   bool // 移除页码（第 xx 页）
	RemoveTocLine   bool // 移除典型目录行（1.1 标题…… 12）
	RemoveFigure    bool // 移除图注（图 1-1 xxx）
	RemoveBracket   bool // 移除【】/[] 括号内容
	RemoveURL       bool // 移除网址 / mailto
	RemoveImageMark bool // 移除图片占位标记（[图片]、img:、image:）
}

// DefaultAudioBookCleanConfig 默认清洗配置：保留章节标题、正文与对话，去除干扰内容。
var DefaultAudioBookCleanConfig = CleanConfig{
	RemovePageNum:   true,
	RemoveTocLine:   true,
	RemoveFigure:    true,
	RemoveBracket:   false,
	RemoveURL:       true,
	RemoveImageMark: true,
}

// 文本清洗共用正则（仅标准库，避免引入额外 DOM 依赖）
var (
	abRePageNum      = regexp.MustCompile(`^\s*第?\d+\s*页\s*$`)
	abReTocLine      = regexp.MustCompile(`^\s*[\d\.]+.*[…\.]{2,}\s*\d+\s*$`) // 目录行 1.2 标题……23（兼容全角省略号…与半角.）
	abReFigure       = regexp.MustCompile(`^\s*图\s*\d+[-－]\d+\s+.*$`)
	abReMultiSpace   = regexp.MustCompile(`\s+`)
	abReBracketZh    = regexp.MustCompile(`【[^】]*】`)
	abReBracketEn    = regexp.MustCompile(`\[[^\]]*\]`)
	abReURL          = regexp.MustCompile(`(?i)(?:https?://|mailto:)[^\s，。！？；：'"）]+`)
	abReImageTagMark = regexp.MustCompile(`(?i)\[图片\]|\[image\]|img:|image:`)
)

// cleanTextLines 对纯文本逐行过滤（两套清洗策略共用核心）：
// 过滤 URL、图片标记、页码、目录行、图注、括号，并合并多余空白。
// 段落之间以空行分隔，便于 TTS 在段落处停顿。
func cleanTextLines(raw string, cfg CleanConfig) string {
	var keepLines []string
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if cfg.RemoveURL {
			line = abReURL.ReplaceAllString(line, "")
		}
		if cfg.RemoveImageMark {
			line = abReImageTagMark.ReplaceAllString(line, "")
		}
		if cfg.RemovePageNum && abRePageNum.MatchString(line) {
			continue
		}
		if cfg.RemoveTocLine && abReTocLine.MatchString(line) {
			continue
		}
		if cfg.RemoveFigure && abReFigure.MatchString(line) {
			continue
		}
		if cfg.RemoveBracket {
			line = abReBracketZh.ReplaceAllString(line, "")
			line = abReBracketEn.ReplaceAllString(line, "")
		}
		line = abReMultiSpace.ReplaceAllString(line, " ")
		line = strings.TrimSpace(line)
		if line != "" {
			keepLines = append(keepLines, line)
		}
	}
	return strings.Join(keepLines, "\n\n")
}

// FilterRawBookText 原始文本过滤器，专给解析质量差的源使用（mobi 等标签残缺场景）。
// 无 DOM、纯文本行过滤，容错高；精华与章节标题由调用方在传入前已保留。
// 若后续以外部工具拿到 azw3 文本，直接调用本函数即可复用同一套过滤逻辑。
func FilterRawBookText(raw string, cfg CleanConfig) string {
	return cleanTextLines(raw, cfg)
}

// CleanBookText 结构化源清洗入口：对已抽出的纯文本（各适配器产出）做行级过滤。
// 与 FilterRawBookText 共用同一过滤核心，保证 epub/mobi 等来源处理一致。
func CleanBookText(raw string, cfg CleanConfig) string {
	return cleanTextLines(raw, cfg)
}
