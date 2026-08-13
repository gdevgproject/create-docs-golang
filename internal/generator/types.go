package generator

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"codedocs/internal/config"
	"codedocs/internal/scanner"
	"codedocs/internal/tokenizer"
)

const defaultBufferCapacity = 512 * 1024

var unsafeFilenameCharacters = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

type ProgressEvent struct {
	Type    string         `json:"type"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

type GenerateResult struct {
	FileName        string  `json:"file_name"`
	FilePath        string  `json:"file_path"`
	TotalFiles      int     `json:"total"`
	TotalLines      int64   `json:"lines"`
	TotalTokens     int64   `json:"tokens"`
	TokenMode       string  `json:"token_mode"`
	Elapsed         float64 `json:"elapsed"`
	SizeBytes       int64   `json:"size"`
	Mode            string  `json:"mode"`
	GeneratedAt     string  `json:"generated_at"`
	BinaryFiles     int     `json:"binary_files,omitempty"`
	SkippedFiles    int     `json:"skipped_files,omitempty"`
	UnreadableFiles int     `json:"unreadable_files,omitempty"`
}

type Generator struct {
	cfg       *config.Config
	sc        *scanner.Scanner
	tok       *tokenizer.Tokenizer
	bufferCap int
}

func NewGenerator(cfg *config.Config, projectScanner *scanner.Scanner, tok *tokenizer.Tokenizer) *Generator {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if projectScanner == nil {
		projectScanner = scanner.NewScanner()
	}
	if tok == nil {
		tok = tokenizer.NewTokenizer(cfg.CacheDir)
	}
	return &Generator{cfg: cfg, sc: projectScanner, tok: tok, bufferCap: defaultBufferCapacity}
}

type job struct {
	index       int
	path        string
	rel         string
	memoryBytes int64
}

type result struct {
	index       int
	rel         string
	chunk       []byte
	lines       int64
	tokens      int64
	memoryBytes int64
	binary      bool
	skipped     bool
	failed      bool
}

type aggregateStats struct {
	lines   int64
	tokens  int64
	binary  int
	skipped int
	failed  int
}

func outputFileName(projectName string, timestamp time.Time) string {
	cleanName := strings.Trim(unsafeFilenameCharacters.ReplaceAllString(projectName, "_"), "_")
	if cleanName == "" {
		var builder strings.Builder
		for _, current := range projectName {
			if unicode.IsLetter(current) || unicode.IsDigit(current) {
				builder.WriteRune(current)
			} else if builder.Len() > 0 {
				builder.WriteByte('_')
			}
		}
		cleanName = strings.Trim(builder.String(), "_")
	}
	if cleanName == "" {
		cleanName = "project"
	}
	cleanRunes := []rune(cleanName)
	if len(cleanRunes) > 80 {
		cleanName = string(cleanRunes[:80])
	}
	return "docs_" + cleanName + "_" + timestamp.Format("20060102_150405.000000000") + ".md"
}
