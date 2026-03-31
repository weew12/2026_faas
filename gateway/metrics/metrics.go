// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)
// Copyright (c) Alex Ellis 2017. All rights reserved.

package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricOptions 定义网关所有监控指标，供 Web 处理器使用
type MetricOptions struct {
	GatewayFunctionInvocation        *prometheus.CounterVec   // 函数调用完成次数（带状态码）
	GatewayFunctionsHistogram        *prometheus.HistogramVec // 函数执行耗时分布
	GatewayFunctionInvocationStarted *prometheus.CounterVec   // 函数请求开始次数
	ServiceReplicasGauge             *prometheus.GaugeVec     // 函数当前副本数
}

// ServiceMetricOptions 用于服务级别的 RED 监控指标（速率、错误、耗时）
type ServiceMetricOptions struct {
	Histogram *prometheus.HistogramVec // 耗时直方图
	Counter   *prometheus.CounterVec   // 请求计数器
}

// once 确保 Prometheus 注册只执行一次，避免重复注册 panic
var once = sync.Once{}

// RegisterExporter 向 Prometheus 注册自定义指标收集器（只执行一次）
func RegisterExporter(exporter *Exporter) {
	once.Do(func() {
		prometheus.MustRegister(exporter)
	})
}

// PrometheusHandler 返回 Prometheus 内置 HTTP 指标处理器
// 用于暴露 /metrics 接口
func PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// BuildMetricsOptions 创建并初始化所有 OpenFaaS 网关监控指标
func BuildMetricsOptions() MetricOptions {
	// 函数执行耗时（秒），标签：function_name、code
	gatewayFunctionsHistogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "gateway_functions_seconds",
		Help: "Function time taken",
	}, []string{"function_name", "code"})

	// 函数调用完成总数，标签：function_name、code
	gatewayFunctionInvocation := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "function",
			Name:      "invocation_total",
			Help:      "Function metrics",
		},
		[]string{"function_name", "code"},
	)

	// 服务当前副本数，标签：function_name
	serviceReplicas := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gateway",
			Name:      "service_count",
			Help:      "Current count of replicas for function",
		},
		[]string{"function_name"},
	)

	// 函数请求开始总数，标签：function_name
	gatewayFunctionInvocationStarted := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "function",
			Name:      "invocation_started",
			Help:      "The total number of function HTTP requests started.",
		},
		[]string{"function_name"},
	)

	// 封装所有指标并返回
	metricsOptions := MetricOptions{
		GatewayFunctionsHistogram:        gatewayFunctionsHistogram,
		GatewayFunctionInvocation:        gatewayFunctionInvocation,
		ServiceReplicasGauge:             serviceReplicas,
		GatewayFunctionInvocationStarted: gatewayFunctionInvocationStarted,
	}

	return metricsOptions
}
