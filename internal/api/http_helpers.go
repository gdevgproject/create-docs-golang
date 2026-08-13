package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxJSONBodyBytes  = 1 << 20
	maxTokenBodyBytes = 8 << 20
	maxImportBytes    = 32 << 20
	maxPreviewBytes   = 4 << 20
)

func (server *Server) decodeJSONBody(w http.ResponseWriter, request *http.Request, destination any, limit int64) bool {
	request.Body = http.MaxBytesReader(w, request.Body, limit)
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(destination); err != nil {
		server.writeBodyError(w, err)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		server.jsonError(w, "Request body must contain one JSON value", http.StatusBadRequest)
		return false
	}
	return true
}

func (server *Server) readBody(w http.ResponseWriter, request *http.Request, limit int64) ([]byte, bool) {
	request.Body = http.MaxBytesReader(w, request.Body, limit)
	data, err := io.ReadAll(request.Body)
	if err != nil {
		server.writeBodyError(w, err)
		return nil, false
	}
	return data, true
}

func (server *Server) writeBodyError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		server.jsonError(w, fmt.Sprintf("Request body exceeds %d MiB", (tooLarge.Limit+(1<<20)-1)>>20), http.StatusRequestEntityTooLarge)
		return
	}
	server.jsonError(w, "Invalid request body", http.StatusBadRequest)
}

func (server *Server) openGeneratedFile(rawName string) (*os.File, os.FileInfo, string, error) {
	fileName := filepath.Base(strings.TrimSpace(rawName))
	if fileName == "" || fileName == "." || fileName == string(filepath.Separator) {
		return nil, nil, "", os.ErrInvalid
	}
	tempDirectory, err := filepath.Abs(server.cfg.TempDir)
	if err != nil {
		return nil, nil, "", err
	}
	targetPath := filepath.Join(tempDirectory, fileName)
	resolvedPath, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return nil, nil, "", err
	}
	relativePath, err := filepath.Rel(tempDirectory, resolvedPath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return nil, nil, "", os.ErrPermission
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		return nil, nil, "", err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, "", err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, "", os.ErrInvalid
	}
	return file, info, fileName, nil
}

func parseOffsetOrLimit(queryValue string, defaultValue int64) (int64, error) {
	if strings.TrimSpace(queryValue) == "" {
		return defaultValue, nil
	}
	value, err := strconv.ParseInt(queryValue, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("value must be a non-negative integer")
	}
	return value, nil
}

func attachmentHeader(fileName string) string {
	value := mime.FormatMediaType("attachment", map[string]string{"filename": fileName})
	if value == "" {
		return "attachment"
	}
	return value
}
