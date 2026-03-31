// Package scaling 提供函数扩缩容相关的配置定义，包含控制扩缩容行为的核心参数与依赖接口
package scaling

import (
	"time"
)

// ScalingConfig 定义函数扩缩容行为的全量配置，控制轮询策略、缓存生命周期、重试机制等核心扩缩容逻辑
type ScalingConfig struct {
	// MaxPollCount 查询函数状态的最大尝试次数，超过此次数后将放弃查询
	MaxPollCount uint

	// FunctionPollInterval 轮询函数就绪状态的延迟或间隔时间
	FunctionPollInterval time.Duration

	// CacheExpiry 缓存条目的有效期，超过此时间后缓存将被视为无效
	CacheExpiry time.Duration

	// ServiceQuery 用于查询函数可用/就绪副本数的接口，同时也支持设置副本数
	ServiceQuery ServiceQuery

	// SetScaleRetries 设置函数副本数时的重试次数，因错误失败时会重试至此次数后放弃
	SetScaleRetries uint
}
