// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

// Package scaling 提供函数缓存元数据的定义与过期判断逻辑，用于管理函数状态缓存的生命周期
package scaling

import (
	"time"
)

// FunctionMeta 保存缓存所需的最后刷新时间及其他元数据
type FunctionMeta struct {
	LastRefresh          time.Time            // 缓存项的最后刷新时间
	ServiceQueryResponse ServiceQueryResponse // 缓存的函数状态查询响应
}

// Expired 判断缓存项是否已过期，基于其存储时间和给定的过期时长
// expiry: 缓存的有效时长
// 返回值: 缓存已过期返回true，未过期返回false
func (fm *FunctionMeta) Expired(expiry time.Duration) bool {
	return time.Now().After(fm.LastRefresh.Add(expiry))
}
