package middleware

import (
	"net/http"
)

func AddCORSHeaders(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		res.Header().Set("Access-Control-Allow-Headers", "*")
		res.Header().Set("Access-Control-Request-Headers", "*")
		res.Header().Set("Access-Control-Allow-Methods", "POST")

		next.ServeHTTP(res, req)
	})
}
