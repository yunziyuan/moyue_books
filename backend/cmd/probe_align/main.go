package main

import (
	"fmt"
	"strings"

	"github.com/gen2brain/go-fitz"
)

func main() {
	path := "/Users/apple/Downloads/399洞穴奇案.mobi"
	doc, err := fitz.New(path)
	if err != nil {
		fmt.Println("open err:", err)
		return
	}
	defer doc.Close()

	outline, err := doc.ToC()
	if err == nil {
		fmt.Println("=== 大纲（0基页号） ===")
		for i, o := range outline {
			if i >= 12 {
				break
			}
			fmt.Printf("  page=%d L%d %s\n", o.Page, o.Level, o.Title)
		}
	}

	fmt.Println("=== 各页文本开头（0基页号） ===")
	n := doc.NumPage()
	for i := 0; i < 12 && i < n; i++ {
		t, err := doc.Text(i)
		if err != nil {
			continue
		}
		t = strings.TrimSpace(strings.ReplaceAll(t, "\n", " "))
		if len([]rune(t)) > 55 {
			t = string([]rune(t)[:55])
		}
		fmt.Printf("  page=%d: %s\n", i, t)
	}
}
