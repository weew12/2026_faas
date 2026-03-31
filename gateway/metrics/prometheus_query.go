package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// PrometheusQuery 用于向 Prometheus 发起指标查询的客户端
// 封装了连接信息、HTTP 客户端和请求头信息
type PrometheusQuery struct {
	host             string       // Prometheus 地址
	port             int          // Prometheus 端口
	client           *http.Client // 自定义 HTTP 客户端
	userAgentVersion string       // User-Agent 标识
}

// PrometheusQueryFetcher 定义查询 Prometheus 的接口
// 外部可通过该接口替换实现
type PrometheusQueryFetcher interface {
	Fetch(query string) (*VectorQueryResponse, error)
}

// NewPrometheusQuery 创建 Prometheus 查询客户端
func NewPrometheusQuery(host string, port int, client *http.Client, userAgentVersion string) PrometheusQuery {
	return PrometheusQuery{
		client:           client,
		host:             host,
		port:             port,
		userAgentVersion: userAgentVersion,
	}
}

// Fetch 向 Prometheus API 发送查询请求，返回解析后的结果
// query：经过 URL 编码的 PromQL 查询语句
func (q PrometheusQuery) Fetch(query string) (*VectorQueryResponse, error) {
	// 构造查询 API 地址
	reqURL := fmt.Sprintf("http://%s:%d/api/v1/query?query=%s", q.host, q.port, query)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	// 设置请求标识
	req.Header.Set("User-Agent", fmt.Sprintf("openfaas-gateway/%s (Prometheus query)", q.userAgentVersion))

	// 执行请求
	res, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	// 读取响应
	bytesOut, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// 检查状态码
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from Prometheus want: %d, got: %d, body: %s", http.StatusOK, res.StatusCode, string(bytesOut))
	}

	// 反序列化 JSON 响应
	var values VectorQueryResponse
	if err := json.Unmarshal(bytesOut, &values); err != nil {
		return nil, fmt.Errorf("error unmarshaling result: %s, '%s'", err, string(bytesOut))
	}

	return &values, nil
}

// VectorQueryResponse 定义 Prometheus API 返回的 vector 类型响应结构
// 只解析需要用到的字段：函数名、状态码、指标值
type VectorQueryResponse struct {
	Data struct {
		Result []struct {
			Metric struct {
				Code         string `json:"code"`          // HTTP 状态码
				FunctionName string `json:"function_name"` // 函数全名（name.namespace）
			}
			Value []interface{} `json:"value"` // 指标值 [时间戳, "值"]
		} `json:"result"`
	} `json:"data"`
}
