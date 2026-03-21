package web

import (
	"embed"
	"io/fs"
)

// DistFS holds the built SPA files (populated at compile time).
// The web/build directory is produced by `bun run build` (SvelteKit + adapter-static).
// When building, ensure `web/build/` exists before running `go build`.
//
//go:embed all:build
var distFS embed.FS

// FS returns a sub-filesystem rooted at "build/" so file paths
// start from the SPA root (e.g. "index.html", "_app/...", etc.).
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "build")
}
