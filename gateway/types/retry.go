// Package types 定义OpenFaaS网关通用工具类型与函数，包含重试机制等辅助能力
package types

import (
	"log"
	"time"
)

// routine 定义可重试的函数类型
// attempt: 当前尝试次数（从0开始计数）
// 返回值: 执行成功返回nil，失败返回对应的error
type routine func(attempt int) error

// Retry 实现通用的重试逻辑，用于执行可能失败的操作并在失败时自动重试
// r: 需要执行的可重试函数
// label: 操作标签，用于日志标识
// attempts: 最大尝试次数
// interval: 两次尝试之间的等待间隔
// 返回值: 所有尝试均失败时返回最后一次错误，成功则返回nil
func Retry(r routine, label string, attempts int, interval time.Duration) error {
	var err error

	// 循环执行尝试，最多attempts次
	for i := 0; i < attempts; i++ {
		res := r(i)
		if res != nil {
			// 尝试失败，记录错误日志并继续
			err = res
			log.Printf("[%s]: %d/%d, error: %s\n", label, i, attempts, res)
		} else {
			// 尝试成功，清除错误并退出重试
			err = nil
			break
		}
		// 等待指定间隔后进行下一次尝试
		time.Sleep(interval)
	}
	return err
}
