package tokenizer

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"codedocs/internal/config"
)

const vocabFailureCooldown = 10 * time.Minute

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func newVocabHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 4
	transport.MaxIdleConnsPerHost = 2
	transport.IdleConnTimeout = 30 * time.Second
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}

func (tok *Tokenizer) EnsureVocabFile() (string, error) {
	return tok.EnsureVocabFileContext(context.Background())
}

// EnsureVocabFileContext retrieves the vocabulary into a verified atomic cache.
func (tok *Tokenizer) EnsureVocabFileContext(ctx context.Context) (string, error) {
	if err := os.MkdirAll(tok.cacheDir, 0o700); err != nil {
		return "", fmt.Errorf("create tokenizer cache: %w", err)
	}
	cacheFile := filepath.Join(tok.cacheDir, "o200k_base.tiktoken")
	failureFile := filepath.Join(tok.cacheDir, "o200k_fail")

	if info, err := os.Stat(cacheFile); err == nil && info.Size() >= tok.minVocabSize && info.Size() <= tok.maxVocabSize {
		if tok.verifySHA256(cacheFile, tok.expectedSHA) {
			return cacheFile, nil
		}
		_ = os.Remove(cacheFile)
	}
	if info, err := os.Stat(failureFile); err == nil && time.Since(info.ModTime()) < vocabFailureCooldown {
		return "", fmt.Errorf("exact tokenizer download is cooling down after a recent network failure")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tok.vocabURL, nil)
	if err != nil {
		return "", fmt.Errorf("create vocabulary request: %w", err)
	}
	req.Header.Set("User-Agent", "CodePulse-Tokenizer/"+config.Version)
	req.Header.Set("Accept", "application/octet-stream")
	response, err := tok.httpClient.Do(req)
	if err != nil {
		tok.markVocabFailure(failureFile)
		return "", fmt.Errorf("download tokenizer vocabulary: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		tok.markVocabFailure(failureFile)
		return "", fmt.Errorf("download tokenizer vocabulary: unexpected HTTP status %s", response.Status)
	}
	if response.ContentLength > tok.maxVocabSize {
		tok.markVocabFailure(failureFile)
		return "", fmt.Errorf("tokenizer vocabulary is too large: %d bytes", response.ContentLength)
	}

	temporary, err := os.CreateTemp(tok.cacheDir, "o200k-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create tokenizer temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	_ = temporary.Chmod(0o600)

	hasher := sha256.New()
	limited := &io.LimitedReader{R: response.Body, N: tok.maxVocabSize + 1}
	written, copyErr := io.Copy(io.MultiWriter(temporary, hasher), limited)
	if copyErr != nil {
		tok.markVocabFailure(failureFile)
		return "", fmt.Errorf("save tokenizer vocabulary: %w", copyErr)
	}
	if written < tok.minVocabSize || written > tok.maxVocabSize {
		tok.markVocabFailure(failureFile)
		return "", fmt.Errorf("tokenizer vocabulary size %d is outside the accepted range", written)
	}
	if response.ContentLength >= 0 && response.ContentLength != written {
		tok.markVocabFailure(failureFile)
		return "", fmt.Errorf("tokenizer vocabulary was truncated: received %d of %d bytes", written, response.ContentLength)
	}
	computedHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(computedHash, tok.expectedSHA) {
		tok.markVocabFailure(failureFile)
		return "", fmt.Errorf("tokenizer vocabulary checksum mismatch")
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync tokenizer vocabulary: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close tokenizer vocabulary: %w", err)
	}
	if err := os.Rename(temporaryPath, cacheFile); err != nil {
		// Another app instance may have won the same atomic cache race.
		if tok.verifySHA256(cacheFile, tok.expectedSHA) {
			_ = os.Remove(failureFile)
			return cacheFile, nil
		}
		return "", fmt.Errorf("commit tokenizer vocabulary cache: %w", err)
	}
	committed = true
	_ = os.Remove(failureFile)
	return cacheFile, nil
}

func (tok *Tokenizer) markVocabFailure(path string) {
	_ = os.WriteFile(path, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o600)
}

func (tok *Tokenizer) verifySHA256(filePath, expectedHash string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return false
	}
	return strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), expectedHash)
}

func (tok *Tokenizer) loadTiktokenRanks(filePath string) (map[string]int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	ranks := make(map[string]int, 200_000)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		raw, decodeErr := base64.StdEncoding.DecodeString(fields[0])
		if decodeErr != nil {
			continue
		}
		rank, parseErr := strconv.Atoi(fields[1])
		if parseErr != nil {
			continue
		}
		ranks[string(raw)] = rank
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ranks, nil
}
