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

	moduser32               = syscall.NewLazyDLL("user32.dll")
	procShowWindow          = moduser32.NewProc("ShowWindow")
	procSetForegroundWindow = moduser32.NewProc("SetForegroundWindow")
)

const (
	DWMWA_USE_IMMERSIVE_DARK_MODE     = 20
	DWMWA_USE_IMMERSIVE_DARK_MODE_OLD = 19
	SW_MAXIMIZE                       = 3
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

func maximizeAndFocusWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	// Maximize window to full screen and focus on startup/restart
	_, _, _ = procShowWindow.Call(hwnd, uintptr(SW_MAXIMIZE))
	_, _, _ = procSetForegroundWindow.Call(hwnd)
}

func openNativeWindow(url string, title string) {
	w := webview2.New(false)
	if w != nil {
		defer w.Destroy()
		w.SetTitle(title)
		w.SetSize(1280, 850, webview2.HintNone)

		hwnd := uintptr(w.Window())

		// Transform Win32 Titlebar into sleek Native Dark Mode
		setWindowDarkMode(hwnd)

		// Automatically Maximize and Focus window on startup/restart
		maximizeAndFocusWindow(hwnd)

		w.Navigate(url)
		w.Run() // Creates TRUE Native Win32 Window directly in Go process space
	}
}
