package main

import (
	"embed"
	"io/fs"
)

// The built frontend (web/dist) is copied here by the build before the
// program is compiled. Without it the program still runs and serves the API.
//
//go:embed all:webdist
var embedded embed.FS

func webFS() fs.FS {
	sub, err := fs.Sub(embedded, "webdist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
