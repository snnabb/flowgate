package web

import "embed"

// EmbeddedFiles contains all the web UI static files
//
//go:embed all:static
var EmbeddedFiles embed.FS
