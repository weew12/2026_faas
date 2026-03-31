// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) OpenFaaS Author(s). All rights reserved.

// Package middleware 提供HTTP中间件组件，包含URL解析、路径转换等功能，用于处理上游请求的路由
package middleware

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// functionMatcher 用于解析URL路径，提取服务名（分组1）和剩余路径（分组2）
var functionMatcher = regexp.MustCompile("^/?(?:async-)?function/([^/?]+)([^?]*)")

// functionMatcher 正则分组的索引和元数据
const (
	hasPathCount = 3 // 正则匹配成功时的分组数量
	routeIndex   = 0 // routeIndex 对应 /function/ 或 /async-function/ 路径
	nameIndex    = 1 // nameIndex 是函数名称
	pathIndex    = 2 // pathIndex 是剩余路径，如 /employee/:id/
)

// BaseURLResolver 上游请求的URL解析器接口
type BaseURLResolver interface {
	// Resolve 解析请求的基础URL
	Resolve(r *http.Request) string
	// BuildURL 构建函数的完整URL
	BuildURL(function, namespace, healthPath string, directFunctions bool) string
}

// URLPathTransformer 上游请求的URL路径转换器接口
type URLPathTransformer interface {
	// Transform 转换传入的URL路径
	Transform(r *http.Request) string
}

// SingleHostBaseURLResolver 针对单个BaseURL的URL解析器
type SingleHostBaseURLResolver struct {
	BaseURL string // 基础URL
}

// BuildURL 构建函数的完整URL
// function: 函数名称
// namespace: 命名空间
// healthPath: 健康检查路径
// directFunctions: 是否直接访问函数
// 返回值: 构建后的完整URL字符串
func (s SingleHostBaseURLResolver) BuildURL(function, namespace, healthPath string, directFunctions bool) string {
	u, _ := url.Parse(s.BaseURL)

	base := fmt.Sprintf("/function/%s.%s/", function, namespace)

	if len(healthPath) > 0 {
		u.Path = path.Join(base, healthPath)
	} else {
		u.Path = base
	}

	return u.String()
}

// Resolve 解析请求的基础URL，去除末尾的斜杠
// r: HTTP请求
// 返回值: 处理后的基础URL
func (s SingleHostBaseURLResolver) Resolve(r *http.Request) string {

	baseURL := s.BaseURL

	if strings.HasSuffix(baseURL, "/") {
		baseURL = baseURL[0 : len(baseURL)-1]
	}
	return baseURL
}

// FunctionAsHostBaseURLResolver 使用URL中的函数名作为主机名的URL解析器
type FunctionAsHostBaseURLResolver struct {
	FunctionSuffix    string // 函数名后缀
	FunctionNamespace string // 函数命名空间
}

// Resolve 解析请求的基础URL，使用函数名作为主机名
// r: HTTP请求
// 返回值: 构建后的基础URL（http://函数名.后缀:8080）
func (f FunctionAsHostBaseURLResolver) Resolve(r *http.Request) string {
	svcName := GetServiceName(r.URL.Path)

	const watchdogPort = 8080
	var suffix string

	if len(f.FunctionSuffix) > 0 {
		if index := strings.LastIndex(svcName, "."); index > -1 && len(svcName) > index+1 {
			suffix = strings.Replace(f.FunctionSuffix, f.FunctionNamespace, "", -1)
		} else {
			suffix = "." + f.FunctionSuffix
		}
	}

	return fmt.Sprintf("http://%s%s:%d", svcName, suffix, watchdogPort)
}

// BuildURL 构建函数的完整URL，使用函数名作为主机名
// function: 函数名称
// namespace: 命名空间
// healthPath: 健康检查路径
// directFunctions: 是否直接访问函数
// 返回值: 构建后的完整URL字符串
func (f FunctionAsHostBaseURLResolver) BuildURL(function, namespace, healthPath string, directFunctions bool) string {
	svcName := function

	const watchdogPort = 8080
	var suffix string

	if len(f.FunctionSuffix) > 0 {
		suffix = strings.Replace(f.FunctionSuffix, f.FunctionNamespace, namespace, 1)
	}

	u, _ := url.Parse(fmt.Sprintf("http://%s.%s:%d", svcName, suffix, watchdogPort))
	if len(healthPath) > 0 {
		u.Path = healthPath
	}

	return u.String()
}

// TransparentURLPathTransformer 透明URL路径转换器，直接传递请求的URL路径
type TransparentURLPathTransformer struct {
}

// Transform 返回未修改的URL路径
// r: HTTP请求
// 返回值: 原始URL路径
func (f TransparentURLPathTransformer) Transform(r *http.Request) string {
	return r.URL.Path
}

// FunctionPrefixTrimmingURLPathTransformer URL路径转换器，去除URL路径中的"/function/servicename/"前缀
type FunctionPrefixTrimmingURLPathTransformer struct {
}

// Transform 去除URL路径中的"/function/servicename/"前缀
// r: HTTP请求
// 返回值: 去除前缀后的URL路径
func (f FunctionPrefixTrimmingURLPathTransformer) Transform(r *http.Request) string {
	ret := r.URL.Path

	if ret != "" {
		// 当转发到函数时，/function/xyz 部分仅网关使用，需去除，保留/rest/of/path
		// 正则匹配成功时，r.URL.Path在索引0，函数名在1，剩余路径在2
		matcher := functionMatcher.Copy()
		parts := matcher.FindStringSubmatch(ret)
		if len(parts) == hasPathCount {
			ret = parts[pathIndex]
		}
	}

	return ret
}

// GetServiceName 从URL路径中提取服务名
// urlValue: URL路径
// 返回值: 提取的服务名，无匹配时返回空字符串
func GetServiceName(urlValue string) string {
	var serviceName string
	forward := "/function/"
	if strings.HasPrefix(urlValue, forward) {
		// 对于路径 /function/xyz/rest/of/path?q=a，服务名是xyz
		// 正则匹配成功时返回三个元素的切片：索引0是原路径，1是服务名，2是剩余路径
		matcher := functionMatcher.Copy()
		matches := matcher.FindStringSubmatch(urlValue)
		if len(matches) == hasPathCount {
			serviceName = matches[nameIndex]
		}
	}
	return strings.Trim(serviceName, "/")
}
