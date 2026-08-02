package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html style.css app.js favicon.ico icon.png
var embedFS embed.FS

// GetFS returns an fs.FS sub-filesystem serving web assets directly
func GetFS() fs.FS {
	return embedFS
}
