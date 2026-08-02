package config

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	// Version can be set at build time via -ldflags "-X codedocs/internal/config.Version=v1.0.4"
	Version    = "v1.0.4"
	GitHubRepo = "gdevgproject/create-docs-golang"
)

const (
	O200KVocabURL    = "https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken"
	O200KVocabSHA256 = "446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d"
	O200KPattern     = `(?i:'s|'t|'re|'ve|'m|'ll|'d)|[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]*[\p{Ll}\p{Lm}\p{Lo}\p{M}]+(?i:'s|'t|'re|'ve|'m|'ll|'d)?|[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]*[\p{Ll}\p{Lm}\p{Lo}\p{M}]+(?i:'s|'t|'re|'ve|'m|'ll|'d)?|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n/]*|\s*[\r\n]+|\s+(?!\S)|\s+`
)

var ExcludedDirs = []string{
	"node_modules",
	".next",
	"vendor",
	".git",
	".idea",
	".vscode",
	"public",
	"dist",
	"build",
	"out",
	"storage",
	"coverage",
	"__pycache__",
	"tmp",
	"temp",
	"logs",
}

var ExcludedFiles = []string{
	".env",
	".env.local",
	".env.example",
	".env.production",
	"package-lock.json",
	"composer.lock",
	"yarn.lock",
	"pnpm-lock.yaml",
	".DS_Store",
	"Thumbs.db",
	"desktop.ini",
	"tsconfig.tsbuildinfo",
	"mix-manifest.json",
	"manifest.json",
}

var BinaryExtensions = []string{
	"png", "jpg", "jpeg", "gif", "bmp", "svg", "webp", "ico", "tif", "tiff",
	"mp3", "wav", "ogg", "mp4", "mov", "avi", "webm", "mkv",
	"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx",
	"zip", "rar", "7z", "tar", "gz", "iso",
	"ttf", "otf", "woff", "woff2", "eot",
	"exe", "dll", "so", "dylib", "class", "jar", "phar", "bin", "obj", "pyc",
}

// Config holds runtime configuration settings
type Config struct {
	Version      string
	Port         int
	Host         string
	BasePath     string // Custom URL prefix, e.g. "/codedocs" -> http://localhost:8080/codedocs
	MaxFileSize  int64
	Workers      int
	TempDir      string
	CacheDir     string
	BookmarkFile string
	OpenBrowser  bool
}

// NormalizeBasePath normalizes base path to start with / and end without /
func NormalizeBasePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimSuffix(p, "/")
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	userCache, err := os.UserCacheDir()
	if err != nil {
		userCache = "."
	}
	cacheDir := filepath.Join(userCache, "codedocs")

	userConfig, err := os.UserConfigDir()
	if err != nil {
		userConfig = "."
	}
	bookmarkFile := filepath.Join(userConfig, "codedocs", "saved_paths.json")

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 4
	}

	return &Config{
		Version:      Version,
		Port:         8080,
		Host:         "0.0.0.0",
		BasePath:     "",
		MaxFileSize:  10485760, // 10MB
		Workers:      workers,
		TempDir:      "./temp_docs",
		CacheDir:     cacheDir,
		BookmarkFile: bookmarkFile,
		OpenBrowser:  true,
	}
}

// ParseFlags loads configuration from CLI flags
func ParseFlags() *Config {
	cfg := DefaultConfig()

	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	flag.StringVar(&cfg.Host, "host", cfg.Host, "HTTP server host")
	flag.StringVar(&cfg.BasePath, "base-path", cfg.BasePath, "Custom URL path prefix (e.g. /codedocs or /docs)")
	flag.Int64Var(&cfg.MaxFileSize, "max-size", cfg.MaxFileSize, "Max file size in bytes to include content (default 10MB)")
	flag.IntVar(&cfg.Workers, "workers", cfg.Workers, "Worker pool concurrency size")
	flag.StringVar(&cfg.TempDir, "temp-dir", cfg.TempDir, "Directory to store generated documentation files")
	flag.StringVar(&cfg.CacheDir, "cache-dir", cfg.CacheDir, "Directory to cache tokenizer vocabulary")
	flag.StringVar(&cfg.BookmarkFile, "bookmark-file", cfg.BookmarkFile, "Path to bookmarks JSON file")
	flag.BoolVar(&cfg.OpenBrowser, "open-browser", cfg.OpenBrowser, "Automatically open default browser on launch")

	flag.Parse()
	cfg.BasePath = NormalizeBasePath(cfg.BasePath)
	return cfg
}
