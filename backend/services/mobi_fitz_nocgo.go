//go:build nocgo

package services

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gen2brain/go-fitz"
)

// mobiTextReader 用 go-fitz(MuPDF) 提取 mobi 文本，按 MuPDF reflow 分页逐页提供纯文本。
type mobiTextReader struct {
	doc *fitz.Document
	n   int
}

// GetContent 返回第 index 个 MuPDF reflow 页的纯文本（无 HTML 标签）。
// 有声书流水线会再经 htmlToText 透传、CleanBookText 去噪，因此对纯文本友好。
func (r *mobiTextReader) GetContent(index int) ([]byte, error) {
	if index < 0 || index >= r.n {
		return nil, fmt.Errorf("mobi page %d out of range", index)
	}
	t, err := r.doc.Text(index)
	if err != nil {
		return nil, err
	}
	return []byte(t), nil
}

// Close 关闭底层 MuPDF 文档。
func (r *mobiTextReader) Close() {
	r.doc.Close()
}

// openMobiReaderImpl 用 go-fitz 打开 mobi。
func openMobiReaderImpl(path string) (mobiReaderIface, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, err
	}
	return &mobiTextReader{doc: doc, n: doc.NumPage()}, nil
}

// extractMobiMetadataImpl 用 go-fitz 提取 mobi 元数据与封面（第 0 页即封面）。
func extractMobiMetadataImpl(path string) (*BookMetadata, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, err
	}
	defer doc.Close()

	meta := &BookMetadata{}
	md := doc.Metadata()
	if v := md["title"]; v != "" {
		meta.Title = strings.TrimSpace(v)
	}
	if v := md["author"]; v != "" {
		meta.Authors = splitAuthor(v)
	} else if v := md["creator"]; v != "" {
		meta.Authors = splitAuthor(v)
	}
	if v := md["language"]; v != "" {
		meta.Language = strings.TrimSpace(v)
	}
	if v := md["subject"]; v != "" {
		meta.Tags = splitTags(v)
	}
	if v := md["description"]; v != "" {
		meta.Description = strings.TrimSpace(v)
	}
	if v := md["producer"]; v != "" {
		meta.Publisher = strings.TrimSpace(v)
	}
	if meta.Title == "" {
		base := filepath.Base(path)
		meta.Title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if cd, cerr := doc.ImagePNG(0, 150); cerr == nil && len(cd) > 0 {
		meta.CoverData = cd
	}
	return meta, nil
}
