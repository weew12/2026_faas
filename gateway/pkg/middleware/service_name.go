// Package middleware 提供HTTP中间件组件，包含命名空间解析等辅助功能
package middleware

import "strings"

// GetNamespace 从完整名称中解析出名称和命名空间，使用最后一个点作为分隔符
// defaultNamespace: 默认命名空间，当完整名称中无点时使用
// fullName: 完整名称，格式为"名称.命名空间"
// 返回值: 解析出的名称和命名空间
func GetNamespace(defaultNamespace, fullName string) (string, string) {
	if index := strings.LastIndex(fullName, "."); index > -1 {
		return fullName[:index], fullName[index+1:]
	}
	return fullName, defaultNamespace
}
