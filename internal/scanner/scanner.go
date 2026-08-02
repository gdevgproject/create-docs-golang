package scanner

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codedocs/internal/config"
)

type Scanner struct {
	excludedDirs  map[string]struct{}
	excludedFiles map[string]struct{}
	binaryExts    map[string]struct{}
}

func NewScanner() *Scanner {
	dirMap := make(map[string]struct{}, len(config.ExcludedDirs))
	for _, d := range config.ExcludedDirs {
		dirMap[strings.ToLower(d)] = struct{}{}
	}

	fileMap := make(map[string]struct{}, len(config.ExcludedFiles))
	for _, f := range config.ExcludedFiles {
		fileMap[strings.ToLower(f)] = struct{}{}
	}

	binMap := make(map[string]struct{}, len(config.BinaryExtensions))
	for _, b := range config.BinaryExtensions {
		binMap[strings.ToLower(b)] = struct{}{}
	}

	return &Scanner{
		excludedDirs:  dirMap,
		excludedFiles: fileMap,
		binaryExts:    binMap,
	}
}

// IsBinary returns true if the extension belongs to binary/media files
func (s *Scanner) IsBinary(ext string) bool {
	cleanExt := strings.ToLower(strings.TrimPrefix(ext, "."))
	_, exists := s.binaryExts[cleanExt]
	return exists
}

// ScanProjectFiles recursively finds all non-excluded files under rootPath
func (s *Scanner) ScanProjectFiles(rootPath string) ([]string, error) {
	cleanRoot := filepath.Clean(rootPath)
	info, err := os.Stat(cleanRoot)
	if err != nil || !info.IsDir() {
		return nil, os.ErrNotExist
	}

	var files []string

	err = filepath.WalkDir(cleanRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // Skip unreadable entries
		}

		name := d.Name()
		nameLower := strings.ToLower(name)

		if d.IsDir() {
			if path != cleanRoot {
				if _, excluded := s.excludedDirs[nameLower]; excluded {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if _, excluded := s.excludedFiles[nameLower]; excluded {
			return nil
		}

		// Normalize to forward slashes for cross-platform consistency
		normalizedPath := filepath.ToSlash(path)
		files = append(files, normalizedPath)
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

// CountProjectLinesFast counts lines across files using fast chunked I/O
func (s *Scanner) CountProjectLinesFast(files []string, maxFileSize int64) int64 {
	var totalLines int64

	buf := make([]byte, 65536) // 64KB chunk buffer

	for _, file := range files {
		ext := filepath.Ext(file)
		if s.IsBinary(ext) {
			continue
		}

		info, err := os.Stat(file)
		if err != nil || info.Size() == 0 || (maxFileSize > 0 && info.Size() > maxFileSize) {
			continue
		}

		f, err := os.Open(file)
		if err != nil {
			continue
		}

		var fileLines int64
		for {
			n, err := f.Read(buf)
			if n > 0 {
				fileLines += int64(bytes.Count(buf[:n], []byte("\n")))
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
		f.Close()
		totalLines += fileLines + 1 // Add +1 for last line without newline
	}

	return totalLines
}
