// Package assets embeds the static files served by the app. app.css is built
// ahead of time by build-css.sh and is not committed.
package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
)

//go:embed static
var files embed.FS

// FS is the static directory.
func FS() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

// Version is a short content hash used to bust browser caches.
func Version() string {
	h := sha256.New()
	for _, name := range []string{"static/app.css", "static/app.js", "static/theme.js"} {
		b, _ := files.ReadFile(name)
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}
