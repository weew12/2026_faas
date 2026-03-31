// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"

	providerTypes "github.com/openfaas/faas-provider/types"
	"github.com/openfaas/faas/gateway/types"
	"github.com/openfaas/faas/gateway/version"
)

// MakeInfoHandler 创建用于展示组件版本信息的 HTTP 处理器
// 合并网关自身版本与底层提供商版本信息，统一对外输出
func MakeInfoHandler(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 使用响应记录器捕获上游服务的返回结果
		responseRecorder := httptest.NewRecorder()
		h.ServeHTTP(responseRecorder, r)
		upstreamCall := responseRecorder.Result()

		defer upstreamCall.Body.Close()

		var provider *providerTypes.ProviderInfo

		// 读取并解析上游提供商的版本信息
		upstreamBody, _ := io.ReadAll(upstreamCall.Body)
		err := json.Unmarshal(upstreamBody, &provider)
		if err != nil {
			log.Printf("Error unmarshalling provider json from body %s. Error %s\n", upstreamBody, err.Error())
		}

		// 组装网关完整版本信息
		gatewayInfo := &types.GatewayInfo{
			Version: &providerTypes.VersionInfo{
				CommitMessage: version.GitCommitMessage,
				Release:       version.BuildVersion(),
				SHA:           version.GitCommitSHA,
			},
			Provider: provider,
			Arch:     types.Arch,
		}

		// 序列化为 JSON
		jsonOut, marshalErr := json.Marshal(gatewayInfo)
		if marshalErr != nil {
			log.Printf("Error during unmarshal of gateway info request %s\n", marshalErr.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 返回 JSON 响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonOut)
	}
}
