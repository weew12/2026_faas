// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

package handlers

import "net/http"

// CORSHandler 为上游处理器添加跨域资源共享（CORS）响应头
type CORSHandler struct {
	Upstream    *http.Handler // 上游原始HTTP处理器
	AllowedHost string        // 允许跨域访问的源域名
}

// ServeHTTP 注入CORS响应头，然后调用上游处理器处理请求
func (c CORSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 设置允许的请求头
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	// 设置允许的HTTP方法
	w.Header().Set("Access-Control-Allow-Methods", http.MethodGet)
	// 设置允许跨域的源
	w.Header().Set("Access-Control-Allow-Origin", c.AllowedHost)

	// 委托给上游处理器继续处理请求
	(*c.Upstream).ServeHTTP(w, r)
}

// DecorateWithCORS 为指定的处理器包装CORS中间件
// upstream: 需要被包装的原始处理器
// allowedHost: 允许跨域的域名
// 返回: 包装后的CORS处理器
func DecorateWithCORS(upstream http.Handler, allowedHost string) http.Handler {
	return CORSHandler{
		Upstream:    &upstream,
		AllowedHost: allowedHost,
	}
}
