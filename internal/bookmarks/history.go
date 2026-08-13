package bookmarks

import (
	"sort"
	"strings"
	"time"
)

func appendOrUpdateHistory(history []LastResult, result LastResult) []LastResult {
	if strings.TrimSpace(result.GeneratedAt) == "" {
		result.GeneratedAt = time.Now().Format("2006-01-02 15:04:05.000 (-07:00)")
	}

	foundIndex := -1
	for index, existing := range history {
		if strings.TrimSpace(existing.GeneratedAt) == strings.TrimSpace(result.GeneratedAt) {
			foundIndex = index
			break
		}
	}
	if foundIndex >= 0 {
		history[foundIndex] = result
	} else {
		history = append(history, result)
	}
	sortHistory(history)
	return history
}

func sortHistory(history []LastResult) {
	sort.SliceStable(history, func(left, right int) bool {
		return history[left].GeneratedAt > history[right].GeneratedAt
	})
}

func syncLastResult(bookmark *Bookmark) {
	sortHistory(bookmark.History)
	if len(bookmark.History) == 0 {
		bookmark.History = []LastResult{}
		bookmark.LastResult = nil
		return
	}
	latest := bookmark.History[0]
	bookmark.LastResult = &latest
}
