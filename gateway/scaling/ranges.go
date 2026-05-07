// Package scaling 提供函数扩缩容相关的常量定义与水平缩放中间件，用于处理函数副本数的调整请求
package scaling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/openfaas/faas-provider/types"
)

const (
	// DefaultMinReplicas 服务的最小副本数默认值
	// DefaultMinReplicas = 1
	DefaultMinReplicas = 0

	// DefaultMaxReplicas 服务自动扩容的最大副本数默认值
	DefaultMaxReplicas = 50 // 函数最大副本数

	// DefaultScalingFactor 扩容增量的定义比例默认值
	DefaultScalingFactor = 10

	// DefaultTypeScale 默认的缩放类型
	DefaultTypeScale = "rps"

	// MinScaleLabel 标识函数最小副本数的标签
	MinScaleLabel = "com.openfaas.scale.min"

	// MaxScaleLabel 标识函数最大副本数的标签
	MaxScaleLabel = "com.openfaas.scale.max"

	// ScalingFactorLabel 标识函数扩容因子的标签
	ScalingFactorLabel = "com.openfaas.scale.factor"
)

// MakeHorizontalScalingHandler 创建一个HTTP中间件，用于拦截和处理水平缩放请求
// 对请求进行方法校验、参数解析与副本数范围修正后，将请求传递给下一个处理器
// next: 下一个HTTP处理函数，用于处理修正后的缩放请求
// 返回值: 包装后的HTTP处理函数
func MakeHorizontalScalingHandler(next http.HandlerFunc) http.HandlerFunc {
	// 打印水平缩放请求的彩色日志
	fmt.Printf("\033[32;1m horizontal scaling request\n\033[0m\n")
	return func(w http.ResponseWriter, r *http.Request) {
		// 校验请求方法，仅允许POST
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST is allowed", http.StatusMethodNotAllowed)
			return
		}

		// 校验请求体是否存在
		if r.Body == nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}

		// 读取请求体内容
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}

		// 解析请求体为ScaleServiceRequest结构
		scaleRequest := types.ScaleServiceRequest{}
		if err := json.Unmarshal(body, &scaleRequest); err != nil {
			http.Error(w, "Error unmarshalling request body", http.StatusBadRequest)
			return
		}

		// 校验并修正副本数：小于1时设为最小默认值
		if scaleRequest.Replicas < 1 {
			scaleRequest.Replicas = 1
		}

		// 校验并修正副本数：大于最大默认值时设为最大默认值
		if scaleRequest.Replicas > DefaultMaxReplicas {
			scaleRequest.Replicas = DefaultMaxReplicas
		}

		// 将修正后的请求重新序列化为JSON
		upstreamReq, _ := json.Marshal(scaleRequest)
		// 恢复请求体为io.ReadCloser类型，以便下一个处理器读取
		r.Body = io.NopCloser(bytes.NewBuffer(upstreamReq))

		// 将修正后的请求传递给下一个处理器
		next.ServeHTTP(w, r)
	}
}
