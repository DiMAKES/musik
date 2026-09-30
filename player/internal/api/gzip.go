package api

import (
	"compress/gzip"
	"net/http"
	"strings"
)

func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz       *gzip.Writer
	chose    bool
	compress bool
}

func compressibleType(ct string) bool {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	switch strings.TrimSpace(strings.ToLower(ct)) {
	case "text/html", "text/css", "text/plain", "text/javascript",
		"application/javascript", "application/json",
		"application/manifest+json", "application/xml", "text/xml":
		return true
	default:
		return false
	}
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if !g.chose {
		g.decide(code)
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) decide(code int) {
	if g.chose {
		return
	}
	g.chose = true
	if code < 200 || code == http.StatusNoContent || code == http.StatusNotModified {
		return
	}
	if g.Header().Get("Content-Encoding") != "" {
		return
	}
	if !compressibleType(g.Header().Get("Content-Type")) {
		return
	}
	g.compress = true
	g.Header().Del("Content-Length")
	g.Header().Set("Content-Encoding", "gzip")
	g.Header().Add("Vary", "Accept-Encoding")
	g.gz = gzip.NewWriter(g.ResponseWriter)
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	if !g.chose {
		g.decide(http.StatusOK)
	}
	if g.compress && g.gz != nil {
		return g.gz.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

func (g *gzipResponseWriter) close() {
	if g.gz != nil {
		_ = g.gz.Close()
		g.gz = nil
	}
}

func (g *gzipResponseWriter) Flush() {
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}
