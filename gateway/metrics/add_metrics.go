package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	types "github.com/openfaas/faas-provider/types"
)

// AddMetricsHandler 为函数列表接口包装 Prometheus 指标增强逻辑
// 先调用上游获取函数列表，再从 Prometheus 查询调用次数并合并到结果中
func AddMetricsHandler(handler http.HandlerFunc, prometheusQuery PrometheusQueryFetcher) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		// 捕获上游函数列表接口的响应
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, r)
		upstreamCall := recorder.Result()

		if upstreamCall.Body == nil {
			log.Println("Upstream call had empty body.")
			return
		}

		defer upstreamCall.Body.Close()
		upstreamBody, _ := io.ReadAll(upstreamCall.Body)

		// 上游返回非200，直接透传错误
		if recorder.Code != http.StatusOK {
			log.Printf("List functions responded with code %d, body: %s",
				recorder.Code,
				string(upstreamBody))
			http.Error(w, string(upstreamBody), recorder.Code)
			return
		}

		// 解析函数列表
		var functions []types.FunctionStatus
		err := json.Unmarshal(upstreamBody, &functions)
		if err != nil {
			log.Printf("Metrics upstream error: %s, value: %s", err, string(upstreamBody))

			http.Error(w, "Unable to parse list of functions from provider", http.StatusInternalServerError)
			return
		}

		// 清空原有调用计数，准备从 Prometheus 重新填充
		for i := range functions {
			functions[i].InvocationCount = 0
		}

		// 查询 Prometheus 指标并合并
		if len(functions) > 0 {

			ns := functions[0].Namespace
			// 构造 Prometheus 查询语句：按命名空间聚合函数调用次数
			q := fmt.Sprintf(`sum(gateway_function_invocation_total{function_name=~".*.%s"}) by (function_name)`, ns)

			// 执行查询
			results, err := prometheusQuery.Fetch(url.QueryEscape(q))
			if err != nil {
				log.Printf("Error querying Prometheus: %s\n", err.Error())
			}
			// 将指标合并到函数列表中
			mixIn(&functions, results)
		}

		// 返回合并指标后的最终结果
		bytesOut, err := json.Marshal(functions)
		if err != nil {
			log.Printf("Error serializing functions: %s", err)
			http.Error(w, "Error writing response after adding metrics", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(bytesOut)
	}
}

// mixIn 将 Prometheus 查询到的调用次数指标，合并到函数状态列表中
// 匹配函数名（name.namespace），将指标值写入 InvocationCount
func mixIn(functions *[]types.FunctionStatus, metrics *VectorQueryResponse) {

	if functions == nil {
		return
	}

	// 遍历函数，匹配指标并累加调用次数
	for i, function := range *functions {
		for _, v := range metrics.Data.Result {

			// 匹配函数全名：函数名.命名空间
			if v.Metric.FunctionName == fmt.Sprintf("%s.%s", function.Name, function.Namespace) {
				metricValue := v.Value[1]
				switch value := metricValue.(type) {
				case string:
					// 解析字符串格式的计数值
					f, err := strconv.ParseFloat(value, 64)
					if err != nil {
						log.Printf("add_metrics: unable to convert value %q for metric: %s", value, err)
						continue
					}
					// 累加调用次数
					(*functions)[i].InvocationCount += f
				}
			}
		}
	}
}
