// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s) 2018. All rights reserved.

package handlers

import (
	"net/http"
	"time"

	"github.com/openfaas/faas-provider/httputil"
)

// MakeNotifierWrapper 将 http.HandlerFunc 包装成拦截器
// 用于在请求处理完成后通知所有 HTTPNotifier
func MakeNotifierWrapper(next http.HandlerFunc, notifiers []HTTPNotifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 记录请求开始时间
		then := time.Now()
		// 记录请求的URL
		url := r.URL.String()

		// 使用拦截器包装响应写入器，用于捕获状态码
		writer := httputil.NewHttpWriteInterceptor(w)
		// 执行实际的处理函数
		next(writer, r)

		// 遍历所有通知器，发送请求完成事件
		for _, notifier := range notifiers {
			notifier.Notify(
				r.Method,
				url,
				url,
				writer.Status(),
				"completed",
				time.Since(then),
			)
		}
	}
}
