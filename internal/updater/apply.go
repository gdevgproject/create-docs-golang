package updater

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const internalUpdateHelperFlag = "--codedocs-update-helper"

const (
	updateReadyPathEnv  = "CODEDOCS_UPDATE_READY_PATH"
	updateReadyTokenEnv = "CODEDOCS_UPDATE_READY_TOKEN"
)

type updatePlan struct {
	ParentPID  int      `json:"parent_pid"`
	TargetPath string   `json:"target_path"`
	NewPath    string   `json:"new_path"`
	BackupPath string   `json:"backup_path"`
	HelperPath string   `json:"helper_path"`
	PlanPath   string   `json:"plan_path"`
	ReadyPath  string   `json:"ready_path"`
	WorkingDir string   `json:"working_dir"`
	Args       []string `json:"args"`
	SHA256     string   `json:"sha256"`
	GOOS       string   `json:"goos"`
	GOARCH     string   `json:"goarch"`
	Version    string   `json:"version"`
	CreatedAt  string   `json:"created_at"`
	ReadyToken string   `json:"ready_token"`
}

func (manager *Manager) ApplyPreparedUpdate() (returnErr error) {
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return fmt.Errorf("updater is closed")
	}
	if manager.progress.State == StateApplying {
		manager.mu.Unlock()
		return fmt.Errorf("update is already being applied")
	}
	prepared := manager.prepared
	if prepared == nil || !prepared.verified || manager.progress.State != StateReady {
		manager.mu.Unlock()
		return fmt.Errorf("no verified update is ready")
	}
	readyVersion := manager.progress.Version
	manager.progress.State = StateApplying
	manager.progress.Message = "Preparing verified update"
	manager.progress.UpdatedAt = nowTimestamp()
	manager.mu.Unlock()
	defer func() {
		if returnErr == nil {
			return
		}
		manager.mu.Lock()
		if !manager.closed && manager.progress.State == StateApplying {
			manager.progress.State = StateReady
			manager.progress.Error = returnErr.Error()
			manager.progress.Message = "Update is ready; installation can be retried"
			manager.progress.CanRetry = true
			manager.progress.UpdatedAt = nowTimestamp()
		}
		manager.mu.Unlock()
	}()

	targetPath, err := manager.resolveExecutable()
	if err != nil {
		return err
	}
	if !samePath(prepared.path, targetPath+".new") {
		return fmt.Errorf("prepared update path is invalid")
	}
	digest, err := fileSHA256(prepared.path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(digest, prepared.sha256) {
		return fmt.Errorf("prepared update failed final SHA-256 verification")
	}
	if err := validateExecutable(prepared.path, manager.goos, manager.goarch); err != nil {
		return err
	}

	helperPath := targetPath + helperSuffix()
	planPath := targetPath + ".update.json"
	readyPath := targetPath + ".update.ready"
	readyToken, err := newUpdateToken()
	if err != nil {
		return fmt.Errorf("create update handshake: %w", err)
	}
	if err := copyFileAtomic(prepared.path, helperPath, 0700); err != nil {
		return fmt.Errorf("prepare update helper: %w", err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		workingDirectory = filepath.Dir(targetPath)
	}
	plan := updatePlan{
		ParentPID:  os.Getpid(),
		TargetPath: targetPath,
		NewPath:    prepared.path,
		BackupPath: targetPath + ".old",
		HelperPath: helperPath,
		PlanPath:   planPath,
		ReadyPath:  readyPath,
		WorkingDir: workingDirectory,
		Args:       append([]string(nil), os.Args[1:]...),
		SHA256:     digest,
		GOOS:       manager.goos,
		GOARCH:     manager.goarch,
		Version:    readyVersion,
		CreatedAt:  nowTimestamp(),
		ReadyToken: readyToken,
	}
	if err := writeUpdatePlan(plan); err != nil {
		_ = os.Remove(helperPath)
		return err
	}
	if _, err := startDetached(helperPath, []string{internalUpdateHelperFlag, planPath}, filepath.Dir(targetPath), nil); err != nil {
		_ = os.Remove(planPath)
		_ = os.Remove(helperPath)
		return fmt.Errorf("start update helper: %w", err)
	}

	manager.mu.Lock()
	manager.progress = DownloadProgress{
		State:     StateApplying,
		Percent:   100,
		Message:   "Installing verified update",
		Version:   plan.Version,
		AssetName: prepared.asset.Name,
		SHA256:    prepared.sha256,
		Verified:  true,
		UpdatedAt: nowTimestamp(),
	}
	manager.mu.Unlock()
	return nil
}

// HandleBootstrap executes the private updater-helper mode before normal flag
// parsing. It returns handled=true only for the exact internal invocation.
func HandleBootstrap(args []string) (handled bool, exitCode int) {
	if len(args) != 3 || args[1] != internalUpdateHelperFlag {
		return false, 0
	}
	if err := runUpdateHelper(args[2]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "CodeDocs update failed:", err)
		return true, 1
	}
	return true, 0
}

func runUpdateHelper(planPath string) error {
	plan, err := readUpdatePlan(planPath)
	if err != nil {
		return err
	}
	if err := validateUpdatePlan(plan, planPath); err != nil {
		return err
	}
	if err := waitForProcessExit(plan.ParentPID, 90*time.Second); err != nil {
		return fmt.Errorf("wait for CodeDocs to close: %w", err)
	}
	digest, err := fileSHA256(plan.NewPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(digest, plan.SHA256) {
		return fmt.Errorf("update SHA-256 changed after verification")
	}
	if err := validateExecutable(plan.NewPath, plan.GOOS, plan.GOARCH); err != nil {
		return err
	}

	_ = os.Remove(plan.BackupPath)
	if err := retryRename(plan.TargetPath, plan.BackupPath, 15*time.Second); err != nil {
		return fmt.Errorf("back up current executable: %w", err)
	}
	if err := retryRename(plan.NewPath, plan.TargetPath, 15*time.Second); err != nil {
		rollbackErr := retryRename(plan.BackupPath, plan.TargetPath, 15*time.Second)
		return errors.Join(fmt.Errorf("install new executable: %w", err), rollbackErr)
	}
	processID, err := startDetached(plan.TargetPath, plan.Args, plan.WorkingDir, []string{
		updateReadyPathEnv + "=" + plan.ReadyPath,
		updateReadyTokenEnv + "=" + plan.ReadyToken,
	})
	if err != nil {
		rollbackNewErr := retryRename(plan.TargetPath, plan.NewPath, 10*time.Second)
		rollbackOldErr := retryRename(plan.BackupPath, plan.TargetPath, 10*time.Second)
		return errors.Join(fmt.Errorf("restart updated CodeDocs: %w", err), rollbackNewErr, rollbackOldErr)
	}
	if err := waitForReadySignal(plan.ReadyPath, plan.ReadyToken, 45*time.Second); err != nil {
		terminateErr := terminateProcess(processID, 10*time.Second)
		_ = os.Remove(plan.NewPath)
		rollbackNewErr := retryRename(plan.TargetPath, plan.NewPath, 10*time.Second)
		rollbackOldErr := retryRename(plan.BackupPath, plan.TargetPath, 10*time.Second)
		_, restartErr := startDetached(plan.TargetPath, plan.Args, plan.WorkingDir, nil)
		_ = os.Remove(plan.PlanPath)
		_ = os.Remove(plan.ReadyPath)
		return errors.Join(fmt.Errorf("updated application did not become healthy: %w", err), terminateErr, rollbackNewErr, rollbackOldErr, restartErr)
	}
	_ = os.Remove(plan.ReadyPath)
	_ = os.Remove(plan.BackupPath)
	_ = os.Remove(plan.PlanPath)
	return nil
}

// MarkStartupHealthy completes the updater handshake after the local server and
// desktop shell are ready. Ordinary launches have no handshake and are a no-op.
func MarkStartupHealthy() error {
	readyPath := os.Getenv(updateReadyPathEnv)
	readyToken := os.Getenv(updateReadyTokenEnv)
	if readyPath == "" && readyToken == "" {
		return nil
	}
	defer os.Unsetenv(updateReadyPathEnv)
	defer os.Unsetenv(updateReadyTokenEnv)

	executable, err := resolvedExecutable()
	if err != nil {
		return err
	}
	if !samePath(readyPath, executable+".update.ready") || !validUpdateToken(readyToken) {
		return fmt.Errorf("invalid update startup handshake")
	}
	temporaryPath := readyPath + ".tmp"
	_ = os.Remove(temporaryPath)
	if err := os.WriteFile(temporaryPath, []byte(readyToken), 0600); err != nil {
		return fmt.Errorf("write update ready signal: %w", err)
	}
	_ = os.Remove(readyPath)
	if err := os.Rename(temporaryPath, readyPath); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("publish update ready signal: %w", err)
	}
	return nil
}

func writeUpdatePlan(plan updatePlan) error {
	data, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("encode update plan: %w", err)
	}
	temporaryPath := plan.PlanPath + ".tmp"
	_ = os.Remove(temporaryPath)
	file, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create update plan: %w", err)
	}
	removeTemporary := true
	defer func() {
		_ = file.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write update plan: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync update plan: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close update plan: %w", err)
	}
	_ = os.Remove(plan.PlanPath)
	if err := os.Rename(temporaryPath, plan.PlanPath); err != nil {
		return fmt.Errorf("publish update plan: %w", err)
	}
	removeTemporary = false
	return nil
}

func readUpdatePlan(planPath string) (updatePlan, error) {
	file, err := os.Open(planPath)
	if err != nil {
		return updatePlan{}, fmt.Errorf("open update plan: %w", err)
	}
	defer file.Close()
	var plan updatePlan
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return updatePlan{}, fmt.Errorf("decode update plan: %w", err)
	}
	return plan, nil
}

func validateUpdatePlan(plan updatePlan, suppliedPlanPath string) error {
	targetPath, err := filepath.Abs(filepath.Clean(plan.TargetPath))
	if err != nil || targetPath == "" {
		return fmt.Errorf("invalid update target")
	}
	expected := map[string]string{
		"new":    targetPath + ".new",
		"backup": targetPath + ".old",
		"helper": targetPath + helperSuffix(),
		"plan":   targetPath + ".update.json",
		"ready":  targetPath + ".update.ready",
	}
	if !samePath(plan.TargetPath, targetPath) ||
		!samePath(plan.NewPath, expected["new"]) ||
		!samePath(plan.BackupPath, expected["backup"]) ||
		!samePath(plan.HelperPath, expected["helper"]) ||
		!samePath(plan.PlanPath, expected["plan"]) ||
		!samePath(plan.ReadyPath, expected["ready"]) ||
		!samePath(suppliedPlanPath, expected["plan"]) {
		return fmt.Errorf("update plan contains paths outside the application directory")
	}
	if plan.ParentPID <= 0 || plan.GOOS != runtime.GOOS || plan.GOARCH != runtime.GOARCH {
		return fmt.Errorf("update plan platform or process is invalid")
	}
	if _, valid := (releaseAsset{Digest: "sha256:" + plan.SHA256}).sha256(); !valid {
		return fmt.Errorf("update plan digest is invalid")
	}
	if !validUpdateToken(plan.ReadyToken) {
		return fmt.Errorf("update plan handshake is invalid")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, plan.CreatedAt)
	if err != nil || time.Since(createdAt) > 24*time.Hour || time.Until(createdAt) > 5*time.Minute {
		return fmt.Errorf("update plan has expired")
	}
	return nil
}

func newUpdateToken() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

func validUpdateToken(token string) bool {
	if len(token) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(token)
	return err == nil && len(decoded) == 32
}

func waitForReadySignal(path, token string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(data)) == token {
			return nil
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("startup handshake timed out after %s", timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func copyFileAtomic(sourcePath, destinationPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	temporaryPath := destinationPath + ".part"
	_ = os.Remove(temporaryPath)
	destination, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	removeTemporary := true
	defer func() {
		_ = destination.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	written, err := io.Copy(destination, io.LimitReader(source, maxExecutableBytes+1))
	if err != nil {
		return err
	}
	if written < minExecutableBytes || written > maxExecutableBytes {
		return fmt.Errorf("helper executable size is outside the safe range")
	}
	if err := destination.Sync(); err != nil {
		return err
	}
	if err := destination.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, mode); err != nil {
		return err
	}
	_ = os.Remove(destinationPath)
	if err := os.Rename(temporaryPath, destinationPath); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file for verification: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, maxExecutableBytes+1))
	if err != nil {
		return "", fmt.Errorf("hash update: %w", err)
	}
	if written < minExecutableBytes || written > maxExecutableBytes {
		return "", fmt.Errorf("update size is outside the safe range")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func retryRename(sourcePath, destinationPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		lastErr = os.Rename(sourcePath, destinationPath)
		if lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func samePath(left, right string) bool {
	leftPath, leftErr := filepath.Abs(filepath.Clean(left))
	rightPath, rightErr := filepath.Abs(filepath.Clean(right))
	if leftErr != nil || rightErr != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(leftPath, rightPath)
	}
	return leftPath == rightPath
}
