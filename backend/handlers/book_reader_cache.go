package handlers

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// 章节内容/资源/文本提取此前每次请求都重新解析整本 EPUB/AZW3，
// 连续朗读或翻页时开销明显。这里按"路径 + 格式 + 文件指纹"缓存阅读器实例，
// 文件被替换后指纹变化，旧实例会在 LRU 淘汰时关闭，不会读到旧内容。
const maxCachedBookReaders = 8

type cachedBookReader struct {
	reader   bookReader
	lastUsed time.Time
	inUse    int
	mu       sync.Mutex
}

var (
	cachedBookReaders   = map[string]*cachedBookReader{}
	cachedBookReadersMu sync.Mutex
)

func bookReaderStamp(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "missing"
	}
	return fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
}

func withCachedBookReader(filePath, fileType string, fn func(bookReader) error) error {
	key := filePath + "\x00" + fileType + "\x00" + bookReaderStamp(filePath)

	cachedBookReadersMu.Lock()
	entry := cachedBookReaders[key]
	if entry == nil {
		reader, err := openBookReader(filePath, fileType)
		if err != nil {
			cachedBookReadersMu.Unlock()
			return err
		}
		entry = &cachedBookReader{reader: reader, lastUsed: time.Now()}
		cachedBookReaders[key] = entry
		evictBookReadersLocked()
	}
	entry.lastUsed = time.Now()
	entry.inUse++
	cachedBookReadersMu.Unlock()

	// 同一个阅读器实例可能被并发请求复用，串行化调用避免 zip/pdb 内部状态竞争。
	entry.mu.Lock()
	err := fn(entry.reader)
	entry.mu.Unlock()

	cachedBookReadersMu.Lock()
	entry.inUse--
	cachedBookReadersMu.Unlock()
	return err
}

func evictBookReadersLocked() {
	for len(cachedBookReaders) > maxCachedBookReaders {
		var oldestKey string
		var oldest time.Time
		found := false
		for key, entry := range cachedBookReaders {
			if entry.inUse > 0 {
				continue
			}
			if !found || entry.lastUsed.Before(oldest) {
				oldestKey = key
				oldest = entry.lastUsed
				found = true
			}
		}
		if !found {
			return
		}
		cachedBookReaders[oldestKey].reader.Close()
		delete(cachedBookReaders, oldestKey)
	}
}
