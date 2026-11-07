package core

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

func Dist() fs.FS { s, _ := fs.Sub(distFS, "dist"); return s }
