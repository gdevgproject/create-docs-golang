package scanner

import (
	"context"
	"errors"
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
	for _, dir := range config.ExcludedDirs {
		dirMap[strings.ToLower(filepath.ToSlash(dir))] = struct{}{}
	}

	fileMap := make(map[string]struct{}, len(config.ExcludedFiles))
	for _, file := range config.ExcludedFiles {
		fileMap[strings.ToLower(file)] = struct{}{}
	}

	binaryMap := make(map[string]struct{}, len(config.BinaryExtensions))
	for _, extension := range config.BinaryExtensions {
		binaryMap[strings.ToLower(extension)] = struct{}{}
	}

	return &Scanner{
		excludedDirs:  dirMap,
		excludedFiles: fileMap,
		binaryExts:    binaryMap,
	}
}

// IsBinary reports whether an extension belongs to a known binary, media, or
// generated asset format.
func (s *Scanner) IsBinary(extension string) bool {
	cleanExtension := strings.ToLower(strings.TrimPrefix(extension, "."))
	_, exists := s.binaryExts[cleanExtension]
	return exists
}

// ScanProjectFiles recursively discovers non-excluded project files.
func (s *Scanner) ScanProjectFiles(rootPath string) ([]string, error) {
	return s.ScanProjectFilesContext(context.Background(), rootPath)
}

// ScanProjectFilesContext is the cancellable form used by interactive scans.
// It evaluates root and nested .gitignore files in their directory scope and
// never follows symbolic links or Windows junction-like reparse entries.
func (s *Scanner) ScanProjectFilesContext(ctx context.Context, rootPath string) ([]string, error) {
	cleanRoot, err := filepath.Abs(filepath.Clean(rootPath))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(cleanRoot)
	if err != nil || !info.IsDir() {
		return nil, os.ErrNotExist
	}

	rootIgnore := loadIgnoreLayer(nil, cleanRoot, cleanRoot)
	ignoreByDir := map[string]*ignoreLayer{cleanRoot: rootIgnore}
	files := make([]string, 0, 256)

	err = filepath.WalkDir(cleanRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if path == cleanRoot {
				return walkErr
			}
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if path == cleanRoot {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(cleanRoot, path)
		if err != nil || relPath == "." || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
			return nil
		}
		relSlash := filepath.ToSlash(relPath)
		relLower := strings.ToLower(relSlash)
		nameLower := strings.ToLower(entry.Name())
		parentIgnore := ignoreByDir[filepath.Dir(path)]

		if entry.IsDir() {
			if s.isExcludedDirectory(nameLower, relLower) || parentIgnore.ignored(relSlash, true) {
				return filepath.SkipDir
			}
			ignoreByDir[path] = loadIgnoreLayer(parentIgnore, cleanRoot, path)
			return nil
		}

		if _, excluded := s.excludedFiles[nameLower]; excluded {
			return nil
		}
		if parentIgnore.ignored(relSlash, false) {
			return nil
		}

		files = append(files, filepath.ToSlash(path))
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func (s *Scanner) isExcludedDirectory(nameLower, relLower string) bool {
	if _, excluded := s.excludedDirs[nameLower]; excluded {
		return true
	}
	_, excluded := s.excludedDirs[relLower]
	return excluded
}
