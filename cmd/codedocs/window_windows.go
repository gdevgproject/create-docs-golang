//go:build windows

package main

import (
	"github.com/jchv/go-webview2"
)

func openNativeWindow(url string, title string) {
	w := webview2.New(false)
	if w != nil {
		defer w.Destroy()
		w.SetTitle(title)
		w.SetSize(1280, 850, webview2.HintNone)
		w.Navigate(url)
		w.Run()
	}
}
