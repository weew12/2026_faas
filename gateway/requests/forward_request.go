// Package requests 提供请求转发相关的工具类型与函数，用于构建代理请求的URL
package requests

import (
	"fmt"
	"net/url"
)

// ForwardRequest 用于代理传入请求的结构，保存请求的关键信息
type ForwardRequest struct {
	RawPath  string // 原始URL路径
	RawQuery string // 原始URL查询字符串
	Method   string // HTTP请求方法
}

// NewForwardRequest 创建一个ForwardRequest实例
// method: HTTP请求方法
// url: 原始请求的URL对象
// 返回值: 初始化后的ForwardRequest
func NewForwardRequest(method string, url url.URL) ForwardRequest {
	return ForwardRequest{
		Method:   method,
		RawQuery: url.RawQuery,
		RawPath:  url.Path,
	}
}

// ToURL 根据目标地址和watchdog端口构建格式化的完整URL
// addr: 目标服务地址
// watchdogPort: watchdog服务的端口
// 返回值: 拼接后的完整HTTP URL字符串
func (f *ForwardRequest) ToURL(addr string, watchdogPort int) string {
	if len(f.RawQuery) > 0 {
		return fmt.Sprintf("http://%s:%d%s?%s", addr, watchdogPort, f.RawPath, f.RawQuery)
	}
	return fmt.Sprintf("http://%s:%d%s", addr, watchdogPort, f.RawPath)

}
