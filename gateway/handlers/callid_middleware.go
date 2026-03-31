// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/docker/distribution/uuid"
	"github.com/openfaas/faas/gateway/version"
)

// MakeCallIDMiddleware 创建请求ID中间件，为每个HTTP请求添加唯一标识与追踪信息
// next: 后续要执行的HTTP处理函数
// 返回值: 包装后的HTTP处理函数
func MakeCallIDMiddleware(next http.HandlerFunc) http.HandlerFunc {

	version := version.Version

	return func(w http.ResponseWriter, r *http.Request) {
		// 记录请求开始时间，用于追踪耗时
		start := time.Now()

		// 若请求未携带X-Call-Id，则自动生成UUID作为唯一请求ID
		if len(r.Header.Get("X-Call-Id")) == 0 {
			callID := uuid.Generate().String()
			r.Header.Add("X-Call-Id", callID)
			w.Header().Add("X-Call-Id", callID)
		}

		// 将请求开始时间（纳秒级）写入请求头与响应头
		r.Header.Add("X-Start-Time", fmt.Sprintf("%d", start.UTC().UnixNano()))
		w.Header().Add("X-Start-Time", fmt.Sprintf("%d", start.UTC().UnixNano()))

		// 标记服务来源，便于问题排查
		w.Header().Add("X-Served-By", fmt.Sprintf("openfaas-ce/%s", version))

		// 执行后续处理逻辑
		next(w, r)
	}
}
