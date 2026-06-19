//go:build !vue

package main

import (
	"embed"
)

//go:embed static/*
var embedFS embed.FS

func init() {
	staticFS = embedFS
}
