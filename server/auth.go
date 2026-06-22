package server

import (
	"net/http"
	"strings"
)

func bearerAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		provided := strings.TrimPrefix(hdr, "Bearer ")
		if provided == "" || provided != token {
			jsonError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
