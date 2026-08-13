//go:build windows

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
)

var (
	modDWMAPI                 = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = modDWMAPI.NewProc("DwmSetWindowAttribute")
)

const (
	dwmwaUseImmersiveDarkMode    = 20
	dwmwaUseImmersiveDarkModeOld = 19
	appIconResourceID            = 2
	defaultWindowWidth           = 1440
	defaultWindowHeight          = 900
	minimumWindowWidth           = 760
	minimumWindowHeight          = 560
)

func setWindowDarkMode(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

	darkMode := int32(1)
	for _, attribute := range []uintptr{dwmwaUseImmersiveDarkMode, dwmwaUseImmersiveDarkModeOld} {
		_, _, _ = procDwmSetWindowAttribute.Call(
			hwnd,
			attribute,
			uintptr(unsafe.Pointer(&darkMode)),
			unsafe.Sizeof(darkMode),
		)
	}
}

func openNativeWindow(ctx context.Context, url, title, cacheDir string, onReady func() error) error {
	options := webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  filepath.Join(cacheDir, "webview2"),
		WindowOptions: webview2.WindowOptions{
			Title:  title,
			Width:  defaultWindowWidth,
			Height: defaultWindowHeight,
			IconId: appIconResourceID,
			Center: true,
		},
	}

	w := webview2.NewWithOptions(options)
	if w == nil {
		return fmt.Errorf("WebView2 could not create a native window; install or repair the Microsoft Edge WebView2 Runtime")
	}
	defer w.Destroy()

	w.SetSize(minimumWindowWidth, minimumWindowHeight, webview2.HintMin)
	setWindowDarkMode(uintptr(w.Window()))
	w.Navigate(url)
	if onReady != nil {
		if err := onReady(); err != nil {
			return fmt.Errorf("complete update startup: %w", err)
		}
	}

	windowDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			w.Terminate()
		case <-windowDone:
		}
	}()

	w.Run()
	close(windowDone)
	return nil
}
