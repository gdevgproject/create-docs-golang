package tokenizer

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"unicode"

	"codedocs/internal/config"

	"github.com/pkoukk/tiktoken-go"
)

const (
	defaultCountCacheBytes   = 8 << 20
	defaultCountCacheEntries = 256
	maxCachedTextBytes       = 128 << 10
)

type Tokenizer struct {
	cacheDir string
	mode     string
	t        *tiktoken.Tiktoken
	initOnce sync.Once
	initErr  error
	cache    *countCache

	vocabURL     string
	expectedSHA  string
	minVocabSize int64
	maxVocabSize int64
	httpClient   httpDoer
}

var sharedEncoding struct {
	sync.RWMutex
	value *tiktoken.Tiktoken
}

func NewTokenizer(cacheDir string) *Tokenizer {
	if cacheDir == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			userCache = "."
		}
		cacheDir = filepath.Join(userCache, "codedocs")
	}
	return &Tokenizer{
		cacheDir:     cacheDir,
		mode:         "estimate",
		cache:        newCountCache(defaultCountCacheBytes, defaultCountCacheEntries),
		vocabURL:     config.O200KVocabURL,
		expectedSHA:  config.O200KVocabSHA256,
		minVocabSize: 1_000_000,
		maxVocabSize: 20 << 20,
		httpClient:   newVocabHTTPClient(),
	}
}

// NewEstimator creates a deterministic offline tokenizer. It is useful for
// callers that prefer instant heuristic results over network setup.
func NewEstimator() *Tokenizer {
	tok := NewTokenizer("")
	tok.initOnce.Do(func() {})
	return tok
}

// Mode returns "exact" when the verified o200k_base encoder is ready.
func (tok *Tokenizer) Mode() string {
	tok.ensureInitialized()
	return tok.mode
}

// InitializationError reports why exact mode could not be initialized.
func (tok *Tokenizer) InitializationError() error {
	tok.ensureInitialized()
	return tok.initErr
}

func (tok *Tokenizer) ensureInitialized() {
	tok.initOnce.Do(tok.init)
}

func (tok *Tokenizer) init() {
	sharedEncoding.RLock()
	shared := sharedEncoding.value
	sharedEncoding.RUnlock()
	if shared != nil {
		tok.t = shared
		tok.mode = "exact"
		return
	}

	vocabFile, err := tok.EnsureVocabFile()
	if err == nil {
		var ranks map[string]int
		ranks, err = tok.loadTiktokenRanks(vocabFile)
		if err == nil && len(ranks) > 100_000 {
			specialTokens := map[string]int{
				"<|endoftext|>":   199999,
				"<|endofprompt|>": 200018,
			}
			var coreBPE *tiktoken.CoreBPE
			coreBPE, err = tiktoken.NewCoreBPE(ranks, specialTokens, config.O200KPattern)
			if err == nil && coreBPE != nil {
				specialSet := make(map[string]any, len(specialTokens))
				for token := range specialTokens {
					specialSet[token] = true
				}
				tok.t = tiktoken.NewTiktoken(coreBPE, nil, specialSet)
				tok.mode = "exact"
				tok.publishSharedEncoding()
				return
			}
		}
	}

	// The upstream library may already have a valid encoding cache. This keeps
	// exact mode available during a transient failure of our dedicated cache.
	if encoding, fallbackErr := tiktoken.GetEncoding("o200k_base"); fallbackErr == nil && encoding != nil {
		tok.t = encoding
		tok.mode = "exact"
		tok.initErr = nil
		tok.publishSharedEncoding()
		return
	}

	tok.mode = "estimate"
	tok.initErr = err
}

func (tok *Tokenizer) publishSharedEncoding() {
	sharedEncoding.Lock()
	if sharedEncoding.value == nil {
		sharedEncoding.value = tok.t
	}
	sharedEncoding.Unlock()
}

// CountTokens returns an exact or estimated count and caches only reasonably
// sized interactive inputs in a bounded, collision-free LRU.
func (tok *Tokenizer) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	if count, ok := tok.cache.get(text); ok {
		return count
	}
	count := tok.CountTokensUncached(text)
	if len(text) <= maxCachedTextBytes {
		tok.cache.add(text, count)
	}
	return count
}

// CountTokensUncached avoids retaining one-off project files while preserving
// exactly the same tokenization result.
func (tok *Tokenizer) CountTokensUncached(text string) int {
	if text == "" {
		return 0
	}
	tok.ensureInitialized()
	if tok.mode == "exact" && tok.t != nil {
		return len(tok.t.Encode(text, nil, nil))
	}
	return tok.EstimateTokensHeuristic(text)
}

var heuristicPattern = regexp.MustCompile("(?i:[sdmt]|ll|ve|re)|[^\\r\\n\\p{L}\\p{N}]?\\p{L}+|\\p{N}{1,3}| ?[^\\s\\p{L}\\p{N}]+[\\r\\n]*|\\s*[\\r\\n]+|\\s+")

// EstimateTokensHeuristic estimates token count when exact vocabulary setup is
// unavailable. It intentionally favors a slight over-estimate for code.
func (tok *Tokenizer) EstimateTokensHeuristic(content string) int {
	if content == "" {
		return 0
	}
	matches := heuristicPattern.FindAllString(content, -1)
	if len(matches) == 0 {
		return int(math.Ceil(float64(len(content)) / 4.0))
	}

	tokens := 0
	for _, match := range matches {
		length := len(match)
		if length == 0 {
			continue
		}
		hasLetter := false
		for _, current := range match {
			if unicode.IsLetter(current) {
				hasLetter = true
				break
			}
		}

		switch {
		case hasLetter && length <= 7:
			tokens++
		case hasLetter:
			tokens += 1 + int(math.Ceil(float64(length-7)/6.0))
		case unicode.IsDigit(rune(match[0])):
			tokens++
		case match[0] == ' ' || match[0] == '\t' || match[0] == '\n' || match[0] == '\r':
			tokens += 1 + max(0, int(math.Ceil(float64(length-4)/4.0)))
		case length <= 2:
			tokens++
		default:
			tokens += int(math.Ceil(float64(length) / 2.0))
		}
	}
	return tokens
}

func (tok *Tokenizer) ClearCache() {
	tok.cache.clear()
}
