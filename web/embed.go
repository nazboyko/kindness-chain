// Package web carries the built frontend inside the binary.
package web

import "embed"

// Dist is the Vite build output. It is filled by `make build-web` or by
// the Dockerfile. Only a keep file is committed, which is enough for the
// embed pattern to compile on a fresh clone.
//
//go:embed all:dist
var Dist embed.FS
