// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

// Package scaling 提供函数副本数缓存的实现，包含缓存接口定义、内存缓存结构及过期管理逻辑
package scaling

import (
	"sync"
	"time"
)

// FunctionCacher 定义函数查询与结果缓存的抽象接口
type FunctionCacher interface {
	// Set 缓存指定函数的查询响应
	// functionName: 函数名称
	// namespace: 函数所在的命名空间
	// serviceQueryResponse: 要缓存的函数状态查询响应
	Set(functionName, namespace string, serviceQueryResponse ServiceQueryResponse)
	// Get 从缓存获取指定函数的查询响应
	// functionName: 函数名称
	// namespace: 函数所在的命名空间
	// 返回值: 缓存的函数状态查询响应，以及缓存是否命中且未过期
	Get(functionName, namespace string) (ServiceQueryResponse, bool)
}

// FunctionCache 提供函数副本数的内存缓存实现
type FunctionCache struct {
	Cache  map[string]*FunctionMeta // 缓存数据，key为"函数名.命名空间"，value为函数元数据
	Expiry time.Duration            // 缓存项的过期时长
	Sync   sync.RWMutex             // 读写锁，保护缓存的并发安全
}

// NewFunctionCache 创建一个用于查询函数元数据的函数缓存
// cacheExpiry: 缓存项的过期时长
// 返回值: 实现了FunctionCacher接口的FunctionCache实例
func NewFunctionCache(cacheExpiry time.Duration) FunctionCacher {
	return &FunctionCache{
		Cache:  make(map[string]*FunctionMeta),
		Expiry: cacheExpiry,
	}
}

// Set 缓存指定函数的副本数及状态信息
// functionName: 函数名称
// namespace: 函数所在的命名空间
// queryRes: 要缓存的函数状态查询响应
func (fc *FunctionCache) Set(functionName, namespace string, queryRes ServiceQueryResponse) {
	fc.Sync.Lock()
	defer fc.Sync.Unlock()

	// 若缓存中不存在该函数项，则初始化
	if _, exists := fc.Cache[functionName+"."+namespace]; !exists {
		fc.Cache[functionName+"."+namespace] = &FunctionMeta{}
	}

	// 更新缓存的最后刷新时间和查询响应
	fc.Cache[functionName+"."+namespace].LastRefresh = time.Now()
	fc.Cache[functionName+"."+namespace].ServiceQueryResponse = queryRes
}

// Get 从缓存获取指定函数的副本数及状态信息，同时检查缓存是否过期
// functionName: 函数名称
// namespace: 函数所在的命名空间
// 返回值: 缓存的函数状态查询响应（未命中时返回默认值），以及缓存是否命中且未过期
func (fc *FunctionCache) Get(functionName, namespace string) (ServiceQueryResponse, bool) {
	// 初始化默认查询响应，可用副本数为0
	queryRes := ServiceQueryResponse{
		AvailableReplicas: 0,
	}

	hit := false
	fc.Sync.RLock()
	defer fc.Sync.RUnlock()

	// 检查缓存中是否存在该函数项
	if val, exists := fc.Cache[functionName+"."+namespace]; exists {
		queryRes = val.ServiceQueryResponse
		// 判断缓存是否未过期
		hit = !val.Expired(fc.Expiry)
	}

	return queryRes, hit
}
