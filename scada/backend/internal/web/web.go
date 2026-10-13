// Package web embeds the built HMI (Vite output) into the executable so a
// single binary serves both the REST/WebSocket API and the web interface.
//
// The frontend build writes directly into ./dist (see vite.config.js):
//
//	cd scada/frontend && npm run build
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// Embedded returns the embedded HMI files and whether a build is present
// (dist only holds a placeholder until the frontend has been built).
func Embedded() (fs.FS, bool) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	_, err = fs.Stat(sub, "index.html")
	return sub, err == nil
}

// Handler serves static files from files and falls back to index.html for
// client-side routes (/views/..., /config, ...).
func Handler(files fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(files))
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if st, err := fs.Stat(files, name); err == nil && !st.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				// Vite puts a content hash in asset file names.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		index, err := fs.ReadFile(files, "index.html")
		if err != nil {
			http.Error(w, "HMI saknas: bygg frontend med 'npm run build' i scada/frontend och bygg om backend.", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	}
}
