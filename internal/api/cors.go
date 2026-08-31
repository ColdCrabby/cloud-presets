package api

import "net/http"

// DefaultAllowedOrigins are the cross-origin web apps permitted to call the API
// from a browser. The catalog and its two Angular apps are otherwise served
// single-origin (see cmd/server withFrontends), so this list is only the
// external consumers of the public API.
var DefaultAllowedOrigins = []string{
	"https://slicer.maxsopp.de",
}

// corsAllowMethods is the set of methods advertised to browsers in a preflight
// response. It mirrors probeMethods so any route the API can serve is allowed.
const corsAllowMethods = "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS"

// CORS wraps next with the cross-origin resource sharing headers a browser
// requires before a page served from one of allowed may read API responses.
//
// Requests without an Origin, or whose Origin is not in allowed, pass through
// untouched, so same-origin and non-browser callers are unaffected. A preflight
// (an OPTIONS request carrying Access-Control-Request-Method) from an allowed
// origin is answered here with 204 and never reaches next.
func CORS(allowed []string, next http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		set[o] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		// The response varies by Origin whether or not this one is allowed, so
		// caches must key on it to avoid serving one origin's headers to another.
		w.Header().Add("Vary", "Origin")

		if _, ok := set[origin]; !ok {
			next.ServeHTTP(w, r)
			return
		}

		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			h.Set("Access-Control-Allow-Methods", corsAllowMethods)
			reqHeaders := r.Header.Get("Access-Control-Request-Headers")
			if reqHeaders == "" {
				reqHeaders = "Authorization, Content-Type"
			}
			h.Set("Access-Control-Allow-Headers", reqHeaders)
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
