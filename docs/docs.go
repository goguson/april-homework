package docs

import (
	"embed"
	"net/http"
	"path"

	"github.com/go-chi/chi/v5"
)

//go:embed *
var Docs embed.FS

func NewOpenAPI(mux *chi.Mux) {
	fileServer := fileServerWithContentType(http.FS(Docs))
	mux.Handle("/api/docs/*", http.StripPrefix("/api/docs", fileServer))
	mux.Get("/api/docs", http.RedirectHandler("/api/docs/", http.StatusMovedPermanently).ServeHTTP)
}

func fileServerWithContentType(root http.FileSystem) http.Handler {
	fs := http.FileServer(root)
	contentTypes := map[string]string{
		".js":   "application/javascript",
		".css":  "text/css",
		".html": "text/html",
		".yaml": "text/yaml",
		".yml":  "text/yaml",
		".json": "application/json",
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ext := path.Ext(r.URL.Path)
		if contentType, ok := contentTypes[ext]; ok {
			w.Header().Set("Content-Type", contentType)
		}
		fs.ServeHTTP(w, r)
	})
}
