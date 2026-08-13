package updater

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	githubAPIBase       = "https://api.github.com"
	maxReleaseBodyBytes = 4 << 20
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	State              string `json:"state"`
	Size               int64  `json:"size"`
	ReleaseVersion     string `json:"-"`
}

type githubRelease struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	HTMLURL     string         `json:"html_url"`
	PublishedAt string         `json:"published_at"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	Assets      []releaseAsset `json:"assets"`
}

func (asset releaseAsset) sha256() (string, bool) {
	const prefix = "sha256:"
	digest := strings.ToLower(strings.TrimSpace(asset.Digest))
	if !strings.HasPrefix(digest, prefix) {
		return "", false
	}
	digest = strings.TrimPrefix(digest, prefix)
	if len(digest) != 64 {
		return "", false
	}
	for _, character := range digest {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return "", false
		}
	}
	return digest, true
}

func newHTTPTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 8
	transport.MaxIdleConnsPerHost = 4
	transport.IdleConnTimeout = 30 * time.Second
	transport.ResponseHeaderTimeout = 15 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.ForceAttemptHTTP2 = true
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return transport
}

func newCheckHTTPClient() *http.Client {
	return &http.Client{
		Transport: newHTTPTransport(),
		Timeout:   20 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if request.URL.Scheme != "https" || !strings.EqualFold(request.URL.Hostname(), "api.github.com") {
				return fmt.Errorf("untrusted release API redirect")
			}
			return nil
		},
	}
}

func newDownloadHTTPClient() *http.Client {
	return &http.Client{
		Transport: newHTTPTransport(),
		Timeout:   15 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if !isTrustedDownloadURL(request.URL) {
				return fmt.Errorf("untrusted download redirect")
			}
			return nil
		},
	}
}

func isTrustedDownloadURL(downloadURL *url.URL) bool {
	if downloadURL == nil || downloadURL.Scheme != "https" || downloadURL.User != nil {
		return false
	}
	switch strings.ToLower(downloadURL.Hostname()) {
	case "github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com":
		return true
	default:
		return false
	}
}

func validateReleaseAssetURL(repository, rawURL string) error {
	downloadURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse release asset URL: %w", err)
	}
	if !isTrustedDownloadURL(downloadURL) || !strings.EqualFold(downloadURL.Hostname(), "github.com") {
		return fmt.Errorf("release asset must use GitHub HTTPS")
	}
	expectedPrefix := "/" + strings.Trim(repository, "/") + "/releases/download/"
	if !strings.HasPrefix(downloadURL.EscapedPath(), expectedPrefix) {
		return fmt.Errorf("release asset does not belong to %s", repository)
	}
	return nil
}
