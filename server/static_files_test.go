package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
)

// testStaticFS returns a tiny frontend tree: a compressible chunk under the
// content-hashed _next/static tree, an already-compressed image, and an HTML
// document that must revalidate.
func testStaticFS() fstest.MapFS {
	js := strings.Repeat("export const value = 1;\n", 200) // > 1 KB
	return fstest.MapFS{
		"_next/static/chunks/app.js":  {Data: []byte(js)},
		"_next/static/media/logo.png": {Data: bytes.Repeat([]byte{0x89, 0x50, 0x4e, 0x47}, 600)},
		"index.html":                  {Data: []byte("<html><body>artex</body></html>")},
		"_next/static/chunks/tiny.js": {Data: []byte("x")},
	}
}

func serveTest(t *testing.T, fss *staticFileServer, name, method string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/"+name, nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	if ok := fss.serveFileIfExists(w, r, name); !ok {
		t.Fatalf("serveFileIfExists(%q) = false, want true", name)
	}
	return w
}

func TestStaticServesBrotliWhenAccepted(t *testing.T) {
	fss := newStaticFileServer(testStaticFS())
	w := serveTest(t, fss, "_next/static/chunks/app.js", http.MethodGet, map[string]string{"Accept-Encoding": "gzip, br"})

	if got := w.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
	if got := w.Header().Get("Content-Length"); got != strconv.Itoa(w.Body.Len()) {
		t.Fatalf("Content-Length = %q, body = %d", got, w.Body.Len())
	}
	decoded, err := io.ReadAll(brotli.NewReader(bytes.NewReader(w.Body.Bytes())))
	if err != nil {
		t.Fatalf("brotli decode: %v", err)
	}
	if got, want := string(decoded), string(testStaticFS()["_next/static/chunks/app.js"].Data); got != want {
		t.Fatalf("decoded body mismatch: got %d bytes, want %d", len(got), len(want))
	}
	if w.Body.Len() >= len(testStaticFS()["_next/static/chunks/app.js"].Data) {
		t.Fatalf("compressed size %d is not smaller than raw %d", w.Body.Len(), len(testStaticFS()["_next/static/chunks/app.js"].Data))
	}
}

func TestStaticGzipFallback(t *testing.T) {
	fss := newStaticFileServer(testStaticFS())
	w := serveTest(t, fss, "_next/static/chunks/app.js", http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})

	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip decode: %v", err)
	}
	if len(decoded) != len(testStaticFS()["_next/static/chunks/app.js"].Data) {
		t.Fatalf("decoded %d bytes, want %d", len(decoded), len(testStaticFS()["_next/static/chunks/app.js"].Data))
	}
}

func TestStaticDoesNotCompressWhenNotAcceptedOrNotWorthIt(t *testing.T) {
	fss := newStaticFileServer(testStaticFS())
	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"_next/static/chunks/app.js", map[string]string{"Accept-Encoding": "identity"}},
		{"_next/static/chunks/app.js", map[string]string{"Accept-Encoding": "br;q=0, gzip;q=0"}},
		{"_next/static/media/logo.png", map[string]string{"Accept-Encoding": "br, gzip"}},
		{"_next/static/chunks/tiny.js", map[string]string{"Accept-Encoding": "br, gzip"}},
	}
	for _, tc := range cases {
		w := serveTest(t, fss, tc.name, http.MethodGet, tc.headers)
		if got := w.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("%s %v: Content-Encoding = %q, want none", tc.name, tc.headers, got)
		}
	}
}

func TestStaticCacheControl(t *testing.T) {
	fss := newStaticFileServer(testStaticFS())
	w := serveTest(t, fss, "_next/static/chunks/app.js", http.MethodGet, nil)
	if got, want := w.Header().Get("Cache-Control"), "public, max-age=31536000, immutable"; got != want {
		t.Fatalf("hashed asset Cache-Control = %q, want %q", got, want)
	}
	html := serveTest(t, fss, "index.html", http.MethodGet, nil)
	if got, want := html.Header().Get("Cache-Control"), "no-cache"; got != want {
		t.Fatalf("html Cache-Control = %q, want %q", got, want)
	}
}

func TestStaticETagRevalidation(t *testing.T) {
	fss := newStaticFileServer(testStaticFS())
	first := serveTest(t, fss, "_next/static/chunks/app.js", http.MethodGet, nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	second := serveTest(t, fss, "_next/static/chunks/app.js", http.MethodGet, map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Fatalf("304 body = %d bytes, want 0", second.Body.Len())
	}
}

func TestStaticRangeBypassesCompression(t *testing.T) {
	fss := newStaticFileServer(testStaticFS())
	w := serveTest(t, fss, "_next/static/chunks/app.js", http.MethodGet, map[string]string{
		"Accept-Encoding": "br, gzip",
		"Range":           "bytes=0-9",
	})
	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want none for range", got)
	}
	if w.Body.Len() != 10 {
		t.Fatalf("range body = %d bytes, want 10", w.Body.Len())
	}
}

func TestStaticHeadHasNoBody(t *testing.T) {
	fss := newStaticFileServer(testStaticFS())
	w := serveTest(t, fss, "_next/static/chunks/app.js", http.MethodHead, map[string]string{"Accept-Encoding": "br"})
	if w.Body.Len() != 0 {
		t.Fatalf("HEAD body = %d bytes, want 0", w.Body.Len())
	}
	if w.Header().Get("Content-Length") == "" {
		t.Fatal("HEAD missing Content-Length")
	}
}

func TestStaticMissingFile(t *testing.T) {
	fss := newStaticFileServer(testStaticFS())
	r := httptest.NewRequest(http.MethodGet, "/nope.js", nil)
	w := httptest.NewRecorder()
	if fss.serveFileIfExists(w, r, "nope.js") {
		t.Fatal("served a missing file")
	}
}

func TestCompressionCachesBodies(t *testing.T) {
	fss := newStaticFileServer(testStaticFS())
	serveTest(t, fss, "_next/static/chunks/app.js", http.MethodGet, map[string]string{"Accept-Encoding": "br"})
	a, ok := fss.load("_next/static/chunks/app.js")
	if !ok || len(a.brBody) == 0 {
		t.Fatal("brotli body was not cached")
	}
	first := a.brBody
	serveTest(t, fss, "_next/static/chunks/app.js", http.MethodGet, map[string]string{"Accept-Encoding": "br"})
	if &first[0] != &a.brBody[0] {
		t.Fatal("compressed body was recomputed")
	}
}
