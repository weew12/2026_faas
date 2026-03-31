// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)

// Copyright (c) Alex Ellis 2017. All rights reserved.

// Package types 定义OpenFaaS网关核心配置类型、环境变量解析逻辑与相关抽象接口，提供网关启动与运行所需的全量配置管理能力
package types

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

// OsEnv 封装系统环境变量获取能力，实现HasEnv接口，用于生产环境直接读取系统环境变量
type OsEnv struct {
}

// Getenv 封装os.Getenv原生方法，获取指定key对应的环境变量值
func (OsEnv) Getenv(key string) string {
	return os.Getenv(key)
}

// HasEnv 定义环境变量获取的抽象接口，用于解耦配置读取逻辑与系统环境变量的直接依赖，方便单元测试 mock
type HasEnv interface {
	Getenv(key string) string
}

// ReadConfig 提供网关配置的读取、解析与校验能力，核心入口为Read方法
type ReadConfig struct {
}

// parseBoolValue 解析字符串格式的布尔配置值，仅当输入为"true"时返回true，其余所有情况均返回false
func parseBoolValue(val string) bool {
	if val == "true" {
		return true
	}
	return false
}

// parseIntOrDurationValue 解析时间类配置，兼容两种格式：纯数字（自动识别为秒）、Go标准时长格式（如"30s"、"2m"、"1h"）
// val: 待解析的配置字符串
// fallback: 解析失败或输入为空时使用的默认兜底值
// 返回值: 解析成功的时长，或解析失败时的fallback默认值
func parseIntOrDurationValue(val string, fallback time.Duration) time.Duration {
	if len(val) > 0 {
		parsedVal, parseErr := strconv.Atoi(val)
		if parseErr == nil && parsedVal >= 0 {
			return time.Duration(parsedVal) * time.Second
		}
	}

	duration, durationErr := time.ParseDuration(val)
	if durationErr != nil {
		return fallback
	}
	return duration
}

// Read 从环境变量中加载、解析并校验网关服务的全量配置
// hasEnv: 环境变量获取接口，生产环境传入OsEnv实例，测试环境可传入mock实现
// 返回值: 解析完成的GatewayConfig配置实例，解析/校验失败时返回对应的错误信息
func (ReadConfig) Read(hasEnv HasEnv) (*GatewayConfig, error) {
	cfg := GatewayConfig{
		PrometheusHost: "prometheus",
		PrometheusPort: 9090,
	}

	defaultDuration := time.Second * 60

	cfg.ReadTimeout = parseIntOrDurationValue(hasEnv.Getenv("read_timeout"), defaultDuration)
	cfg.WriteTimeout = parseIntOrDurationValue(hasEnv.Getenv("write_timeout"), defaultDuration)
	cfg.UpstreamTimeout = parseIntOrDurationValue(hasEnv.Getenv("upstream_timeout"), defaultDuration)

	if len(hasEnv.Getenv("functions_provider_url")) > 0 {
		var err error
		cfg.FunctionsProviderURL, err = url.Parse(hasEnv.Getenv("functions_provider_url"))
		if err != nil {
			return nil, fmt.Errorf("if functions_provider_url is provided, then it should be a valid URL, error: %s", err)
		}
	}

	if len(hasEnv.Getenv("logs_provider_url")) > 0 {
		var err error
		cfg.LogsProviderURL, err = url.Parse(hasEnv.Getenv("logs_provider_url"))
		if err != nil {
			return nil, fmt.Errorf("if logs_provider_url is provided, then it should be a valid URL, error: %s", err)
		}
	} else if cfg.FunctionsProviderURL != nil {
		cfg.LogsProviderURL, _ = url.Parse(cfg.FunctionsProviderURL.String())
	}

	faasNATSAddress := hasEnv.Getenv("faas_nats_address")
	if len(faasNATSAddress) > 0 {
		cfg.NATSAddress = &faasNATSAddress
	}

	faasNATSPort := hasEnv.Getenv("faas_nats_port")
	if len(faasNATSPort) > 0 {
		port, err := strconv.Atoi(faasNATSPort)
		if err == nil {
			cfg.NATSPort = &port
		} else {
			return nil, fmt.Errorf("faas_nats_port invalid number: %s", faasNATSPort)
		}
	}

	faasNATSClusterName := hasEnv.Getenv("faas_nats_cluster_name")
	if len(faasNATSClusterName) > 0 {
		cfg.NATSClusterName = &faasNATSClusterName
	} else {
		v := "faas-cluster"
		cfg.NATSClusterName = &v
	}

	faasNATSChannel := hasEnv.Getenv("faas_nats_channel")
	if len(faasNATSChannel) > 0 {
		cfg.NATSChannel = &faasNATSChannel
	} else {
		v := "faas-request"
		cfg.NATSChannel = &v
	}

	prometheusPort := hasEnv.Getenv("faas_prometheus_port")
	if len(prometheusPort) > 0 {
		prometheusPortVal, err := strconv.Atoi(prometheusPort)
		if err != nil {
			return nil, fmt.Errorf("faas_prometheus_port invalid number: %s", faasNATSPort)
		}
		cfg.PrometheusPort = prometheusPortVal

	}

	prometheusHost := hasEnv.Getenv("faas_prometheus_host")
	if len(prometheusHost) > 0 {
		cfg.PrometheusHost = prometheusHost
	}

	cfg.UseBasicAuth = parseBoolValue(hasEnv.Getenv("basic_auth"))

	secretPath := hasEnv.Getenv("secret_mount_path")
	if len(secretPath) == 0 {
		secretPath = "/run/secrets/"
	}
	cfg.SecretMountPath = secretPath
	cfg.ScaleFromZero = parseBoolValue(hasEnv.Getenv("scale_from_zero"))

	cfg.MaxIdleConns = 1024
	cfg.MaxIdleConnsPerHost = 1024

	maxIdleConns := hasEnv.Getenv("max_idle_conns")
	if len(maxIdleConns) > 0 {
		val, err := strconv.Atoi(maxIdleConns)
		if err != nil {
			return nil, fmt.Errorf("invalid value for max_idle_conns: %s", maxIdleConns)
		}
		cfg.MaxIdleConns = val

	}

	maxIdleConnsPerHost := hasEnv.Getenv("max_idle_conns_per_host")
	if len(maxIdleConnsPerHost) > 0 {
		val, err := strconv.Atoi(maxIdleConnsPerHost)
		if err != nil {
			return nil, fmt.Errorf("invalid value for max_idle_conns_per_host: %s", maxIdleConnsPerHost)
		}
		cfg.MaxIdleConnsPerHost = val

	}

	cfg.AuthProxyURL = hasEnv.Getenv("auth_proxy_url")
	cfg.AuthProxyPassBody = parseBoolValue(hasEnv.Getenv("auth_proxy_pass_body"))

	cfg.Namespace = hasEnv.Getenv("function_namespace")

	return &cfg, nil
}

// GatewayConfig 定义OpenFaaS API网关服务进程的全量配置项，包含HTTP、上游服务、中间件、可观测性等核心配置
type GatewayConfig struct {

	// 网关从客户端读取HTTP请求的超时时间
	ReadTimeout time.Duration

	// 网关向客户端写入函数执行HTTP响应的超时时间
	WriteTimeout time.Duration

	// 网关向上游函数服务发起HTTP调用的最大超时时间
	UpstreamTimeout time.Duration

	// 外部函数提供商的访问URL，网关将通过该地址与函数运行时交互
	FunctionsProviderURL *url.URL

	// 外部函数日志提供商的访问URL，未配置时默认复用函数提供商URL
	LogsProviderURL *url.URL

	// NATS服务地址，异步函数调用模式必填项
	NATSAddress *string

	// NATS服务端口，异步函数调用模式必填项
	NATSPort *int

	// NATS Streaming集群名称，异步函数调用模式必填项
	NATSClusterName *string

	// 用于异步函数调用的NATS Streaming通道名称
	NATSChannel *string

	// Prometheus监控服务的连接主机地址
	PrometheusHost string

	// Prometheus监控服务的连接端口
	PrometheusPort int

	// 是否开启基础认证，开启后网关将从指定挂载路径读取密钥完成请求认证
	UseBasicAuth bool

	// 嵌入式基础认证的密钥文件挂载路径
	SecretMountPath string

	// 是否开启零副本扩容能力，允许网关将副本数为0的函数自动扩容到配置的最小副本数
	ScaleFromZero bool

	// HTTP客户端全局最大空闲连接数，默认值1024，用于高并发场景下的HTTP代理性能调优
	MaxIdleConns int

	// HTTP客户端单目标主机最大空闲连接数，默认值1024，用于高并发场景下的HTTP代理性能调优
	MaxIdleConnsPerHost int

	// 外部认证代理URL，为空时禁用认证代理，配置有效URL时网关会将请求转发至该地址完成认证（示例值：http://basic-auth.openfaas:8080/validate）
	AuthProxyURL string

	// 是否将原始请求体转发给认证代理服务
	AuthProxyPassBody bool

	// 函数资源所在的Kubernetes命名空间
	Namespace string
}

// UseNATS 判断当前配置是否启用NATS异步函数模式，仅当NATS地址与端口均完成配置时返回true
func (g *GatewayConfig) UseNATS() bool {
	return g.NATSPort != nil &&
		g.NATSAddress != nil
}

// UseExternalProvider 判断当前配置是否使用外部函数提供商，当前版本所有函数运行时均需配置外部提供商
func (g *GatewayConfig) UseExternalProvider() bool {
	return g.FunctionsProviderURL != nil
}
