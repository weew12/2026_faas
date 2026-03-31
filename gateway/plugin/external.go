// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) Alex Ellis 2017. All rights reserved.

// Package plugin 提供外部服务查询插件，通过HTTP将服务查询代理到外部提供商
package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	types "github.com/openfaas/faas-provider/types"
	middleware "github.com/openfaas/faas/gateway/pkg/middleware"
	"github.com/openfaas/faas/gateway/scaling"
)

// ExternalServiceQuery 通过HTTP将服务查询代理到外部插件
type ExternalServiceQuery struct {
	URL          url.URL                 // 外部服务提供商的URL
	ProxyClient  http.Client             // 用于代理请求的HTTP客户端
	AuthInjector middleware.AuthInjector // 用于注入认证信息的注入器

	// IncludeUsage 是否在响应中包含使用指标
	IncludeUsage bool
}

// NewExternalServiceQuery 创建一个通过HTTP代理服务查询到外部插件的实例
// externalURL: 外部服务提供商的URL
// authInjector: 认证信息注入器
// 返回值: 实现了scaling.ServiceQuery接口的ExternalServiceQuery实例
func NewExternalServiceQuery(externalURL url.URL, authInjector middleware.AuthInjector) scaling.ServiceQuery {
	timeout := 3 * time.Second

	proxyClient := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 0,
			}).DialContext,
			MaxIdleConns:          1,
			DisableKeepAlives:     true,
			IdleConnTimeout:       120 * time.Millisecond,
			ExpectContinueTimeout: 1500 * time.Millisecond,
		},
	}

	return ExternalServiceQuery{
		URL:          externalURL,
		ProxyClient:  proxyClient,
		AuthInjector: authInjector,
		IncludeUsage: false,
	}
}

// GetReplicas 获取函数的副本数
// serviceName: 服务名称
// serviceNamespace: 服务所在的命名空间
// 返回值: 包含副本信息的ServiceQueryResponse，以及查询过程中可能出现的错误
func (s ExternalServiceQuery) GetReplicas(serviceName, serviceNamespace string) (scaling.ServiceQueryResponse, error) {
	start := time.Now()

	var err error
	var emptyServiceQueryResponse scaling.ServiceQueryResponse

	function := types.FunctionStatus{}

	// 构建请求URL
	urlPath := fmt.Sprintf("%ssystem/function/%s?namespace=%s&usage=%v",
		s.URL.String(),
		serviceName,
		serviceNamespace,
		s.IncludeUsage)

	// 创建HTTP请求
	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return emptyServiceQueryResponse, err
	}

	// 注入认证信息
	if s.AuthInjector != nil {
		s.AuthInjector.Inject(req)
	}

	// 执行请求
	res, err := s.ProxyClient.Do(req)
	if err != nil {
		log.Println(urlPath, err)
		return emptyServiceQueryResponse, err

	}

	// 读取响应体
	var bytesOut []byte
	if res.Body != nil {
		bytesOut, _ = io.ReadAll(res.Body)
		defer res.Body.Close()
	}

	// 处理响应
	if res.StatusCode == http.StatusOK {
		// 解析响应JSON
		if err := json.Unmarshal(bytesOut, &function); err != nil {
			log.Printf("Unable to unmarshal: %q, %s", string(bytesOut), err)
			return emptyServiceQueryResponse, err
		}

		// log.Printf("GetReplicas [%s.%s] took: %fs", serviceName, serviceNamespace, time.Since(start).Seconds())

	} else {
		// 非200状态码处理
		log.Printf("GetReplicas [%s.%s] took: %.4fs, code: %d\n", serviceName, serviceNamespace, time.Since(start).Seconds(), res.StatusCode)
		return emptyServiceQueryResponse, fmt.Errorf("server returned non-200 status code (%d) for function, %s, body: %s", res.StatusCode, serviceName, string(bytesOut))
	}

	// 设置默认值
	minReplicas := uint64(scaling.DefaultMinReplicas)
	maxReplicas := uint64(scaling.DefaultMaxReplicas)
	scalingFactor := uint64(scaling.DefaultScalingFactor)
	availableReplicas := function.AvailableReplicas

	// 从标签中提取配置值
	if function.Labels != nil {
		labels := *function.Labels

		minReplicas = extractLabelValue(labels[scaling.MinScaleLabel], minReplicas)
		maxReplicas = extractLabelValue(labels[scaling.MaxScaleLabel], maxReplicas)
		extractedScalingFactor := extractLabelValue(labels[scaling.ScalingFactorLabel], scalingFactor)

		// 验证缩放因子范围
		if extractedScalingFactor > 0 && extractedScalingFactor <= 100 {
			scalingFactor = extractedScalingFactor
		} else {
			return scaling.ServiceQueryResponse{}, fmt.Errorf("bad scaling factor: %d, is not in range of [0 - 100]", extractedScalingFactor)
		}
	}

	return scaling.ServiceQueryResponse{
		Replicas:          function.Replicas,
		MaxReplicas:       maxReplicas,
		MinReplicas:       minReplicas,
		ScalingFactor:     scalingFactor,
		AvailableReplicas: availableReplicas,
		Annotations:       function.Annotations,
	}, err
}

// SetReplicas 更新函数的副本数
// serviceName: 服务名称
// serviceNamespace: 服务所在的命名空间
// count: 要设置的目标副本数
// 返回值: 设置过程中可能出现的错误
func (s ExternalServiceQuery) SetReplicas(serviceName, serviceNamespace string, count uint64) error {
	var err error

	// 构建缩放请求
	scaleReq := types.ScaleServiceRequest{
		ServiceName: serviceName,
		Replicas:    count,
	}

	// 序列化请求体
	requestBody, err := json.Marshal(scaleReq)
	if err != nil {
		return err
	}

	start := time.Now()
	// 构建请求URL
	urlPath := fmt.Sprintf("%ssystem/scale-function/%s?namespace=%s", s.URL.String(), serviceName, serviceNamespace)
	req, _ := http.NewRequest(http.MethodPost, urlPath, bytes.NewReader(requestBody))

	// 注入认证信息
	if s.AuthInjector != nil {
		s.AuthInjector.Inject(req)
	}

	defer req.Body.Close()
	// 执行请求
	res, err := s.ProxyClient.Do(req)

	if err != nil {
		log.Println(urlPath, err)
	} else {
		if res.Body != nil {
			defer res.Body.Close()
		}
	}

	// 检查响应状态码
	if !(res.StatusCode == http.StatusOK || res.StatusCode == http.StatusAccepted) {
		err = fmt.Errorf("error scaling HTTP code %d, %s", res.StatusCode, urlPath)
	}

	log.Printf("SetReplicas [%s.%s] took: %.4fs",
		serviceName, serviceNamespace, time.Since(start).Seconds())

	return err
}

// extractLabelValue 解析提供的原始标签值，如果解析失败则返回提供的默认值并记录日志
// rawLabelValue: 原始标签值
// fallback: 解析失败时的默认值
// 返回值: 解析后的标签值，或默认值
func extractLabelValue(rawLabelValue string, fallback uint64) uint64 {
	if len(rawLabelValue) <= 0 {
		return fallback
	}

	value, err := strconv.Atoi(rawLabelValue)

	if err != nil {
		log.Printf("Provided label value %s should be of type uint", rawLabelValue)
		return fallback
	}

	return uint64(value)
}
