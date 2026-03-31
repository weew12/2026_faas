// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

package handlers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/openfaas/faas/gateway/pkg/middleware"
	"github.com/openfaas/faas/gateway/scaling"
)

// MakeScalingHandler 创建一个**自动扩缩容中间件**
// 作用：在请求转发前，先将函数从 0 副本扩容至 N 副本
// 等待函数就绪后再执行后续处理器；若超时未就绪，则直接返回错误给客户端
func MakeScalingHandler(next http.HandlerFunc, scaler scaling.FunctionScaler, config scaling.ScalingConfig, defaultNamespace string) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		// 1. 从请求 URL 中解析出函数名 + 命名空间
		functionName, namespace := middleware.GetNamespace(defaultNamespace, middleware.GetServiceName(r.URL.String()))

		// 2. 执行扩容逻辑（0 → N）
		res := scaler.Scale(functionName, namespace)

		// 3. 函数不存在 → 404
		if !res.Found {
			errStr := fmt.Sprintf("error finding function %s.%s: %s", functionName, namespace, res.Error.Error())
			log.Printf("Scaling: %s\n", errStr)

			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(errStr))
			return
		}

		// 4. 扩容过程出错 → 500
		if res.Error != nil {
			errStr := fmt.Sprintf("error finding function %s.%s: %s", functionName, namespace, res.Error.Error())
			log.Printf("Scaling: %s\n", errStr)

			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(errStr))
			return
		}

		// 5. 函数已就绪 → 放行请求，执行后续代理逻辑
		if res.Available {
			next.ServeHTTP(w, r)
			return
		}

		// 6. 扩容超时，函数未就绪 → 不处理请求，记录超时日志
		log.Printf("\033[33mScaling: \033[0m")
		log.Printf("[Scale] function=%s.%s 0=>N timed-out after %.4fs\n",
			functionName, namespace, res.Duration.Seconds())
	}
}
