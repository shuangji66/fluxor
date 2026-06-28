//go:build vue

package main

import (
	"embed"
	"io/fs"
)

//go:embed web/dist/*
var embedFS embed.FS

func init() {
	sub, err := fs.Sub(embedFS, "web/dist")
	if err != nil {
		panic(err)
	}
	staticFS = sub
}
