package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// probeMethods are the HTTP methods checked when deciding whether an unmatched
// request is a 404 (no such resource) or a 405 (resource exists, method not
// allowed) and, if 405, which methods to advertise in the Allow header.
var probeMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

// problemRouter wraps an http.ServeMux so that requests it cannot route render
// as RFC 9457 application/problem+json, matching every other error the API
// emits, instead of the ServeMux plain-text defaults ("404 page not found",
// "Method Not Allowed"). See docs/api-surface.md ("Error Model").
type problemRouter struct {
	mux *http.ServeMux
}

func (pr *problemRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, pattern := pr.mux.Handler(r); pattern != "" {
		pr.mux.ServeHTTP(w, r)
		return
	}

	// No handler for this method+path. Distinguish a wrong method on an
	// existing path (405) from an unknown path (404) by probing other methods.
	if allowed := pr.allowedMethods(r.URL.Path); len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		writeProblem(w, http.StatusMethodNotAllowed,
			fmt.Sprintf("method %s is not allowed on %s", r.Method, r.URL.Path))
		return
	}
	writeProblem(w, http.StatusNotFound, "no resource matches "+r.URL.Path)
}

// allowedMethods returns the methods that route to a handler for path.
func (pr *problemRouter) allowedMethods(path string) []string {
	var allowed []string
	for _, m := range probeMethods {
		probe := &http.Request{Method: m, URL: &url.URL{Path: path}}
		if _, pattern := pr.mux.Handler(probe); pattern != "" {
			allowed = append(allowed, m)
		}
	}
	return allowed
}

// writeProblem renders an RFC 9457 problem document using Huma's error model,
// so hand-written error responses share the exact shape Huma generates for
// operation errors.
func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(huma.NewError(status, detail))
}
