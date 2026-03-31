// Package types 定义核心数据结构与类型
package types

import "net/http"

// HandlerSet HTTP处理函数集合，用于绑定到路由器
type HandlerSet struct {
	// Proxy 调用函数
	Proxy http.HandlerFunc

	// DeployFunction 部署新函数
	DeployFunction http.HandlerFunc

	// DeleteFunction 删除已部署函数
	DeleteFunction http.HandlerFunc

	// ListFunctions 列出命名空间内所有已部署函数
	ListFunctions http.HandlerFunc

	// Alert 处理AlertManager触发的告警
	Alert http.HandlerFunc

	// UpdateFunction 更新已有函数
	UpdateFunction http.HandlerFunc

	// FunctionStatus 获取已部署函数状态
	FunctionStatus http.HandlerFunc

	// QueuedProxy 队列化工作并返回同步响应
	QueuedProxy http.HandlerFunc

	// ScaleFunction 扩缩容函数
	ScaleFunction http.HandlerFunc

	// InfoHandler 提供版本和构建信息
	InfoHandler http.HandlerFunc

	// TelemetryHandler 遥测数据处理
	TelemetryHandler http.HandlerFunc

	// SecretHandler 管理密钥
	SecretHandler http.HandlerFunc

	// LogProxyHandler 流式传输函数日志
	LogProxyHandler http.HandlerFunc

	// NamespaceListerHandler 列出命名空间
	NamespaceListerHandler http.HandlerFunc

	// NamespaceMutatorHandler 修改命名空间
	NamespaceMutatorHandler http.HandlerFunc
}
