//go:build embedui

package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// webuiDist holds the statically-exported frontend (built with
// `cd web && npm run build:static`, then copied into server/webui/dist). Compiled
// into the binary only when building with `-tags embedui`. The `all:` prefix is
// required so Next's `_next/` asset dir (leading underscore) is included.
//
//go:embed all:webui/dist
var webuiDist embed.FS

// webuiHandler serves the embedded SPA. Public (no JWT) — auth is enforced
// client-side and on /api. Serves the exported per-route index.html files and
// falls back to index.html so client-side routing still resolves unknown paths.
//
// staticFileServer adds gzip/brotli and ETag/immutable caching, which an
// embed.FS otherwise lacks. Without them a direct :8787 deployment (our compose
// file) re-downloads the multi-megabyte uncompressed bundle on every navigation,
// so clicks appear to time out on a remote link. Serving behind nginx hid this.
func (s *Server) webuiHandler() http.Handler {
	root, err := fs.Sub(webuiDist, "webui/dist")
	if err != nil {
		panic(err)
	}
	files := newStaticFileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		// 1) exact file (assets: _next/*, favicon.ico, ...)
		// 2) route dir → <p>/index.html (trailingSlash export)
		// 3) <p>.html
		// 4) SPA 兜底 → index.html（交给客户端路由）
		for _, cand := range []string{p, p + "/index.html", p + ".html"} {
			if files.serveFileIfExists(w, r, cand) {
				return
			}
		}
		// A missing content-hashed build asset is a real 404, never the SPA
		// document: returning HTML with a 200 makes the browser parse it as
		// JavaScript and hides a broken deployment.
		if strings.HasPrefix(p, "_next/") {
			http.NotFound(w, r)
			return
		}
		if files.serveFileIfExists(w, r, "index.html") {
			return
		}
		http.NotFound(w, r)
	})
}
