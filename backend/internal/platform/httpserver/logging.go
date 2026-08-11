package httpserver

import "net/http"

// redactWSToken strips the `token` query parameter from the request's
// RequestURI before chimiddleware.Logger logs it, so a WebSocket
// handshake's access token never reaches stdout/log aggregators. It
// must run before Logger in the middleware chain (mutates r.RequestURI
// in place, which Logger reads).
func redactWSToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			if q := r.URL.Query(); q.Has("token") {
				q.Set("token", "REDACTED")
				redacted := *r.URL
				redacted.RawQuery = q.Encode()
				r.RequestURI = redacted.RequestURI()
			}
		}
		next.ServeHTTP(w, r)
	})
}
