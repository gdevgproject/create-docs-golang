package config

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	// Version can be set at build time via -ldflags "-X codedocs/internal/config.Version=v1.5.2"
	Version    = "v1.5.2"
	GitHubRepo = "gdevgproject/create-docs-golang"
)

const (
	O200KVocabURL    = "https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken"
	O200KVocabSHA256 = "446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d"
	O200KPattern     = `(?i:'s|'t|'re|'ve|'m|'ll|'d)|[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]*[\p{Ll}\p{Lm}\p{Lo}\p{M}]+(?i:'s|'t|'re|'ve|'m|'ll|'d)?|[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]*[\p{Ll}\p{Lm}\p{Lo}\p{M}]+(?i:'s|'t|'re|'ve|'m|'ll|'d)?|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n/]*|\s*[\r\n]+|\s+(?!\S)|\s+`
)

// ExcludedDirs contains comprehensive directory exclusions across all modern tech stacks
// (Node, Next, Nest, React, Java, Spring, Kotlin, Android, Go, Python, C#/.NET, PHP/Laravel, Rust, Game Engines, etc.)
var ExcludedDirs = []string{
	// JavaScript / TypeScript / Frontend / Mobile Frameworks
	"node_modules",
	".next",
	".nuxt",
	".svelte-kit",
	".output",
	".turbo",
	".cache",
	"coverage",
	".angular",
	".pnpm-store",
	".npm",
	".yarn",
	"android/build",
	"ios/Pods",
	".expo",
	".dart_tool",
	"public",
	"dist",
	"build",
	"out",

	// Java / Kotlin / Spring Boot / Gradle / Maven / Minecraft Mods
	"target",
	".gradle",
	".mvn",
	"bin",
	".fatjar",
	".apt_generated",

	// Python / Django / FastAPI / Flask
	"__pycache__",
	".pytest_cache",
	".venv",
	"venv",
	"env",
	".env",
	".tox",
	".mypy_cache",
	".ruff_cache",
	".eggs",
	"htmlcov",

	// Go (Golang)
	"vendor",
	"cgo",
	".bin",

	// C# / .NET / Unity / Unreal Engine / C++ Game Engines
	"obj",
	".vs",
	".idea",
	".vscode",
	"packages",
	"Library",
	"Temp",
	"Logs",
	"Builds",
	"DerivedDataCache",
	"Intermediate",
	"Saved",
	"Binaries",
	"Plugins/Developer",

	// PHP / Laravel / Symfony / WordPress
	"storage",
	"storage/framework",
	"storage/logs",
	"bootstrap/cache",
	".phpunit.cache",

	// Rust / Cargo / Ruby
	"vendor/bundle",
	".bundle",

	// DevOps / Infrastructure
	".terraform",
	".terragrunt-cache",
	".serverless",

	// Version Control & System Trashes
	".git",
	".svn",
	".hg",
	"tmp",
	"temp",
	"logs",
	".sass-cache",
}

// ExcludedFiles contains lockfiles, environment secrets, and IDE caches
var ExcludedFiles = []string{
	".env",
	".env.local",
	".env.example",
	".env.production",
	".env.staging",
	".env.test",
	"package-lock.json",
	"composer.lock",
	"yarn.lock",
	"pnpm-lock.yaml",
	"Cargo.lock",
	"Gemfile.lock",
	"poetry.lock",
	"mix.lock",
	"pubspec.lock",
	".DS_Store",
	"Thumbs.db",
	"desktop.ini",
	"tsconfig.tsbuildinfo",
	"mix-manifest.json",
	"manifest.json",
	"vcpkg.json",
}

// BinaryExtensions contains all binary, media, asset, and compiled file extensions
var BinaryExtensions = []string{
	// Compiled Executables & Libraries
	"exe", "dll", "so", "dylib", "class", "jar", "war", "ear", "phar", "bin", "obj", "pyc", "pyo", "pyd", "o", "a", "lib", "sys", "drv", "nupkg",

	// Game Engine & Assets (Unity, Unreal Engine, Steam Games, Minecraft Mods)
	"pak", "vpk", "bsa", "ba2", "uasset", "umap", "asset", "unitypackage", "nbt", "mca", "gcm", "iso", "rom", "sav", "fbx", "obj", "blend", "gltf", "glb", "3ds", "dae", "max",

	// Graphics & Game Textures
	"tga", "dds", "hdr", "exr", "psd", "ai", "eps", "ktx", "astc", "pvr", "pkm", "ktx2",

	// Images
	"png", "jpg", "jpeg", "gif", "bmp", "svg", "webp", "ico", "tif", "tiff", "avif", "heic",

	// Audio & Sounds
	"mp3", "wav", "ogg", "flac", "aac", "m4a", "wma", "opus", "mid", "midi",

	// Videos
	"mp4", "mov", "avi", "webm", "mkv", "flv", "wmv", "m4v",

	// Archives & Compressed Packages
	"zip", "rar", "7z", "tar", "gz", "bz2", "xz", "tgz", "zst",

	// Fonts
	"ttf", "otf", "woff", "woff2", "eot",

	// Documents & PDFs
	"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx",
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

	// Leave 1 CPU core free for OS responsiveness if CPU count > 4
	workers := runtime.NumCPU()
	if workers > 4 {
		workers = workers - 1
	}
	if workers < 1 {
		workers = 1
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
