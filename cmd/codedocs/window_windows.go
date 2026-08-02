//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
)

var (
	moddwmapi                 = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = moddwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	DWMWA_USE_IMMERSIVE_DARK_MODE     = 20
	DWMWA_USE_IMMERSIVE_DARK_MODE_OLD = 19
)

func setWindowDarkMode(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	darkMode := int32(1)
	// Enable DWM Immersive Dark Mode for Windows 10 (1909+) and Windows 11 titlebar
	_, _, _ = procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(DWMWA_USE_IMMERSIVE_DARK_MODE),
		uintptr(unsafe.Pointer(&darkMode)),
		unsafe.Sizeof(darkMode),
	)
	_, _, _ = procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(DWMWA_USE_IMMERSIVE_DARK_MODE_OLD),
		uintptr(unsafe.Pointer(&darkMode)),
		unsafe.Sizeof(darkMode),
	)
}

func openNativeWindow(url string, title string) {
	w := webview2.New(false)
	if w != nil {
		defer w.Destroy()
		w.SetTitle(title)
		w.SetSize(1280, 850, webview2.HintNone)

		// Set sleek Win32 Native Dark Titlebar matching dark app theme
		setWindowDarkMode(uintptr(w.Window()))

		w.Navigate(url)
		w.Run()
	}
}
