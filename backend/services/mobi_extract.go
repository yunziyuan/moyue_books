package services

// mobiReaderIface 有声书流水线对 mobi 章节文本读取器的要求（与 EPUB/AZW3 读取器同形）。
type mobiReaderIface interface {
	GetContent(int) ([]byte, error)
	Close()
}

// openMobiReader 打开 mobi 并返回章节文本读取器。
// 编译期带 -tags nocgo（go-fitz/MuPDF）时使用 go-fitz 提取文本；否则回退纯 Go 的 OpenAZW3。
// 默认（无 nocgo 标签）构建保持 CGO_ENABLED=0 单二进制，mobi 走原有纯 Go 解析器。
func openMobiReader(path string) (mobiReaderIface, error) {
	return openMobiReaderImpl(path)
}

// ExtractMobiMetadata 提取 mobi 元数据（nocgo 走 go-fitz，否则回退 OpenAZW3）。
func ExtractMobiMetadata(path string) (*BookMetadata, error) {
	return extractMobiMetadataImpl(path)
}
