package services

import (
	"regexp"
	"strings"
	"unicode/utf8"
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

// HTML 噪音清洗共用正则（仅用于解析质量差、标签未完全展开的源，如 mobi/azw3 低质量流）。
// 目标是剔除仍残留在文本里的标签名、属性赋值、实体与乱码字符，避免它们被 TTS 当成正文朗读。
var (
	// 完整/破损 HTML 标签：从 < 起，到 > 或遇到非 ASCII（中文）或行尾为止。
	// MOBI7 的标签常缺失闭合 >，使 font/div/h3/属性名等裸露，必须按 < 起始整段清除。
	abReTagFragment = regexp.MustCompile(`<[ -~]*`)
	abReEntity      = regexp.MustCompile(`&(?:[a-zA-Z]+|#[0-9]+);`)
	abReHTMLAttr    = regexp.MustCompile(`[A-Za-z_:][\w:.-]*\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'<>]+)`)
	abReTagToken    = regexp.MustCompile(`</?|\b(?:html|head|body|div|span|font|p|br|h[1-6]|table|tr|td|th|img|a|ul|ol|li|em|strong|b|i|u|center|meta|style|script|title|guide|mbp|pagebreak|dl|dt|dd|blockquote|hr|sub|sup|justify|face|size|width|height|align|class|recindex|kindle)\b`)
	// 残留的孤立 ASCII 字母串（长度 >=3）：如 ldiv、/font 残留的 "ont"、overlap 垃圾 "gggg"、
	// 以及标签碎片 ylo/fac/tif 等；中文正文几乎不会连续出现 3+ 个 ASCII 字母，误删风险极低。
	abReAsciiWord = regexp.MustCompile(`[A-Za-z]{3,}`)
)

// stripHTMLNoise 移除文本中残留的 HTML 实体、属性赋值（name="..." / name=value）、标签名
// token（含 < 缺失的破损标签，如 ldiv、/font>、<导）、孤立 ASCII 字母碎片与乱码字节。
// 用于 mobi 等标签未能完全展开的来源。白名单仅含 HTML 关键字，中文正文几乎不会命中，
// 误删风险极低。
func stripHTMLNoise(s string) string {
	// 先移除 MOBI7 解码残留的无效 UTF-8 字节（乱码控制字节 0x80–0x9F/0xA0–0xFF）：
	// 这些字节会被 Go 当无效序列转成 U+FFFD（显示为方框），既无法朗读，又打断中文断句，
	// 必须整段剔除。注意它们不是合法 U+FFFD 字符，无法用字符串替换清除，需在字节层处理。
	s = removeInvalidUTF8(s)
	// 整段清除从 < 起始的完整/破损标签（即便缺失闭合 > 也能清掉 font/div/h3/属性名等）
	s = abReTagFragment.ReplaceAllString(s, " ")
	s = abReEntity.ReplaceAllString(s, " ")
	s = abReHTMLAttr.ReplaceAllString(s, " ")
	s = abReTagToken.ReplaceAllString(s, " ")
	s = abReAsciiWord.ReplaceAllString(s, " ")
	// 残留的孤立尖括号/引号/等号（正常中文正文不含这些 ASCII 控制字符）
	s = strings.NewReplacer("<", " ", ">", " ", "\"", " ", "=", " ").Replace(s)
	return s
}

// removeInvalidUTF8 剔除字符串中无效的 UTF-8 字节序列（MOBI7 解码残留的乱码字节）。
// Go 在遍历无效序列时每字节产生一个 utf8.RuneError(U+FFFD)，过滤掉即可；合法中文不受影响。
func removeInvalidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r != utf8.RuneError {
			b.WriteRune(r)
		}
	}
	return b.String()
}

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
	raw = stripHTMLNoise(raw)
	return cleanTextLines(raw, cfg)
}

// CleanBookText 结构化源清洗入口：对已抽出的纯文本（各适配器产出）做行级过滤。
// 与 FilterRawBookText 共用同一过滤核心，保证 epub/mobi 等来源处理一致。
func CleanBookText(raw string, cfg CleanConfig) string {
	return cleanTextLines(raw, cfg)
}
