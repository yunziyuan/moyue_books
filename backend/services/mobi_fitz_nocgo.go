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

// extractMobiTOCImpl 用 go-fitz 的 ToC（MuPDF 大纲）提取目录。
// Outline.Page 为 0 基页号，与 mobiTextReader.GetContent 的页序号一致，
// 有声书/阅读器可直接按 Index 定位；Page 字段按项目约定存 1 基页码。
func extractMobiTOCImpl(path string) ([]TOCItem, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, err
	}
	defer doc.Close()

	outline, err := doc.ToC()
	if err != nil || len(outline) == 0 {
		return nil, err
	}

	// MuPDF 对 reflow 文档（mobi/epub）的大纲页号按目标文件位置解析，而节起点
	// 常落在页中、标题实际渲染在下一页，导致大纲页号系统性提前一页（实测
	// 399洞穴奇案.mobi：大纲"导读（一）"=页4，实际标题在页5）。这里用
	// "标题文本是否出现在该页"做校正：先查 P，再查 P+1；都查不到则保持原
	// 页号并强制页号单调递增（章节按出现顺序排列）。
	n := doc.NumPage()
	pageText := map[int]string{}
	textAt := func(p int) string {
		if p < 0 || p >= n {
			return ""
		}
		if t, ok := pageText[p]; ok {
			return t
		}
		t, err := doc.Text(p)
		if err != nil {
			t = ""
		}
		pageText[p] = t
		return t
	}
	// 去除空白后比较：正文渲染会在标题中插入换行/空格（如"观点三 法律与道德的两难"）
	squeeze := func(s string) string { return strings.Join(strings.Fields(s), "") }
	contains := func(p int, title string) bool {
		return title != "" && strings.Contains(squeeze(textAt(p)), squeeze(title))
	}

	items := make([]TOCItem, 0, len(outline))
	prev := -1
	for _, o := range outline {
		title := strings.TrimSpace(o.Title)
		if title == "" || o.Page < 0 {
			continue // 无标题或外链条目跳过
		}
		pg := o.Page
		if !contains(pg, title) && contains(pg+1, title) {
			pg++
		}
		if pg <= prev {
			pg = prev + 1
		}
		if pg >= n {
			break // 超出末页的条目丢弃
		}
		prev = pg
		items = append(items, TOCItem{
			Index: pg,
			Title: title,
			Level: o.Level,
			Page:  pg + 1,
		})
	}
	return items, nil
}
