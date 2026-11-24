package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

// staticFileServer serves files out of an io/fs.FS (normally the embedded
// frontend) with the two things http.ServeFileFS cannot provide for an
// embed.FS:
//
//   - compression. An embed.FS response otherwise ships the raw bytes even
//     when the browser advertises gzip/br. The exported Next.js UI is several
//     megabytes of JavaScript, so on a remote link the browser's six
//     connections per host stall and complete together — which looks like the
//     UI timing out after every click.
//   - caching. embed.FS entries have a zero ModTime, so ServeContent emits no
//     Last-Modified and no ETag. Without an ETag every navigation re-downloads
//     the whole bundle.
//
// Compressed bodies are computed once and cached in memory. Files are only
// compressed when the client asks for it, the type is textual, the response is
// a full 200 (not a Range or HEAD range request), and the body is worth it.
type staticFileServer struct {
	fsys  fs.FS
	cache sync.Map // name -> *staticAsset
}

type staticAsset struct {
	raw         []byte
	etag        string
	contentType string

	gzipOnce sync.Once
	gzipBody []byte
	brOnce   sync.Once
	brBody   []byte
}

func newStaticFileServer(fsys fs.FS) *staticFileServer {
	return &staticFileServer{fsys: fsys}
}

// serveFileIfExists serves name when it is a regular file in the fs, reporting
// whether it did. Existence is checked through the content cache so a request
// for a missing path costs one failed fs read.
func (s *staticFileServer) serveFileIfExists(w http.ResponseWriter, r *http.Request, name string) bool {
	a, ok := s.load(name)
	if !ok {
		return false
	}
	s.serve(w, r, a)
	return true
}

func (s *staticFileServer) load(name string) (*staticAsset, bool) {
	if v, ok := s.cache.Load(name); ok {
		return v.(*staticAsset), true
	}
	data, err := fs.ReadFile(s.fsys, name)
	if err != nil || data == nil {
		return nil, false
	}
	sum := sha256.Sum256(data)
	a := &staticAsset{
		raw:         data,
		etag:        `"` + hex.EncodeToString(sum[:16]) + `"`,
		contentType: staticContentType(name),
	}
	actual, _ := s.cache.LoadOrStore(name, a)
	return actual.(*staticAsset), true
}

func (s *staticFileServer) serve(w http.ResponseWriter, r *http.Request, a *staticAsset) {
	h := w.Header()
	h.Set("Content-Type", a.contentType)
	h.Set("ETag", a.etag)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Vary", "Accept-Encoding")
	h.Set("Cache-Control", staticCacheControl(r.URL.Path))

	// Unconditional revalidation: a matching ETag means the browser already has
	// the exact bytes, including for the content-hashed immutable assets (which
	// normally never reach this branch).
	if etagMatches(r.Header.Get("If-None-Match"), a.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Range requests must not be compressed and need the standard partial-content
	// handling. ServeContent also applies our ETag/Content-Type headers.
	if r.Header.Get("Range") != "" {
		http.ServeContent(w, r, path.Base(r.URL.Path), time.Time{}, bytes.NewReader(a.raw))
		return
	}

	body := a.raw
	if enc := negotiateEncoding(r, a); enc != "" {
		if enc == "br" {
			a.brOnce.Do(func() { a.brBody = brotliCompress(a.raw) })
			body = a.brBody
		} else {
			a.gzipOnce.Do(func() { a.gzipBody = gzipCompress(a.raw) })
			body = a.gzipBody
		}
		h.Set("Content-Encoding", enc)
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := w.Write(body); err != nil {
		return
	}
}

// staticCacheControl marks the content-hashed Next.js build tree immutable and
// makes everything else revalidate. HTML and RSC payloads must revalidate so a
// rebuilt binary is picked up after an upgrade; the ETag keeps that cheap.
func staticCacheControl(urlPath string) string {
	if strings.HasPrefix(urlPath, "/_next/static/") {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

// negotiateEncoding returns "br" or "gzip" when the request accepts one and the
// asset is worth compressing, otherwise "".
func negotiateEncoding(r *http.Request, a *staticAsset) string {
	if r.Method == http.MethodHead || len(a.raw) < 1024 || !isCompressible(a.contentType) {
		return ""
	}
	ae := r.Header.Get("Accept-Encoding")
	// Prefer brotli; brotli-5 compresses the UI better than gzip for similar CPU.
	if acceptsEncoding(ae, "br") {
		return "br"
	}
	if acceptsEncoding(ae, "gzip") {
		return "gzip"
	}
	return ""
}

// acceptsEncoding reports whether name is acceptable per an Accept-Encoding
// header. It understands q-values well enough to reject explicit q=0.
func acceptsEncoding(header, name string) bool {
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), name) {
			continue
		}
		for _, p := range fields[1:] {
			p = strings.TrimSpace(p)
			if !strings.HasPrefix(p, "q=") {
				continue
			}
			if q := strings.TrimPrefix(p, "q="); q == "0" || strings.HasPrefix(q, "0.") && strings.Trim(q[2:], "0") == "" {
				return false
			}
		}
		return true
	}
	return false
}

func isCompressible(contentType string) bool {
	ct := strings.ToLower(contentType)
	if strings.HasPrefix(ct, "text/") {
		// SSE is an endless stream: compressing it buffers frames and breaks the UI.
		return !strings.HasPrefix(ct, "text/event-stream")
	}
	switch {
	case strings.Contains(ct, "javascript"),
		strings.Contains(ct, "json"),
		strings.Contains(ct, "xml"),
		strings.Contains(ct, "svg"),
		strings.Contains(ct, "wasm"):
		return true
	}
	return false
}

func gzipCompress(src []byte) []byte {
	var buf bytes.Buffer
	// BestSpeed is the right trade-off for on-demand per-file compression: the
	// UI chunks are large but the cache means each is compressed once.
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	_, _ = zw.Write(src)
	_ = zw.Close()
	return buf.Bytes()
}

func brotliCompress(src []byte) []byte {
	var buf bytes.Buffer
	zw := brotli.NewWriterLevel(&buf, 5)
	_, _ = zw.Write(src)
	_ = zw.Close()
	return buf.Bytes()
}

func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || part == etag || strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}

// staticContentType fixes the content types we rely on rather than depending on
// the host's mime.types database (which differs between Linux and macOS and can
// make a binary behave differently in the image and on a developer machine).
func staticContentType(name string) string {
	switch ext := strings.ToLower(path.Ext(name)); ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json", ".map":
		return "application/json"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".wasm":
		return "application/wasm"
	case ".xml":
		return "application/xml"
	case ".webmanifest":
		return "application/manifest+json"
	case ".pdf":
		return "application/pdf"
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
