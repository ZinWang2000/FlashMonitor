package web

import (
	"embed"
	"io/fs"
)

var embedded embed.FS

func FS() fs.FS {
	return embedded
}
