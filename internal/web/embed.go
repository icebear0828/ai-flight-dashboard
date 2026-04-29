package web

import "embed"

//go:embed dist/*
//go:embed dist/**/*
var StaticFiles embed.FS
