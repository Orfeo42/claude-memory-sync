package synthapi

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

func spaHandler(webDir string) http.Handler {
	if webDir == "" {
		return http.NotFoundHandler()
	}
	fsys := os.DirFS(webDir)
	files := http.FileServerFS(fsys)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		info, err := fs.Stat(fsys, name)
		if name == "" || err != nil || info.IsDir() {
			index := r.Clone(r.Context())
			index.URL.Path = "/"
			files.ServeHTTP(w, index)
			return
		}
		files.ServeHTTP(w, r)
	})
}
