package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

// cacheablePaths are the read endpoints whose representation is a pure,
// deterministic function of (catalog revision, build identity, request) —
// search, the vendor directory, and preset detail. Everything else (health,
// uploads, claims, /v1/me) either changes independently of the catalog or has
// side effects, so it is never revision-cached.
func cacheableGET(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	switch p := r.URL.Path; {
	case p == BasePath+"/presets", p == BasePath+"/vendors":
		return true
	case strings.HasPrefix(p, BasePath+"/presets/"):
		// GET /v1/presets/{id}, but not a future nested route under it
		// (e.g. .../download) — those are a different representation.
		return !strings.Contains(p[len(BasePath+"/presets/"):], "/")
	default:
		return false
	}
}

// revisionCache wraps next so cacheable GETs carry X-Catalog-Revision and a
// strong ETag derived from the served catalog's Revision + BuildID + the
// request URL, and a matching If-None-Match short-circuits to 304 without
// running the handler at all. See docs/api-surface.md ("Catalog Revision and
// Caching").
//
// The ETag needs no response body to compute: because the representation is
// an exact, deterministic function of its inputs (nothing here depends on
// wall-clock time or randomness), hashing those inputs is equivalent to
// hashing the output. That is what lets a cache hit skip the handler entirely
// instead of recomputing a search or re-marshalling a preset only to throw it
// away.
func revisionCache(holder *catalog.Holder, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := holder.Current()
		if current == nil || !cacheableGET(r) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-Catalog-Revision", current.Revision)
		w.Header().Set("Cache-Control", "no-cache")

		etag := computeETag(current.Revision, current.BuildID, r.URL.String())
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		next.ServeHTTP(&etagWriter{ResponseWriter: w, etag: etag}, r)
	})
}

// computeETag derives a strong ETag from the catalog's revision and build
// identity plus the request URL (path + query), so two different pages,
// filters, or preset ids never collide, and a cursor pinned to a stale
// revision never appears fresh. Truncated to 16 bytes: this identifies a
// representation, not a security boundary.
func computeETag(revision, buildID, requestURI string) string {
	sum := sha256.Sum256([]byte(revision + "\x00" + buildID + "\x00" + requestURI))
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// etagWriter sets the ETag response header the first time the wrapped
// handler commits a 200 OK, and leaves any other status (403, 409, 503, ...)
// unlabelled — an error response is not the cacheable representation the
// ETag names, so it must never be revalidated as if it were.
type etagWriter struct {
	http.ResponseWriter
	etag        string
	wroteHeader bool
}

func (w *etagWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if status == http.StatusOK {
			w.Header().Set("ETag", w.etag)
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *etagWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
