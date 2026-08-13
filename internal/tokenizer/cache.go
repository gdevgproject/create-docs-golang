package tokenizer

import (
	"container/list"
	"strings"
	"sync"
)

type countCacheEntry struct {
	text  string
	count int
	size  int
}

type countCache struct {
	mu         sync.Mutex
	entries    map[string]*list.Element
	recency    *list.List
	bytes      int
	maxBytes   int
	maxEntries int
}

func newCountCache(maxBytes, maxEntries int) *countCache {
	return &countCache{
		entries:    make(map[string]*list.Element),
		recency:    list.New(),
		maxBytes:   maxBytes,
		maxEntries: maxEntries,
	}
}

func (cache *countCache) get(text string) (int, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element, exists := cache.entries[text]
	if !exists {
		return 0, false
	}
	cache.recency.MoveToFront(element)
	return element.Value.(countCacheEntry).count, true
}

func (cache *countCache) add(text string, count int) {
	if cache.maxBytes <= 0 || cache.maxEntries <= 0 || len(text) > cache.maxBytes {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if element, exists := cache.entries[text]; exists {
		entry := element.Value.(countCacheEntry)
		entry.count = count
		element.Value = entry
		cache.recency.MoveToFront(element)
		return
	}

	ownedText := strings.Clone(text)
	entry := countCacheEntry{text: ownedText, count: count, size: len(ownedText)}
	element := cache.recency.PushFront(entry)
	cache.entries[ownedText] = element
	cache.bytes += entry.size

	for cache.bytes > cache.maxBytes || cache.recency.Len() > cache.maxEntries {
		oldest := cache.recency.Back()
		if oldest == nil {
			break
		}
		oldEntry := oldest.Value.(countCacheEntry)
		delete(cache.entries, oldEntry.text)
		cache.bytes -= oldEntry.size
		cache.recency.Remove(oldest)
	}
}

func (cache *countCache) clear() {
	cache.mu.Lock()
	cache.entries = make(map[string]*list.Element)
	cache.recency.Init()
	cache.bytes = 0
	cache.mu.Unlock()
}
