package bookmarks

type LastResult struct {
	Label       string  `json:"label,omitempty"`
	FileName    string  `json:"file_name"`
	TotalFiles  int     `json:"total"`
	TotalLines  int64   `json:"lines"`
	TotalTokens int64   `json:"tokens"`
	TokenMode   string  `json:"token_mode"`
	SizeBytes   int64   `json:"size"`
	Elapsed     float64 `json:"elapsed"`
	GeneratedAt string  `json:"generated_at"`
}

type Bookmark struct {
	ID         string       `json:"id"`
	Path       string       `json:"path"`
	Note       string       `json:"note"`
	Order      int          `json:"order"`
	CreatedAt  string       `json:"created_at"`
	LastResult *LastResult  `json:"last_result,omitempty"`
	History    []LastResult `json:"history,omitempty"`
}
