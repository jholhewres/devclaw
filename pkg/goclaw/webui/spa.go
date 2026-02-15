package webui

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaFileServer serves the React SPA from an embedded or provided filesystem.
// For any path that doesn't match a static file, it falls back to index.html
// so that client-side routing (React Router) can handle the path.
type spaFileServer struct {
	root http.FileSystem
}

func newSPAFileServer(fsys fs.FS) *spaFileServer {
	return &spaFileServer{root: http.FS(fsys)}
}

func (s *spaFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Never intercept API routes.
	if strings.HasPrefix(path, "/api/") {
		http.NotFound(w, r)
		return
	}

	// Try to serve the exact file (JS, CSS, images, etc.)
	if path != "/" {
		f, err := s.root.Open(path)
		if err == nil {
			f.Close()
			http.FileServer(s.root).ServeHTTP(w, r)
			return
		}
	}

	// For all other paths, serve index.html (SPA fallback).
	// This allows React Router to handle client-side routing.
	r.URL.Path = "/"
	http.FileServer(s.root).ServeHTTP(w, r)
}
