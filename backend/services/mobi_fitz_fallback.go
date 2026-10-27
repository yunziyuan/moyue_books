//go:build !nocgo

package services

// openMobiReaderImpl 默认（无 nocgo 标签）构建下回退到纯 Go 的 OpenAZW3 解析器，
// 保持 CGO_ENABLED=0 单二进制；KF8(azw3) 质量良好，老式 MOBI7 可能乱码（见历史诊断）。
func openMobiReaderImpl(path string) (mobiReaderIface, error) {
	return OpenAZW3(path)
}

func extractMobiMetadataImpl(path string) (*BookMetadata, error) {
	m, err := extractAZW3Metadata(path)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// extractMobiTOCImpl 默认（无 nocgo 标签）构建下回退到纯 Go 解析器的目录。
func extractMobiTOCImpl(path string) ([]TOCItem, error) {
	return extractAZW3TOC(path)
}
