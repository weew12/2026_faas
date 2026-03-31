// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) Alex Ellis 2017. All rights reserved.

package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"

	"github.com/openfaas/faas/gateway/pkg/middleware"
	"github.com/openfaas/faas/gateway/requests"
	"github.com/openfaas/faas/gateway/scaling"
)

// MakeAlertHandler 创建处理 Prometheus Alertmanager 告警的 HTTP 处理器
// 接收告警请求，解析后自动对函数进行扩缩容操作
func MakeAlertHandler(service scaling.ServiceQuery, defaultNamespace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		// 校验请求体是否存在
		if r.Body == nil {
			http.Error(w, "A body is required for this endpoint", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()

		// 读取请求体数据
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Unable to read alert."))

			log.Println(err)
			return
		}

		// 反序列化告警数据
		var req requests.PrometheusAlert
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Unable to parse alert, bad format."))
			log.Println(err)
			return
		}

		// 处理所有告警
		errors := handleAlerts(req, service, defaultNamespace)
		if len(errors) > 0 {
			log.Println(errors)
			var errorOutput string
			for d, err := range errors {
				errorOutput += fmt.Sprintf("[%d] %s\n", d, err)
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(errorOutput))
			return
		}

		// 处理成功
		w.WriteHeader(http.StatusOK)
	}
}

// handleAlerts 批量处理 Prometheus 告警列表
// 遍历所有告警，逐个执行函数扩容操作，收集处理过程中的错误
func handleAlerts(req requests.PrometheusAlert, service scaling.ServiceQuery, defaultNamespace string) []error {
	var errors []error
	for _, alert := range req.Alerts {
		if err := scaleService(alert, service, defaultNamespace); err != nil {
			log.Println(err)
			errors = append(errors, err)
		}
	}

	return errors
}

// scaleService 根据告警状态执行单个函数的扩缩容逻辑
// 解析函数名与命名空间 → 查询当前副本数 → 计算目标副本数 → 执行扩容/缩容
func scaleService(alert requests.PrometheusInnerAlert, service scaling.ServiceQuery, defaultNamespace string) error {
	var err error

	// 解析函数名称与命名空间
	serviceName, namespace := middleware.GetNamespace(defaultNamespace, alert.Labels.FunctionName)

	// 函数名称有效时执行扩容
	if len(serviceName) > 0 {
		queryResponse, getErr := service.GetReplicas(serviceName, namespace)
		if getErr == nil {
			status := alert.Status

			// 计算目标副本数
			newReplicas := CalculateReplicas(status, queryResponse.Replicas, uint64(queryResponse.MaxReplicas), queryResponse.MinReplicas, queryResponse.ScalingFactor)

			log.Printf("\033[33mScaling: \033[0m")
			log.Printf("[Scale] function=%s %d => %d.\n", serviceName, queryResponse.Replicas, newReplicas)

			// 副本数无变化则直接返回
			if newReplicas == queryResponse.Replicas {
				return nil
			}

			// 更新副本数
			updateErr := service.SetReplicas(serviceName, namespace, newReplicas)
			if updateErr != nil {
				err = updateErr
			}
		}
	}
	return err
}

// CalculateReplicas 根据告警状态计算目标副本数
// firing：扩容（按比例步长增加）
// resolved：缩容至最小副本数
func CalculateReplicas(status string, currentReplicas uint64, maxReplicas uint64, minReplicas uint64, scalingFactor uint64) uint64 {
	var newReplicas uint64

	// 最大副本数限制为默认最大值
	maxReplicas = uint64(math.Min(float64(maxReplicas), float64(scaling.DefaultMaxReplicas)))
	// 计算扩容步长：最大副本数 * 扩容因子百分比
	step := uint64(math.Ceil(float64(maxReplicas) / 100 * float64(scalingFactor)))

	// 告警触发中：扩容
	if status == "firing" && step > 0 {
		if currentReplicas+step > maxReplicas {
			newReplicas = maxReplicas
		} else {
			newReplicas = currentReplicas + step
		}
	} else { // 告警已恢复：缩容至最小值
		newReplicas = minReplicas
	}

	return newReplicas
}
