// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

// Package scaling 提供带缓存的函数查询功能，通过缓存和单飞模式减少对底层服务的重复查询
package scaling

import (
	"fmt"
	"log"

	"golang.org/x/sync/singleflight"
)

// CachedFunctionQuery 是FunctionQuery的带缓存实现，结合缓存和单飞模式优化函数查询性能
type CachedFunctionQuery struct {
	cache            FunctionCacher      // 函数状态缓存
	serviceQuery     ServiceQuery        // 底层服务查询接口
	emptyAnnotations map[string]string   // 空注解映射，用于无注解时的默认返回
	singleFlight     *singleflight.Group // 单飞组件，合并相同的查询请求
}

// NewCachedFunctionQuery 创建一个带缓存的FunctionQuery实例
// cache: 函数状态缓存实现
// serviceQuery: 底层服务查询接口
// 返回值: 实现了FunctionQuery接口的CachedFunctionQuery实例
func NewCachedFunctionQuery(cache FunctionCacher, serviceQuery ServiceQuery) FunctionQuery {
	return &CachedFunctionQuery{
		cache:            cache,
		serviceQuery:     serviceQuery,
		emptyAnnotations: map[string]string{},
		singleFlight:     &singleflight.Group{},
	}
}

// GetAnnotations 获取指定函数的注解
// name: 函数名称
// namespace: 函数所在的命名空间
// 返回值: 函数的注解映射，无注解时返回空映射；查询失败时返回错误
func (c *CachedFunctionQuery) GetAnnotations(name string, namespace string) (annotations map[string]string, err error) {
	res, err := c.Get(name, namespace)
	if err != nil {
		return c.emptyAnnotations, err
	}

	if res.Annotations == nil {
		return c.emptyAnnotations, nil
	}
	return *res.Annotations, nil
}

// Get 获取指定函数的状态信息，优先从缓存读取，缓存未命中时从底层服务查询
// fn: 函数名称
// ns: 函数所在的命名空间
// 返回值: 函数状态查询响应；查询失败时返回错误
func (c *CachedFunctionQuery) Get(fn string, ns string) (ServiceQueryResponse, error) {

	// 首先检查缓存
	query, hit := c.cache.Get(fn, ns)
	if !hit {
		// 缓存未命中，使用单飞模式合并相同请求，从底层服务查询
		key := fmt.Sprintf("GetReplicas-%s.%s", fn, ns)
		queryResponse, err, _ := c.singleFlight.Do(key, func() (interface{}, error) {
			log.Printf("Cache miss - run GetReplicas")
			// 缓存未命中时，从提供商API获取数据
			return c.serviceQuery.GetReplicas(fn, ns)
		})

		if err != nil {
			return ServiceQueryResponse{}, err
		}

		// 将查询结果存入缓存
		if queryResponse != nil {
			c.cache.Set(fn, ns, queryResponse.(ServiceQueryResponse))
		}

	} else {
		// 缓存命中，直接返回缓存数据
		return query, nil
	}

	// 此时数据几乎肯定已存在，若仍未命中则返回错误
	query, hit = c.cache.Get(fn, ns)
	if !hit {
		return ServiceQueryResponse{}, fmt.Errorf("error with cache key: %s", fn+"."+ns)
	}

	return query, nil
}

// FunctionQuery 定义函数查询的抽象接口，包含获取函数状态和注解的方法
type FunctionQuery interface {
	// Get 获取指定函数的状态信息
	Get(name string, namespace string) (ServiceQueryResponse, error)
	// GetAnnotations 获取指定函数的注解
	GetAnnotations(name string, namespace string) (annotations map[string]string, err error)
}
