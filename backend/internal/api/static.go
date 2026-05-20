// Tiny helpers for serving embedded static files. Pulled into its own file so
// router.go stays focused on routing.
package api

import (
	"io"
	"path/filepath"
)

func copyFS(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}

func contentTypeFor(p string) string {
	switch filepath.Ext(p) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}
