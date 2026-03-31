// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) Alex Ellis 2017. All rights reserved.

package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	ftypes "github.com/openfaas/faas-provider/types"
	"github.com/openfaas/faas/gateway/metrics"
	"github.com/openfaas/faas/gateway/pkg/middleware"
	"github.com/openfaas/faas/gateway/scaling"
)

// MakeQueuedProxy 创建一个异步队列代理处理器
// 将收到的 HTTP 请求放入队列，实现函数异步调用，立即返回 202 Accepted
func MakeQueuedProxy(metrics metrics.MetricOptions, queuer ftypes.RequestQueuer, pathTransformer middleware.URLPathTransformer, defaultNS string, functionQuery scaling.FunctionQuery) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 读取请求体数据
		var body []byte
		if r.Body != nil {
			defer r.Body.Close()

			var err error
			body, err = io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		// 解析回调 URL（可选）
		callbackURL, err := getCallbackURLHeader(r.Header)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// 从路由变量中获取函数名
		vars := mux.Vars(r)
		name := vars["name"]

		// 构建队列请求对象
		req := &ftypes.QueueRequest{
			Function:    name,
			Body:        body,
			Method:      r.Method,
			QueryString: r.URL.RawQuery,
			Path:        pathTransformer.Transform(r),
			Header:      r.Header,
			Host:        r.Host,
			CallbackURL: callbackURL,
		}

		// 将请求加入队列
		if err = queuer.Queue(req); err != nil {
			log.Printf("Error queuing request: %v", err)
			http.Error(w, fmt.Sprintf("Error queuing request: %s", err.Error()),
				http.StatusInternalServerError)
			return
		}

		// 请求已入队，返回 202 Accepted
		w.WriteHeader(http.StatusAccepted)
	}
}

// getCallbackURLHeader 从请求头中解析 X-Callback-Url
// 用于异步任务完成后回调通知
func getCallbackURLHeader(header http.Header) (*url.URL, error) {
	value := header.Get("X-Callback-Url")
	var callbackURL *url.URL

	if len(value) > 0 {
		urlVal, err := url.Parse(value)
		if err != nil {
			return callbackURL, err
		}

		callbackURL = urlVal
	}

	return callbackURL, nil
}

// getNameParts 从函数字符串中解析出函数名和命名空间
// 格式：functionName.namespace
func getNameParts(name string) (fn, ns string) {
	fn = name
	ns = ""

	// 以最后一个 . 作为分隔符
	if index := strings.LastIndex(name, "."); index > 0 {
		fn = name[:index]
		ns = name[index+1:]
	}
	return fn, ns
}
