package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openfaas/faas/gateway/metrics"
	"github.com/openfaas/faas/gateway/pkg/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTPNotifier 通知接口
// 用于监听 HTTP 请求的生命周期事件（开始/完成），并执行回调
type HTTPNotifier interface {
	Notify(method string, URL string, originalURL string, statusCode int, event string, duration time.Duration)
}

// urlToLabel 将 URL 路径标准化为可用于指标标签的格式
// 去除末尾斜杠，空路径转为 /
func urlToLabel(path string) string {
	if len(path) > 0 {
		path = strings.TrimRight(path, "/")
	}
	if path == "" {
		path = "/"
	}
	return path
}

// PrometheusFunctionNotifier Prometheus 指标记录器
// 实现 HTTPNotifier，将函数调用指标写入 Prometheus
type PrometheusFunctionNotifier struct {
	Metrics           *metrics.MetricOptions // 指标配置
	FunctionNamespace string                 // 函数默认命名空间
}

// Notify 实现 HTTPNotifier 接口，记录函数调用指标
// 根据事件类型（started/completed）记录启动数、调用数、耗时直方图
func (p PrometheusFunctionNotifier) Notify(method string, URL string, originalURL string, statusCode int, event string, duration time.Duration) {
	// 从原始 URL 中提取函数名
	serviceName := middleware.GetServiceName(originalURL)

	// 如果配置了默认命名空间，且函数名不含 .，则拼接命名空间
	if len(p.FunctionNamespace) > 0 {
		if !strings.Contains(serviceName, ".") {
			serviceName = fmt.Sprintf("%s.%s", serviceName, p.FunctionNamespace)
		}
	}

	// 状态码转为字符串标签
	code := strconv.Itoa(statusCode)
	labels := prometheus.Labels{"function_name": serviceName, "code": code}

	// 请求完成：记录耗时 & 调用计数
	if event == "completed" {
		seconds := duration.Seconds()
		p.Metrics.GatewayFunctionsHistogram.
			With(labels).
			Observe(seconds)

		p.Metrics.GatewayFunctionInvocation.
			With(labels).
			Inc()
	} else if event == "started" {
		// 请求开始：记录启动计数
		p.Metrics.GatewayFunctionInvocationStarted.WithLabelValues(serviceName).Inc()
	}
}

// LoggingNotifier 日志通知器
// 实现 HTTPNotifier，用于打印请求日志（当前注释未启用）
type LoggingNotifier struct {
}

// Notify 实现 HTTPNotifier 接口，打印请求完成日志
func (LoggingNotifier) Notify(method string, URL string, originalURL string, statusCode int, event string, duration time.Duration) {
	if event == "completed" {
		// log.Printf("Forwarded [%s] to %s - [%d] - %.4fs", method, originalURL, statusCode, duration.Seconds())
	}
}
