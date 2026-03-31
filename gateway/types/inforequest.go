// Package types 定义核心数据结构与类型
package types

import providerTypes "github.com/openfaas/faas-provider/types"

// Arch 网关运行的平台架构
var Arch string

// GatewayInfo 提供网关及其连接组件的信息
type GatewayInfo struct {
	// Provider 提供商信息
	Provider *providerTypes.ProviderInfo `json:"provider"`
	// Version 版本信息
	Version *providerTypes.VersionInfo `json:"version"`
	// Arch 平台架构
	Arch string `json:"arch"`
}
