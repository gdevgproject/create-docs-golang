package scanner

import (
	"path/filepath"
	"sort"
	"strings"
)

type treeNode map[string]treeNode

// GenerateDirectoryTree generates an ASCII directory tree string matching tree command style
func GenerateDirectoryTree(rootPath string, files []string) string {
	cleanRoot := filepath.Clean(filepath.FromSlash(rootPath))

	rootNode := make(treeNode)

	for _, file := range files {
		relPath, err := filepath.Rel(cleanRoot, filepath.Clean(filepath.FromSlash(file)))
		if err != nil || relPath == "." || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
			continue
		}
		relPath = filepath.ToSlash(relPath)
		parts := strings.Split(relPath, "/")
		curr := rootNode
		for _, part := range parts {
			if _, exists := curr[part]; !exists {
				curr[part] = make(treeNode)
			}
			curr = curr[part]
		}
	}

	var sb strings.Builder
	rootName := filepath.Base(cleanRoot)
	if rootName == "" || rootName == "." || rootName == "/" {
		rootName = cleanRoot
	}
	sb.WriteString(rootName + "/\n")

	printTree(rootNode, &sb, "")
	return sb.String()
}

func printTree(node treeNode, sb *strings.Builder, prefix string) {
	keys := make([]string, 0, len(node))
	for k := range node {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	lastIdx := len(keys) - 1
	for i, key := range keys {
		isLast := i == lastIdx
		marker := "├── "
		subPrefix := "│   "
		if isLast {
			marker = "└── "
			subPrefix = "    "
		}

		sb.WriteString(prefix + marker + key + "\n")
		if len(node[key]) > 0 {
			printTree(node[key], sb, prefix+subPrefix)
		}
	}
}
