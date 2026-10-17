package middleware

import "net/http"

// Auth 是认证中间件的占位实现，后续阶段接入 JWT 或 API Key
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: 阶段 4 实现真正的认证逻辑
		next.ServeHTTP(w, r)
	})
}
