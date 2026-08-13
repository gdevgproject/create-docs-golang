//go:build windows

package updater

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestUpdateHelperEndToEndWindows is opt-in because it launches a real,
// detached CodeDocs process. CI can enable it after building the Windows app.
func TestUpdateHelperEndToEndWindows(t *testing.T) {
	if os.Getenv("CODEDOCS_RUN_UPDATE_E2E") != "1" {
		t.Skip("set CODEDOCS_RUN_UPDATE_E2E=1 to run the updater process test")
	}
	sourceBinary := os.Getenv("CODEDOCS_E2E_BINARY")
	if sourceBinary == "" {
		t.Fatal("CODEDOCS_E2E_BINARY is required")
	}
	sourceBinary, err := filepath.Abs(sourceBinary)
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	targetPath := filepath.Join(directory, "codedocs-e2e.exe")
	newPath := targetPath + ".new"
	helperPath := targetPath + helperSuffix()
	for _, destination := range []string{targetPath, newPath, helperPath} {
		if err := copyFileAtomic(sourceBinary, destination, 0700); err != nil {
			t.Fatalf("copy E2E binary to %s: %v", destination, err)
		}
	}
	digest, err := fileSHA256(newPath)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	plan := updatePlan{
		ParentPID:  2147480000,
		TargetPath: targetPath,
		NewPath:    newPath,
		BackupPath: targetPath + ".old",
		HelperPath: helperPath,
		PlanPath:   targetPath + ".update.json",
		ReadyPath:  targetPath + ".update.ready",
		WorkingDir: directory,
		Args: []string{
			"--open-browser=false",
			"--host=127.0.0.1",
			"--port=" + strconv.Itoa(port),
			"--temp-dir=" + filepath.Join(directory, "documents"),
			"--cache-dir=" + filepath.Join(directory, "cache"),
			"--bookmark-file=" + filepath.Join(directory, "bookmarks.json"),
		},
		SHA256:     digest,
		GOOS:       "windows",
		GOARCH:     "amd64",
		Version:    "v-e2e",
		CreatedAt:  nowTimestamp(),
		ReadyToken: strings.Repeat("a", 64),
	}
	if err := writeUpdatePlan(plan); err != nil {
		t.Fatal(err)
	}

	helperContext, cancelHelper := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelHelper()
	command := exec.CommandContext(helperContext, helperPath, internalUpdateHelperFlag, plan.PlanPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("update helper failed: %v\n%s", err, output)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 2 * time.Second}
	defer func() {
		request, _ := http.NewRequest(http.MethodPost, baseURL+"/api/shutdown", nil)
		if request != nil {
			_, _ = client.Do(request)
		}
	}()
	waitForPing(t, client, baseURL+"/api/ping")

	for _, removedPath := range []string{plan.BackupPath, plan.PlanPath, plan.ReadyPath} {
		if _, err := os.Stat(removedPath); !os.IsNotExist(err) {
			t.Errorf("successful update left artifact %s", removedPath)
		}
	}
	installedDigest, err := fileSHA256(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if installedDigest != digest {
		t.Fatalf("installed executable digest = %s, want %s", installedDigest, digest)
	}

	request, _ := http.NewRequest(http.MethodPost, baseURL+"/api/shutdown", nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("shut down E2E app: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	waitForExecutableRelease(t, targetPath)
}

func waitForPing(t *testing.T, client *http.Client, pingURL string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(pingURL)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("updated application did not expose its health endpoint")
}

func waitForExecutableRelease(t *testing.T, targetPath string) {
	t.Helper()
	probePath := targetPath + ".probe"
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := os.Rename(targetPath, probePath); err == nil {
			if err := os.Rename(probePath, targetPath); err != nil {
				t.Fatalf("restore executable after release probe: %v", err)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("updated application did not release its executable after shutdown")
}
