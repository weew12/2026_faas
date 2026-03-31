// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)
// Copyright (c) Alex Ellis 2017
// Copyright (c) 2018 OpenFaaS Author(s)

package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"time"

	"log"

	"github.com/openfaas/faas-provider/auth"
	types "github.com/openfaas/faas-provider/types"
	"github.com/prometheus/client_golang/prometheus"
)

// Exporter 实现 Prometheus 指标收集器接口
// 负责采集函数副本数、调用次数、启动次数等监控指标
type Exporter struct {
	metricOptions     MetricOptions              // 指标配置项
	services          []types.FunctionStatus     // 函数状态列表（缓存）
	credentials       *auth.BasicAuthCredentials // 认证信息
	FunctionNamespace string                     // 默认函数命名空间
}

// NewExporter 创建 OpenFaaS 网关 Prometheus 指标收集器
func NewExporter(options MetricOptions, credentials *auth.BasicAuthCredentials, namespace string) *Exporter {
	return &Exporter{
		metricOptions:     options,
		services:          []types.FunctionStatus{},
		credentials:       credentials,
		FunctionNamespace: namespace,
	}
}

// Describe 实现 prometheus.Collector 接口
// 向 Prometheus 描述所有暴露的指标
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.metricOptions.GatewayFunctionInvocation.Describe(ch)
	e.metricOptions.GatewayFunctionsHistogram.Describe(ch)
	e.metricOptions.ServiceReplicasGauge.Describe(ch)
	e.metricOptions.GatewayFunctionInvocationStarted.Describe(ch)
}

// Collect 实现 prometheus.Collector 接口
// 采集并推送指标数据给 Prometheus
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	// 上报调用次数、耗时直方图、启动次数指标
	e.metricOptions.GatewayFunctionInvocation.Collect(ch)
	e.metricOptions.GatewayFunctionsHistogram.Collect(ch)
	e.metricOptions.GatewayFunctionInvocationStarted.Collect(ch)

	// 重置副本数 gauge，重新填充当前最新值
	e.metricOptions.ServiceReplicasGauge.Reset()
	for _, service := range e.services {
		var serviceName string
		if len(service.Namespace) > 0 {
			serviceName = fmt.Sprintf("%s.%s", service.Name, service.Namespace)
		} else {
			serviceName = service.Name
		}

		// 设置函数当前副本数
		e.metricOptions.ServiceReplicasGauge.
			WithLabelValues(serviceName).
			Set(float64(service.Replicas))
	}

	// 上报副本数指标
	e.metricOptions.ServiceReplicasGauge.Collect(ch)
}

// StartServiceWatcher 启动定时任务
// 定期从 OpenFaaS 后端获取函数列表，更新副本数指标
func (e *Exporter) StartServiceWatcher(endpointURL url.URL, metricsOptions MetricOptions, label string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				// 获取所有命名空间
				namespaces, err := e.getNamespaces(endpointURL)
				if err != nil {
					log.Printf("Error listing namespaces: %s", err)
				}

				services := []types.FunctionStatus{}

				// 无命名空间（如 faasd）
				if len(namespaces) == 0 {
					services, err = e.getFunctions(endpointURL, e.FunctionNamespace)
					if err != nil {
						log.Printf("Error getting functions from: %s, error: %s", e.FunctionNamespace, err)
						continue
					}
					e.services = services
				} else {
					// 遍历所有命名空间，获取函数列表
					for _, namespace := range namespaces {
						nsServices, err := e.getFunctions(endpointURL, namespace)
						if err != nil {
							log.Printf("Error getting functions from: %s, error: %s", namespace, err)
							continue
						}
						services = append(services, nsServices...)
					}
				}

				// 更新缓存的函数列表
				e.services = services

			case <-quit:
				return
			}
		}
	}()
}

// getHTTPClient 创建带超时配置的 HTTP 客户端
func (e *Exporter) getHTTPClient(timeout time.Duration) http.Client {
	return http.Client{
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
}

// getFunctions 查询指定命名空间下的所有函数信息
func (e *Exporter) getFunctions(endpointURL url.URL, namespace string) ([]types.FunctionStatus, error) {
	timeout := 5 * time.Second
	proxyClient := e.getHTTPClient(timeout)

	// 构造请求地址
	endpointURL.Path = path.Join(endpointURL.Path, "/system/functions")
	if len(namespace) > 0 {
		q := endpointURL.Query()
		q.Set("namespace", namespace)
		endpointURL.RawQuery = q.Encode()
	}

	// 创建请求并添加认证
	get, _ := http.NewRequest(http.MethodGet, endpointURL.String(), nil)
	if e.credentials != nil {
		get.SetBasicAuth(e.credentials.User, e.credentials.Password)
	}

	// 执行请求
	services := []types.FunctionStatus{}
	res, err := proxyClient.Do(get)
	if err != nil {
		return services, err
	}

	// 读取响应体
	var body []byte
	if res.Body != nil {
		defer res.Body.Close()

		if b, err := io.ReadAll(res.Body); err != nil {
			return services, err
		} else {
			body = b
		}
	}

	if len(body) == 0 {
		return services, fmt.Errorf("no response body from /system/functions")
	}

	// 解析函数列表
	if err := json.Unmarshal(body, &services); err != nil {
		return services, fmt.Errorf("error unmarshalling response: %s, error: %s",
			string(body), err)
	}

	return services, nil
}

// getNamespaces 获取 OpenFaaS 所有命名空间
func (e *Exporter) getNamespaces(endpointURL url.URL) ([]string, error) {
	namespaces := []string{}
	endpointURL.Path = path.Join(endpointURL.Path, "system/namespaces")

	// 创建请求
	get, _ := http.NewRequest(http.MethodGet, endpointURL.String(), nil)
	if e.credentials != nil {
		get.SetBasicAuth(e.credentials.User, e.credentials.Password)
	}

	timeout := 5 * time.Second
	proxyClient := e.getHTTPClient(timeout)

	// 执行请求
	res, err := proxyClient.Do(get)
	if err != nil {
		return namespaces, err
	}

	// 404 表示不支持命名空间（如旧版/单体版）
	if res.StatusCode == http.StatusNotFound {
		return namespaces, nil
	}

	// 读取响应
	var body []byte
	if res.Body != nil {
		defer res.Body.Close()

		if b, err := io.ReadAll(res.Body); err != nil {
			return namespaces, err
		} else {
			body = b
		}
	}

	if len(body) == 0 {
		return namespaces, fmt.Errorf("no response body from /system/namespaces")
	}

	// 解析命名空间列表
	if err := json.Unmarshal(body, &namespaces); err != nil {
		return namespaces, fmt.Errorf("error unmarshalling response: %s, error: %s", string(body), err)
	}

	return namespaces, nil
}
