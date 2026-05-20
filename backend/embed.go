// Package backend embeds the built React UI so the server is a single binary.
// The directory ui/dist is produced by `make ui-build`; in Phase 2 it contains
// a placeholder index.html only.
package backend

import "embed"

//go:embed ui/dist
var UIFiles embed.FS
