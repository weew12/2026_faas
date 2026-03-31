// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) Alex Ellis 2017. All rights reserved.

// Package requests 提供Prometheus与AlertManager告警相关的数据结构定义，用于解析和处理告警请求
package requests

// PrometheusInnerAlertLabel Prometheus内部告警的标签信息
type PrometheusInnerAlertLabel struct {
	AlertName    string `json:"alertname"`     // 告警名称
	FunctionName string `json:"function_name"` // 关联的函数名称
}

// PrometheusInnerAlert Prometheus内部告警信息
type PrometheusInnerAlert struct {
	Status string                    `json:"status"` // 告警状态
	Labels PrometheusInnerAlertLabel `json:"labels"` // 告警标签
}

// PrometheusAlert AlertManager生成的Prometheus告警结构
type PrometheusAlert struct {
	Status   string                 `json:"status"`   // 告警整体状态
	Receiver string                 `json:"receiver"` // 告警接收者
	Alerts   []PrometheusInnerAlert `json:"alerts"`   // 内部告警列表
}
