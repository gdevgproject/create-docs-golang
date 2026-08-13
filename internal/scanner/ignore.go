package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type ignoreLayer struct {
	parent *ignoreLayer
	base   string
	rules  []ignoreRule
}

type ignoreRule struct {
	negated       bool
	directoryOnly bool
	exact         *regexp.Regexp
	descendant    *regexp.Regexp
}

func loadIgnoreLayer(parent *ignoreLayer, root, directory string) *ignoreLayer {
	file, err := os.Open(filepath.Join(directory, ".gitignore"))
	if err != nil {
		return parent
	}
	defer file.Close()

	base, err := filepath.Rel(root, directory)
	if err != nil || base == "." {
		base = ""
	} else {
		base = filepath.ToSlash(base)
	}

	layer := &ignoreLayer{parent: parent, base: base}
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 4*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		if rule, ok := compileIgnoreRule(scanner.Text()); ok {
			layer.rules = append(layer.rules, rule)
		}
	}
	if len(layer.rules) == 0 {
		return parent
	}
	return layer
}

func (layer *ignoreLayer) ignored(rootRelativePath string, isDirectory bool) bool {
	if layer == nil {
		return false
	}
	ignored := false
	if layer.parent != nil {
		ignored = layer.parent.ignored(rootRelativePath, isDirectory)
	}

	localPath := strings.TrimPrefix(filepath.ToSlash(rootRelativePath), "./")
	if layer.base != "" {
		prefix := strings.TrimSuffix(layer.base, "/") + "/"
		if !strings.HasPrefix(localPath, prefix) {
			return ignored
		}
		localPath = strings.TrimPrefix(localPath, prefix)
	}

	for _, rule := range layer.rules {
		if rule.matches(localPath, isDirectory) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func (rule ignoreRule) matches(path string, isDirectory bool) bool {
	if rule.exact.MatchString(path) && (!rule.directoryOnly || isDirectory) {
		return true
	}
	// A negation only re-includes the path it names. Letting it match every
	// descendant would incorrectly turn `!build/` into `!build/**`.
	if rule.negated {
		return false
	}
	return rule.descendant.MatchString(path)
}

func compileIgnoreRule(line string) (ignoreRule, bool) {
	line = strings.TrimSuffix(line, "\r")
	line = trimUnescapedTrailingSpaces(line)
	if line == "" {
		return ignoreRule{}, false
	}

	if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
		line = line[1:]
	} else if strings.HasPrefix(line, "#") {
		return ignoreRule{}, false
	}

	negated := strings.HasPrefix(line, "!")
	if negated {
		line = strings.TrimPrefix(line, "!")
	}
	directoryOnly := strings.HasSuffix(line, "/")
	line = strings.TrimSuffix(line, "/")
	anchored := strings.HasPrefix(line, "/")
	line = strings.TrimPrefix(line, "/")
	line = filepath.ToSlash(line)
	if line == "" {
		return ignoreRule{}, false
	}

	pattern := globToRegexp(line)
	if anchored || strings.Contains(line, "/") {
		pattern = "^" + pattern
	} else {
		pattern = `(?:^|/)` + pattern
	}
	flags := ""
	if runtime.GOOS == "windows" {
		flags = "(?i)"
	}

	exact, err := regexp.Compile(flags + pattern + "$")
	if err != nil {
		return ignoreRule{}, false
	}
	descendant, err := regexp.Compile(flags + pattern + `/.*$`)
	if err != nil {
		return ignoreRule{}, false
	}

	return ignoreRule{
		negated:       negated,
		directoryOnly: directoryOnly,
		exact:         exact,
		descendant:    descendant,
	}, true
}

func trimUnescapedTrailingSpaces(value string) string {
	for strings.HasSuffix(value, " ") {
		backslashes := 0
		for i := len(value) - 2; i >= 0 && value[i] == '\\'; i-- {
			backslashes++
		}
		if backslashes%2 == 1 {
			value = value[:len(value)-2] + " "
			break
		}
		value = strings.TrimSuffix(value, " ")
	}
	return value
}

func globToRegexp(pattern string) string {
	var builder strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					builder.WriteString(`(?:.*/)?`)
				} else {
					builder.WriteString(`.*`)
				}
			} else {
				builder.WriteString(`[^/]*`)
			}
		case '?':
			builder.WriteString(`[^/]`)
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 0 {
				builder.WriteString(`\[`)
				continue
			}
			end += i + 1
			class := pattern[i+1 : end]
			if strings.HasPrefix(class, "!") {
				class = "^" + strings.TrimPrefix(class, "!")
			}
			builder.WriteByte('[')
			builder.WriteString(class)
			builder.WriteByte(']')
			i = end
		case '\\':
			if i+1 < len(pattern) {
				i++
				builder.WriteString(regexp.QuoteMeta(string(pattern[i])))
			} else {
				builder.WriteString(`\\`)
			}
		default:
			builder.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	return builder.String()
}
