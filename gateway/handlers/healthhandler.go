// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s) 2019. All rights reserved.

package handlers

import "net/http"

// HealthzHandler 健康检查处理器
// 用于指标服务、K8s 等平台检测服务是否存活
func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	// 仅允许 GET 请求
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
		break

	// 其他请求方法返回 405
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
