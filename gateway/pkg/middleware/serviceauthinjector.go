// Package middleware 提供HTTP中间件相关接口，包含认证信息注入能力
package middleware

import "net/http"

// AuthInjector 认证信息注入器接口
// 用于将认证信息注入到将要被代理/发送到远程上游服务的HTTP请求中
type AuthInjector interface {
	// Inject 将认证信息注入到 HTTP 请求中
	Inject(r *http.Request)
}
