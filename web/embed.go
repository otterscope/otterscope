// Package web embeds the built frontend. dist/ is gitignored (only
// .gitkeep is committed), so a plain `go build ./...` always compiles;
// building the frontend first (`npm run build`) makes the binary serve the
// real UI, otherwise the server falls back to its placeholder page.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend rooted at its index.html, or ok=false when
// the frontend wasn't built before the binary.
func Dist() (fsys fs.FS, ok bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}
