// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s) 2018. All rights reserved.

package types

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/openfaas/faas/gateway/version"
)

// NewHTTPClientReverseProxy 创建一个基于http.Client的反向代理，用于转发请求到上游主机
// baseURL: 上游服务的基础URL
// timeout: 请求超时时间
// maxIdleConns: 最大空闲连接数
// maxIdleConnsPerHost: 每个主机的最大空闲连接数
func NewHTTPClientReverseProxy(baseURL *url.URL, timeout time.Duration, maxIdleConns, maxIdleConnsPerHost int) *HTTPClientReverseProxy {
	h := HTTPClientReverseProxy{
		BaseURL: baseURL,
		Timeout: timeout,
	}

	h.Client = http.DefaultClient
	h.Timeout = timeout
	h.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// 这些默认客户端的覆盖配置可实现连接复用，防止高流量下CoreDNS对网关进行速率限制
	//
	// 参考两个类似项目的配置调整：
	// https://github.com/prometheus/prometheus/pull/3592
	// https://github.com/minio/minio/pull/5860

	// 基于Go 1.11的http.DefaultTransport配置
	h.Client.Transport = &proxyTransport{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: timeout,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	return &h
}

// HTTPClientReverseProxy 使用http.Client将请求代理到远程BaseURL
type HTTPClientReverseProxy struct {
	BaseURL *url.URL
	Client  *http.Client
	Timeout time.Duration
}

// proxyTransport 是反向代理客户端的http.RoundTripper实现
// 用于确保请求设置默认头（如User-Agent）
type proxyTransport struct {
	// Transport 是发起请求时使用的底层HTTP传输层
	Transport http.RoundTripper
}

// RoundTrip 实现RoundTripper接口，为请求设置默认User-Agent头
func (t *proxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, ok := req.Header["User-Agent"]; !ok {
		req.Header.Set("User-Agent", fmt.Sprintf("openfaas-ce-gateway/%s", version.BuildVersion()))
	}

	return t.Transport.RoundTrip(req)
}
