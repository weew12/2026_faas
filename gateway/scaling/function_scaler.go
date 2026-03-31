// Package scaling 提供函数从零开始扩容的核心逻辑，包含缓存检查、单飞请求合并、重试机制等功能
package scaling

import (
	"fmt"
	"log"
	"time"

	"github.com/openfaas/faas/gateway/types"
	"golang.org/x/sync/singleflight"
)

// NewFunctionScaler 创建一个新的函数扩容器，使用指定的扩容配置和函数缓存
// config: 扩容行为的配置参数
// functionCacher: 用于缓存函数副本状态的缓存接口
// 返回值: 初始化后的FunctionScaler实例
func NewFunctionScaler(config ScalingConfig, functionCacher FunctionCacher) FunctionScaler {
	return FunctionScaler{
		Cache:        functionCacher,
		Config:       config,
		SingleFlight: &singleflight.Group{},
	}
}

// FunctionScaler 实现函数从零副本扩容的核心组件
type FunctionScaler struct {
	Cache        FunctionCacher      // 函数副本状态缓存
	Config       ScalingConfig       // 扩容配置
	SingleFlight *singleflight.Group // 单飞组件，用于合并相同的请求，避免重复查询或扩容
}

// FunctionScaleResult 保存函数从零扩容的执行结果
type FunctionScaleResult struct {
	Available bool          // 函数是否有可用副本
	Error     error         // 扩容过程中发生的错误
	Found     bool          // 函数是否存在
	Duration  time.Duration // 扩容操作的总耗时
}

// Scale 将函数从零副本扩容到1或其最小副本数元数据中设置的值
// functionName: 目标函数名称
// namespace: 函数所在的命名空间
// 返回值: 包含扩容结果的FunctionScaleResult
func (f *FunctionScaler) Scale(functionName, namespace string) FunctionScaleResult {
	start := time.Now()

	// 首先检查缓存，如果有可用副本，则可以直接处理请求
	if cachedResponse, hit := f.Cache.Get(functionName, namespace); hit &&
		cachedResponse.AvailableReplicas > 0 {
		return FunctionScaleResult{
			Error:     nil,
			Available: true,
			Found:     true,
			Duration:  time.Since(start),
		}
	}

	// 缓存未命中或没有可用副本，查询实时接口
	getKey := fmt.Sprintf("GetReplicas-%s.%s", functionName, namespace)
	res, err, _ := f.SingleFlight.Do(getKey, func() (interface{}, error) {
		return f.Config.ServiceQuery.GetReplicas(functionName, namespace)
	})

	if err != nil {
		return FunctionScaleResult{
			Error:     err,
			Available: false,
			Found:     false,
			Duration:  time.Since(start),
		}
	}
	if res == nil {
		return FunctionScaleResult{
			Error:     fmt.Errorf("empty response from server"),
			Available: false,
			Found:     false,
			Duration:  time.Since(start),
		}
	}

	// 检查实时数据中是否有可用副本
	if res.(ServiceQueryResponse).AvailableReplicas > 0 {
		return FunctionScaleResult{
			Error:     nil,
			Available: true,
			Found:     true,
			Duration:  time.Since(start),
		}
	}

	// 将GetReplicas的结果存入缓存
	queryResponse := res.(ServiceQueryResponse)
	f.Cache.Set(functionName, namespace, queryResponse)

	// 如果当前副本数为0，则需要触发扩容事件
	if queryResponse.Replicas == 0 {
		minReplicas := uint64(1)
		if queryResponse.MinReplicas > 0 {
			minReplicas = queryResponse.MinReplicas
		}

		// 在重试循环中，先查询当前副本数，若仍为0则设置副本数
		scaleResult := types.Retry(func(attempt int) error {

			res, err, _ := f.SingleFlight.Do(getKey, func() (interface{}, error) {
				return f.Config.ServiceQuery.GetReplicas(functionName, namespace)
			})

			if err != nil {
				return err
			}

			// 缓存响应结果
			queryResponse = res.(ServiceQueryResponse)
			f.Cache.Set(functionName, namespace, queryResponse)

			// 扩容已完成，因为当前副本数已设置为1或更多
			if queryResponse.Replicas > 0 {
				return nil
			}

			// 请求扩容到最小副本数
			setKey := fmt.Sprintf("SetReplicas-%s.%s", functionName, namespace)

			if _, err, _ := f.SingleFlight.Do(setKey, func() (interface{}, error) {

				log.Printf("[Scale %d/%d] function=%s 0 => %d requested",
					attempt, int(f.Config.SetScaleRetries), functionName, minReplicas)

				if err := f.Config.ServiceQuery.SetReplicas(functionName, namespace, minReplicas); err != nil {
					return nil, fmt.Errorf("unable to scale function [%s], err: %s", functionName, err)
				}
				return nil, nil
			}); err != nil {
				return err
			}

			return nil

		}, "Scale", int(f.Config.SetScaleRetries), f.Config.FunctionPollInterval)

		if scaleResult != nil {
			return FunctionScaleResult{
				Error:     scaleResult,
				Available: false,
				Found:     true,
				Duration:  time.Since(start),
			}
		}

	}

	// 等待至少一个函数副本变为可用
	for i := 0; i < int(f.Config.MaxPollCount); i++ {

		res, err, _ := f.SingleFlight.Do(getKey, func() (interface{}, error) {
			return f.Config.ServiceQuery.GetReplicas(functionName, namespace)
		})
		queryResponse := res.(ServiceQueryResponse)

		if err == nil {
			f.Cache.Set(functionName, namespace, queryResponse)
		}

		totalTime := time.Since(start)

		if err != nil {
			return FunctionScaleResult{
				Error:     err,
				Available: false,
				Found:     true,
				Duration:  totalTime,
			}
		}

		if queryResponse.AvailableReplicas > 0 {

			log.Printf("[Ready] function=%s waited for - %.4fs", functionName, totalTime.Seconds())

			return FunctionScaleResult{
				Error:     nil,
				Available: true,
				Found:     true,
				Duration:  totalTime,
			}
		}

		time.Sleep(f.Config.FunctionPollInterval)
	}

	return FunctionScaleResult{
		Error:     nil,
		Available: true,
		Found:     true,
		Duration:  time.Since(start),
	}
}
