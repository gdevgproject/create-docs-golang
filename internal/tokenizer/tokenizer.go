package tokenizer

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"codedocs/internal/config"

	"github.com/pkoukk/tiktoken-go"
)

type Tokenizer struct {
	cacheDir string
	mode     string // "exact" or "estimate"
	t        *tiktoken.Tiktoken
	memo     sync.Map
	initOnce sync.Once
	mu       sync.Mutex
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
		cacheDir: cacheDir,
		mode:     "estimate",
	}
}

// Mode returns "exact" if o200k_base tiktoken is available, or "estimate" if using heuristic fallback
func (tok *Tokenizer) Mode() string {
	tok.initOnce.Do(func() {
		tok.init()
	})
	return tok.mode
}

// EnsureVocabFile makes sure o200k_base.tiktoken exists and matches SHA-256 checksum
func (tok *Tokenizer) EnsureVocabFile() (string, error) {
	if err := os.MkdirAll(tok.cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	cacheFile := filepath.Join(tok.cacheDir, "o200k_base.tiktoken")
	failFile := filepath.Join(tok.cacheDir, "o200k_fail")

	// Check existing cache file
	if info, err := os.Stat(cacheFile); err == nil && info.Size() > 1000000 {
		if tok.verifySHA256(cacheFile, config.O200KVocabSHA256) {
			return cacheFile, nil
		}
		_ = os.Remove(cacheFile) // Corrupted file, remove to redownload
	}

	// Check recent failure marker (don't retry download within 10 minutes)
	if info, err := os.Stat(failFile); err == nil {
		if time.Since(info.ModTime()) < 10*time.Minute {
			return "", fmt.Errorf("offline mode active due to recent download failure")
		}
	}

	// Download from OpenAI public Blob storage
	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	resp, err := client.Get(config.O200KVocabURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		_ = os.WriteFile(failFile, []byte(fmt.Sprintf("%d", time.Now().Unix())), 0644)
		return "", fmt.Errorf("failed to download vocab: %w", err)
	}
	defer resp.Body.Close()

	tmpFile := cacheFile + ".tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	hasher := sha256.New()
	mw := io.MultiWriter(out, hasher)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		out.Close()
		_ = os.Remove(tmpFile)
		_ = os.WriteFile(failFile, []byte(fmt.Sprintf("%d", time.Now().Unix())), 0644)
		return "", fmt.Errorf("failed to save vocab file: %w", err)
	}
	out.Close()

	computedHash := hex.EncodeToString(hasher.Sum(nil))
	if computedHash != config.O200KVocabSHA256 {
		_ = os.Remove(tmpFile)
		_ = os.WriteFile(failFile, []byte(fmt.Sprintf("%d", time.Now().Unix())), 0644)
		return "", fmt.Errorf("SHA-256 mismatch: expected %s, got %s", config.O200KVocabSHA256, computedHash)
	}

	if err := os.Rename(tmpFile, cacheFile); err != nil {
		_ = os.Remove(tmpFile)
		return "", fmt.Errorf("failed to rename cache file: %w", err)
	}

	_ = os.Remove(failFile)
	return cacheFile, nil
}

func (tok *Tokenizer) verifySHA256(filePath, expectedHash string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == expectedHash
}

// loadTiktokenRanks parses base64 rank file into map[string]int
func (tok *Tokenizer) loadTiktokenRanks(filePath string) (map[string]int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ranks := make(map[string]int, 200000)
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			continue
		}

		rawBytes, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			continue
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		ranks[string(rawBytes)] = rank
	}

	return ranks, scanner.Err()
}

func (tok *Tokenizer) init() {
	tok.mu.Lock()
	defer tok.mu.Unlock()

	// Try loading from verified local cached vocab file
	vocabFile, err := tok.EnsureVocabFile()
	if err == nil {
		ranks, err := tok.loadTiktokenRanks(vocabFile)
		if err == nil && len(ranks) > 0 {
			specialTokens := map[string]int{
				"<|endoftext|>":   199999,
				"<|endofprompt|>": 200018,
			}
			coreBPE, err := tiktoken.NewCoreBPE(ranks, specialTokens, config.O200KPattern)
			if err == nil && coreBPE != nil {
				specialSet := make(map[string]any, len(specialTokens))
				for k := range specialTokens {
					specialSet[k] = true
				}
				tok.t = tiktoken.NewTiktoken(coreBPE, nil, specialSet)
				tok.mode = "exact"
				return
			}
		}
	}

	// Try default tiktoken GetEncoding("o200k_base")
	enc, err := tiktoken.GetEncoding("o200k_base")
	if err == nil && enc != nil {
		tok.t = enc
		tok.mode = "exact"
		return
	}

	tok.mode = "estimate"
}

// CountTokens returns the exact or estimated token count for text
func (tok *Tokenizer) CountTokens(text string) int {
	if text == "" {
		return 0
	}

	tok.initOnce.Do(func() {
		tok.init()
	})

	if tok.mode == "exact" && tok.t != nil {
		return tok.countExactMemoized(text)
	}

	return tok.EstimateTokensHeuristic(text)
}

func fnvHash64(s string) uint64 {
	var hash uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		hash ^= uint64(s[i])
		hash *= 1099511628211
	}
	return hash
}

// countExactMemoized uses FNV-1a hash memoization to accelerate exact BPE encoding with 100% precision
func (tok *Tokenizer) countExactMemoized(text string) int {
	h := fnvHash64(text)
	if val, ok := tok.memo.Load(h); ok {
		return val.(int)
	}

	tokens := len(tok.t.Encode(text, nil, nil))
	tok.memo.Store(h, tokens)
	return tokens
}

var (
	heuristicPattern = regexp.MustCompile(`(?i:[sdmt]|ll|ve|re)|[^\r\n\p{L}\p{N}]?\p{L}+|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n]*|\s*[\r\n]+|\s+`)
)

// EstimateTokensHeuristic estimates token count when offline
func (tok *Tokenizer) EstimateTokensHeuristic(content string) int {
	if content == "" {
		return 0
	}

	matches := heuristicPattern.FindAllString(content, -1)
	if len(matches) == 0 {
		return int(math.Ceil(float64(len(content)) / 4.0))
	}

	tokens := 0
	for _, m := range matches {
		l := len(m)
		if l == 0 {
			continue
		}

		hasLetter := false
		for _, r := range m {
			if unicode.IsLetter(r) {
				hasLetter = true
				break
			}
		}

		if hasLetter {
			if l <= 7 {
				tokens += 1
			} else {
				tokens += 1 + int(math.Ceil(float64(l-7)/6.0))
			}
		} else if unicode.IsDigit(rune(m[0])) {
			tokens += 1
		} else if m[0] == ' ' || m[0] == '\t' || m[0] == '\n' || m[0] == '\r' {
			if l <= 4 {
				tokens += 1
			} else {
				tokens += 1 + int(math.Ceil(float64(l-4)/4.0))
			}
		} else {
			if l <= 2 {
				tokens += 1
			} else {
				tokens += int(math.Ceil(float64(l) / 2.0))
			}
		}
	}

	return tokens
}

// ClearCache clears memoization cache
func (tok *Tokenizer) ClearCache() {
	tok.memo = sync.Map{}
}
