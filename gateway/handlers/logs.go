package handlers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// crlf 换行符
const crlf = "\r\n"

// upstreamLogsEndpoint 上游日志服务的API端点路径
const upstreamLogsEndpoint = "/system/logs"

// writerFlusher 组合接口，要求同时实现写入和刷新
type writerFlusher interface {
	io.Writer
	http.Flusher
}

// unbufferedWriter 无缓冲写入器，每次写入后立即刷新，保证日志实时推送到客户端
type unbufferedWriter struct {
	dst writerFlusher
}

// Write 写入数据并立即刷新，实现实时日志流输出
func (u *unbufferedWriter) Write(p []byte) (n int, err error) {
	n, err = u.dst.Write(p)
	u.dst.Flush()
	return n, err
}

// NewLogHandlerFunc 创建日志代理的HTTP处理函数
// 将客户端的日志请求转发到上游日志服务，并以流的形式返回给客户端
func NewLogHandlerFunc(logProvider url.URL, timeout time.Duration) http.HandlerFunc {
	writeRequestURI := false
	if _, exists := os.LookupEnv("write_request_uri"); exists {
		writeRequestURI = exists
	}

	// 拼接上游日志服务基础地址
	upstreamLogProviderBase := strings.TrimSuffix(logProvider.String(), "/")

	return func(w http.ResponseWriter, r *http.Request) {
		// 设置请求超时上下文
		ctx, cancelQuery := context.WithTimeout(r.Context(), timeout)
		defer cancelQuery()

		if r.Body != nil {
			defer r.Body.Close()
		}

		// 构建转发到上游日志服务的请求
		logRequest := buildUpstreamRequest(r, upstreamLogProviderBase, upstreamLogsEndpoint)
		if logRequest.Body != nil {
			defer logRequest.Body.Close()
		}

		// 检查响应是否支持连接关闭通知（客户端断开时停止转发）
		cn, ok := w.(http.CloseNotifier)
		if !ok {
			log.Println("LogHandler: response is not a CloseNotifier, required for streaming response")
			http.NotFound(w, r)
			return
		}

		// 检查响应是否支持实时刷新（日志流必须）
		wf, ok := w.(writerFlusher)
		if !ok {
			log.Println("LogHandler: response is not a Flusher, required for streaming response")
			http.NotFound(w, r)
			return
		}

		// 调试日志：打印转发的目标地址
		if writeRequestURI {
			log.Printf("LogProxy: proxying request to %s %s\n", logRequest.Host, logRequest.URL.String())
		}

		// 创建可取消的上下文，用于客户端断开时停止请求
		ctx, cancel := context.WithCancel(ctx)
		logRequest = logRequest.WithContext(ctx)
		defer cancel()

		// 直接使用底层RoundTrip发送请求，保持长连接
		logResp, err := http.DefaultTransport.RoundTrip(logRequest)
		if err != nil {
			log.Printf("LogProxy: forwarding request failed: %s\n", err.Error())
			http.Error(w, "log request failed", http.StatusInternalServerError)
			return
		}
		defer logResp.Body.Close()

		// 根据上游响应状态码处理
		switch logResp.StatusCode {
		case http.StatusNotFound, http.StatusNotImplemented:
			w.WriteHeader(http.StatusNotImplemented)
			return

		case http.StatusOK:
			// 启动日志流转发：监听客户端断开和数据复制完成
			select {
			case err := <-copyNotify(&unbufferedWriter{wf}, logResp.Body):
				if err != nil {
					log.Printf("LogProxy: error while copy: %s", err.Error())
				}
			case <-cn.CloseNotify():
				log.Println("LogProxy: client connection closed")
			}

		default:
			http.Error(w, fmt.Sprintf("unknown log request error (%v)", logResp.StatusCode), http.StatusInternalServerError)
		}

		return
	}
}

// copyNotify 异步复制数据流，并通过channel返回错误
// 用于在后台复制日志流，不阻塞主协程
func copyNotify(destination io.Writer, source io.Reader) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(destination, source)
		done <- err
	}()
	return done
}
