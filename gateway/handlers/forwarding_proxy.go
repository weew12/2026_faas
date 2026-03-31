// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync"
	"time"

	fhttputil "github.com/openfaas/faas-provider/httputil"
	"github.com/openfaas/faas/gateway/pkg/middleware"
	"github.com/openfaas/faas/gateway/types"
)

// MakeForwardingProxyHandler 创建一个HTTP请求转发代理处理器
// 将网关收到的请求转发到对应的函数服务，支持超时、认证、通知、路径重写等能力
func MakeForwardingProxyHandler(proxy *types.HTTPClientReverseProxy,
	notifiers []HTTPNotifier,
	baseURLResolver middleware.BaseURLResolver,
	urlPathTransformer middleware.URLPathTransformer,
	serviceAuthInjector middleware.AuthInjector) http.HandlerFunc {

	// 从环境变量判断是否打印请求URI
	writeRequestURI := false
	if _, exists := os.LookupEnv("write_request_uri"); exists {
		writeRequestURI = exists
	}

	// 创建路径重写的反向代理
	reverseProxy := makeRewriteProxy(baseURLResolver, urlPathTransformer)

	return func(w http.ResponseWriter, r *http.Request) {

		// 解析目标服务基础URL
		baseURL := baseURLResolver.Resolve(r)
		originalURL := r.URL.String()
		// 转换请求路径
		requestURL := urlPathTransformer.Transform(r)

		// 触发请求开始通知
		for _, notifier := range notifiers {
			notifier.Notify(r.Method, requestURL, originalURL, http.StatusProcessing, "started", time.Second*0)
		}

		start := time.Now()

		// 转发请求到上游函数
		statusCode, err := forwardRequest(w, r, proxy.Client, baseURL, requestURL, proxy.Timeout, writeRequestURI, serviceAuthInjector, reverseProxy)
		if err != nil {
			log.Printf("error with upstream request to: %s, %s\n", requestURL, err.Error())
		}

		seconds := time.Since(start)

		// 触发请求完成通知
		for _, notifier := range notifiers {
			notifier.Notify(r.Method, requestURL, originalURL, statusCode, "completed", seconds)
		}
	}
}

// buildUpstreamRequest 构建转发到上游函数的HTTP请求
// 复制请求头、添加转发头、拼接URL
func buildUpstreamRequest(r *http.Request, baseURL string, requestURL string) *http.Request {
	url := baseURL + requestURL

	// 拼接查询参数
	if len(r.URL.RawQuery) > 0 {
		url = fmt.Sprintf("%s?%s", url, r.URL.RawQuery)
	}

	upstreamReq, _ := http.NewRequest(r.Method, url, nil)

	// 复制请求头
	copyHeaders(upstreamReq.Header, &r.Header)
	// 删除端到端不需要的跳头
	deleteHeaders(&upstreamReq.Header, &hopHeaders)

	// 添加转发相关头
	if len(r.Host) > 0 && upstreamReq.Header.Get("X-Forwarded-Host") == "" {
		upstreamReq.Header["X-Forwarded-Host"] = []string{r.Host}
	}

	if upstreamReq.Header.Get("X-Forwarded-For") == "" {
		upstreamReq.Header["X-Forwarded-For"] = []string{r.RemoteAddr}
	}

	// 传递请求体
	if r.Body != nil {
		upstreamReq.Body = r.Body
	}

	return upstreamReq
}

// forwardRequest 执行请求转发
// 处理普通请求与事件流（SSE）请求，支持认证注入、超时控制
func forwardRequest(w http.ResponseWriter,
	r *http.Request,
	proxyClient *http.Client,
	baseURL string,
	requestURL string,
	timeout time.Duration,
	writeRequestURI bool,
	serviceAuthInjector middleware.AuthInjector,
	reverseProxy *httputil.ReverseProxy) (int, error) {

	if r.Body != nil {
		defer r.Body.Close()
	}

	// 构建上游请求
	upstreamReq := buildUpstreamRequest(r, baseURL, requestURL)

	// 注入服务认证信息
	if serviceAuthInjector != nil {
		serviceAuthInjector.Inject(upstreamReq)
	}

	// 打印调试日志
	if writeRequestURI {
		log.Printf("forwardRequest: %s %s\n", upstreamReq.Host, upstreamReq.URL.String())
	}

	// 处理事件流请求（SSE）
	if strings.HasPrefix(r.Header.Get("Accept"), "text/event-stream") {
		return handleEventStream(w, r, reverseProxy, upstreamReq, timeout)
	}

	// 普通请求：设置超时并发送
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	res, err := proxyClient.Do(upstreamReq.WithContext(ctx))
	if err != nil {
		badStatus := http.StatusBadGateway
		w.WriteHeader(badStatus)
		return badStatus, err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	// 复制响应头并返回结果
	copyHeaders(w.Header(), &res.Header)
	w.WriteHeader(res.StatusCode)

	if res.Body != nil {
		io.Copy(w, res.Body)
	}

	return res.StatusCode, nil
}

// handleEventStream 专门处理SSE（Server-Sent Events）长连接请求
// 使用反向代理保持长连接，处理panic与请求取消
func handleEventStream(w http.ResponseWriter, r *http.Request, reverseProxy *httputil.ReverseProxy, upstreamReq *http.Request, timeout time.Duration) (int, error) {
	ww := fhttputil.NewHttpWriteInterceptor(w)

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeoutCause(r.Context(), timeout, http.ErrHandlerTimeout)
	defer cancel()

	r = r.WithContext(ctx)
	wg := &sync.WaitGroup{}
	wg.Add(1)

	// 异步运行反向代理
	go func() {
		defer func() {
			wg.Done()
			// 捕获panic，避免服务崩溃
			if r := recover(); r != nil {
				if errors.Is(r.(error), http.ErrAbortHandler) {
					log.Printf("Aborted [%s] for: %s", upstreamReq.Method, upstreamReq.URL.Path)
				} else {
					log.Printf("Recovered from panic in reverseproxy: %v", r)
				}
			}
		}()

		reverseProxy.ServeHTTP(ww, r)
	}()

	wg.Wait()

	return ww.Status(), nil
}

// copyHeaders 深度复制HTTP请求头
func copyHeaders(destination http.Header, source *http.Header) {
	for k, v := range *source {
		vClone := make([]string, len(v))
		copy(vClone, v)
		(destination)[k] = vClone
	}
}

// deleteHeaders 删除指定的请求头
func deleteHeaders(target *http.Header, exclude *[]string) {
	for _, h := range *exclude {
		target.Del(h)
	}
}

// hopHeaders 定义端到端转发时需要移除的跳头
// 遵循RFC 7230标准，反向代理必须移除这些头
var hopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// makeRewriteProxy 创建支持路径重写的标准库反向代理
func makeRewriteProxy(baseURLResolver middleware.BaseURLResolver, urlPathTransformer middleware.URLPathTransformer) *httputil.ReverseProxy {

	return &httputil.ReverseProxy{
		// 禁用代理错误日志
		ErrorLog: log.New(io.Discard, "proxy:", 0),
		// 空错误处理器
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {},
		// 请求定向器：修改请求目标与路径
		Director: func(r *http.Request) {
			baseURL := baseURLResolver.Resolve(r)
			baseURLu, _ := r.URL.Parse(baseURL)

			requestURL := urlPathTransformer.Transform(r)

			r.URL.Scheme = "http"
			r.URL.Path = requestURL
			r.URL.Host = baseURLu.Host
		},
	}
}
