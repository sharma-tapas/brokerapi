package x_region_header

import (
	"context"
	"net/http"
	"regexp"
)

const (
	xRegionKey = "X-Region"
)

//AddToContext the X-*-Region to the context
func AddToContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		pattern := regexp.MustCompile(`X([-]*[a-zA-Z]*)-Region`)
		value := ""
		for k := range req.Header {
			res := pattern.MatchString(k)
			if res {
				value = req.Header.Get(k)
				break
			}
		}
		newCtx := context.WithValue(req.Context(), xRegionKey, value)
		next.ServeHTTP(w, req.WithContext(newCtx))
	})
}
