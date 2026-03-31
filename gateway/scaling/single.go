// Package scaling 提供函数扩缩容相关的工具组件，包含SingleFlight（单飞）模式实现，用于防止相同操作的重复执行
package scaling

import (
	"log"
	"sync"
)

// Call 表示单个正在进行的调用上下文，用于协调多个等待该调用结果的请求
type Call struct {
	wg  *sync.WaitGroup     // 等待组，用于阻塞等待该调用完成的所有请求
	res *SingleFlightResult // 调用执行的结果指针，执行完成后所有等待者可获取
}

// SingleFlight 单飞管理器，用于防止相同key的操作重复执行，确保同一时间只有一个相同key的函数在运行
type SingleFlight struct {
	lock  *sync.RWMutex    // 读写锁，保护calls map的并发安全
	calls map[string]*Call // 存储正在进行的调用，key为操作唯一标识，value为调用上下文
}

// SingleFlightResult 存储SingleFlight操作的执行结果
type SingleFlightResult struct {
	Result interface{} // 函数执行的成功结果
	Error  error       // 函数执行的错误信息
}

// NewSingleFlight 创建并初始化一个新的SingleFlight实例
func NewSingleFlight() *SingleFlight {
	return &SingleFlight{
		lock:  &sync.RWMutex{},
		calls: map[string]*Call{},
	}
}

// Do 执行SingleFlight操作：若已有相同key的调用在进行，则等待其结果；否则执行传入的函数并返回结果
// key: 操作的唯一标识，相同key的请求会被合并
// f: 实际要执行的函数，仅当无相同key的调用时执行一次
// 返回值: 函数执行的结果和错误
func (s *SingleFlight) Do(key string, f func() (interface{}, error)) (interface{}, error) {

	// 加写锁，检查是否已有相同key的调用在进行
	s.lock.Lock()

	// 若存在正在进行的同key调用
	if call, ok := s.calls[key]; ok {
		s.lock.Unlock() // 解锁，避免阻塞其他操作
		call.wg.Wait()  // 等待该调用完成

		// 直接返回已完成的调用结果
		return call.res.Result, call.res.Error
	}

	// 若不存在同key调用，创建新的调用上下文
	var call *Call
	if s.calls[key] == nil {
		call = &Call{
			wg: &sync.WaitGroup{},
		}
		s.calls[key] = call // 将新调用存入map
	}

	call.wg.Add(1) // 等待组计数+1，后续所有等待者都会阻塞在Wait()上

	s.lock.Unlock() // 解锁，允许其他请求进来

	// 启动goroutine异步执行实际函数
	go func() {
		log.Printf("Miss, so running: %s", key)
		res, err := f() // 执行传入的实际函数

		// 函数执行完成后，加写锁更新结果并清理
		s.lock.Lock()
		// 保存执行结果
		call.res = &SingleFlightResult{
			Result: res,
			Error:  err,
		}

		call.wg.Done() // 等待组计数-1，唤醒所有等待的请求

		delete(s.calls, key) // 从map中删除已完成的调用，避免内存泄漏

		s.lock.Unlock()
	}()

	// 当前请求也等待调用完成
	call.wg.Wait()

	// 返回执行结果
	return call.res.Result, call.res.Error
}
