// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

// Package middleware 提供HTTP中间件组件，包含基础认证凭证注入功能
package middleware

import (
	"net/http"

	"github.com/openfaas/faas-provider/auth"
)

// BasicAuthInjector 基础认证注入器，用于将基础认证凭证注入到HTTP请求中
type BasicAuthInjector struct {
	Credentials *auth.BasicAuthCredentials // 基础认证凭证
}

// Inject 为HTTP请求设置基础认证头
// r: 需要注入认证信息的HTTP请求
func (b BasicAuthInjector) Inject(r *http.Request) {
	if r != nil && b.Credentials != nil {
		r.SetBasicAuth(b.Credentials.User, b.Credentials.Password)
	}
}
