// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

// Package scaling 提供函数扩缩容相关的核心接口与数据结构，定义了服务副本查询、设置的抽象契约，以及函数状态查询的响应格式
package scaling

// ServiceQuery 定义了服务副本查询与设置的抽象接口，用于解耦扩缩容逻辑与具体的基础设施实现（如Kubernetes、Docker Swarm等）
type ServiceQuery interface {
	// GetReplicas 查询指定服务的当前副本状态
	// service: 目标服务名称
	// namespace: 服务所在的命名空间
	// 返回值: 包含副本信息的ServiceQueryResponse，以及查询过程中可能出现的错误
	GetReplicas(service, namespace string) (response ServiceQueryResponse, err error)
	// SetReplicas 设置指定服务的目标副本数
	// service: 目标服务名称
	// namespace: 服务所在的命名空间
	// count: 要设置的目标副本数
	// 返回值: 设置过程中可能出现的错误
	SetReplicas(service, namespace string, count uint64) error
}

// ServiceQueryResponse 定义了函数状态查询的响应数据结构，包含副本相关的核心指标与元数据
type ServiceQueryResponse struct {
	Replicas          uint64             // 当前配置的副本数
	MaxReplicas       uint64             // 最大允许副本数
	MinReplicas       uint64             // 最小允许副本数
	ScalingFactor     uint64             // 扩缩容因子（用于控制每次扩缩容的步长比例）
	AvailableReplicas uint64             // 可用副本数（已就绪并可处理请求的副本数）
	Annotations       *map[string]string // 服务的注解指针，存储额外的元数据信息
}
