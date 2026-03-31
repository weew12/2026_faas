// License: OpenFaaS Community Edition (CE) EULA
// Copyright (c) 2017,2019-2024 OpenFaaS Author(s)
// Copyright (c) Alex Ellis 2017. All rights reserved.

package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/openfaas/faas-provider/auth"
	"github.com/openfaas/faas/gateway/handlers"
	"github.com/openfaas/faas/gateway/metrics"
	"github.com/openfaas/faas/gateway/pkg/middleware"
	"github.com/openfaas/faas/gateway/plugin"
	"github.com/openfaas/faas/gateway/scaling"
	"github.com/openfaas/faas/gateway/types"
	"github.com/openfaas/faas/gateway/version"
	natsHandler "github.com/openfaas/nats-queue-worker/handler"
)

// NameExpression 函数名合法字符正则表达式
const NameExpression = "-a-zA-Z_0-9."

func main() {
	// 1. 读取环境变量配置
	osEnv := types.OsEnv{}
	readConfig := types.ReadConfig{}
	config, configErr := readConfig.Read(osEnv)

	if configErr != nil {
		log.Fatalln(configErr)
	}
	// 必须配置外部函数提供者
	if !config.UseExternalProvider() {
		log.Fatalln("You must provide an external provider via 'functions_provider_url' env-var.")
	}

	// 打印启动信息
	fmt.Printf("\033[32;1m weew12 modified version\n\033[0m\n")
	fmt.Printf("OpenFaaS Gateway - Community Edition (CE)\n"+
		"\nVersion: %s Commit: %s\nTimeouts: read=%s\twrite=%s\tupstream=%s\nFunction provider: %s\n\n",
		version.BuildVersion(),
		version.GitCommitSHA,
		config.ReadTimeout,
		config.WriteTimeout,
		config.UpstreamTimeout,
		config.FunctionsProviderURL)

	// 2. 加载服务间认证凭据（BasicAuth）
	var credentials *auth.BasicAuthCredentials
	if config.UseBasicAuth {
		var err error
		reader := auth.ReadBasicAuthFromDisk{
			SecretMountPath: config.SecretMountPath,
		}
		credentials, err = reader.Read()

		if err != nil {
			log.Panic(err.Error())
		}
	}

	var faasHandlers types.HandlerSet

	// 3. 初始化 Prometheus 指标采集
	servicePollInterval := time.Second * 5
	metricsOptions := metrics.BuildMetricsOptions()
	exporter := metrics.NewExporter(metricsOptions, credentials, config.Namespace)
	// 启动定时任务，定期同步函数副本数
	exporter.StartServiceWatcher(*config.FunctionsProviderURL, metricsOptions, "func", servicePollInterval)
	metrics.RegisterExporter(exporter)

	// 4. 创建反向代理客户端（用于转发请求到函数提供者）
	reverseProxy := types.NewHTTPClientReverseProxy(config.FunctionsProviderURL,
		config.UpstreamTimeout,
		config.MaxIdleConns,
		config.MaxIdleConnsPerHost)

	// 5. 注册请求通知器（日志 + Prometheus 指标）
	loggingNotifier := handlers.LoggingNotifier{}
	prometheusNotifier := handlers.PrometheusFunctionNotifier{
		Metrics:           &metricsOptions,
		FunctionNamespace: config.Namespace,
	}
	// 函数代理用通知器
	functionNotifiers := []handlers.HTTPNotifier{loggingNotifier, prometheusNotifier}
	// 系统接口用通知器
	forwardingNotifiers := []handlers.HTTPNotifier{loggingNotifier}
	// 静默通知器
	quietNotifier := []handlers.HTTPNotifier{}

	// 6. URL 解析与路径转换
	urlResolver := middleware.SingleHostBaseURLResolver{BaseURL: config.FunctionsProviderURL.String()}
	var functionURLResolver middleware.BaseURLResolver
	var functionURLTransformer middleware.URLPathTransformer
	nilURLTransformer := middleware.TransparentURLPathTransformer{}
	trimURLTransformer := middleware.FunctionPrefixTrimmingURLPathTransformer{}

	functionURLResolver = urlResolver
	functionURLTransformer = nilURLTransformer

	// 7. 服务认证注入器
	var serviceAuthInjector middleware.AuthInjector
	if config.UseBasicAuth {
		serviceAuthInjector = &middleware.BasicAuthInjector{Credentials: credentials}
	}

	// 8. 外部服务查询器（用于查询函数元数据）
	externalServiceQuery := plugin.NewExternalServiceQuery(*config.FunctionsProviderURL, serviceAuthInjector)

	// 9. 配置自动扩缩容（0 → N）
	scalingConfig := scaling.ScalingConfig{
		MaxPollCount:         uint(1000),
		SetScaleRetries:      uint(20),
		FunctionPollInterval: time.Millisecond * 100,
		CacheExpiry:          time.Millisecond * 250, // 缓存过期时间
		ServiceQuery:         externalServiceQuery,
	}
	// 函数注解缓存
	functionAnnotationCache := scaling.NewFunctionCache(scalingConfig.CacheExpiry)
	cachedFunctionQuery := scaling.NewCachedFunctionQuery(functionAnnotationCache, externalServiceQuery)

	// 10. 核心代理处理器（带请求ID追踪）
	faasHandlers.Proxy = handlers.MakeCallIDMiddleware(
		handlers.MakeForwardingProxyHandler(reverseProxy, functionNotifiers, functionURLResolver, functionURLTransformer, nil),
	)

	// 11. 注册所有系统 API 处理器
	faasHandlers.ListFunctions = handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector)
	faasHandlers.DeployFunction = handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector)
	faasHandlers.DeleteFunction = handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector)
	faasHandlers.UpdateFunction = handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector)
	faasHandlers.FunctionStatus = handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector)

	faasHandlers.InfoHandler = handlers.MakeInfoHandler(handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector))
	faasHandlers.TelemetryHandler = handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, nil)

	faasHandlers.SecretHandler = handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector)
	faasHandlers.NamespaceListerHandler = handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector)
	faasHandlers.NamespaceMutatorHandler = handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector)

	// 告警处理器（Prometheus 告警触发扩缩容）
	faasHandlers.Alert = handlers.MakeNotifierWrapper(
		handlers.MakeAlertHandler(externalServiceQuery, config.Namespace),
		quietNotifier,
	)

	// 日志代理处理器
	faasHandlers.LogProxyHandler = handlers.NewLogHandlerFunc(*config.LogsProviderURL, config.WriteTimeout)

	// 12. 启用 0 → N 自动扩容
	functionProxy := faasHandlers.Proxy
	if config.ScaleFromZero {
		scalingFunctionCache := scaling.NewFunctionCache(scalingConfig.CacheExpiry)
		scaler := scaling.NewFunctionScaler(scalingConfig, scalingFunctionCache)
		// 包装代理，请求前先扩容
		functionProxy = handlers.MakeScalingHandler(functionProxy, scaler, scalingConfig, config.Namespace)
	}

	// 13. 启用 NATS 异步调用
	if config.UseNATS() {
		log.Println("Async enabled: Using NATS Streaming")
		log.Println("Deprecation Notice: NATS Streaming is no longer maintained and won't receive updates from June 2023")

		maxReconnect := 60
		interval := time.Second * 2
		defaultNATSConfig := natsHandler.NewDefaultNATSConfig(maxReconnect, interval)

		// 创建 NATS 队列
		natsQueue, queueErr := natsHandler.CreateNATSQueue(*config.NATSAddress, *config.NATSPort, *config.NATSClusterName, *config.NATSChannel, defaultNATSConfig)
		if queueErr != nil {
			log.Fatalln(queueErr)
		}

		// 注册异步代理
		faasHandlers.QueuedProxy = handlers.MakeNotifierWrapper(
			handlers.MakeCallIDMiddleware(handlers.MakeQueuedProxy(metricsOptions, natsQueue, trimURLTransformer, config.Namespace, cachedFunctionQuery)),
			forwardingNotifiers,
		)
	}

	// 14. 为函数列表添加 Prometheus 调用指标
	prometheusQuery := metrics.NewPrometheusQuery(config.PrometheusHost, config.PrometheusPort, http.DefaultClient, version.BuildVersion())
	faasHandlers.ListFunctions = metrics.AddMetricsHandler(faasHandlers.ListFunctions, prometheusQuery)
	faasHandlers.ScaleFunction = scaling.MakeHorizontalScalingHandler(handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector))

	// 15. 为所有敏感接口添加 BasicAuth 保护
	if credentials != nil {
		faasHandlers.Alert = auth.DecorateWithBasicAuth(faasHandlers.Alert, credentials)
		faasHandlers.UpdateFunction = auth.DecorateWithBasicAuth(faasHandlers.UpdateFunction, credentials)
		faasHandlers.DeleteFunction = auth.DecorateWithBasicAuth(faasHandlers.DeleteFunction, credentials)
		faasHandlers.DeployFunction = auth.DecorateWithBasicAuth(faasHandlers.DeployFunction, credentials)
		faasHandlers.ListFunctions = auth.DecorateWithBasicAuth(faasHandlers.ListFunctions, credentials)
		faasHandlers.ScaleFunction = auth.DecorateWithBasicAuth(faasHandlers.ScaleFunction, credentials)
		faasHandlers.FunctionStatus = auth.DecorateWithBasicAuth(faasHandlers.FunctionStatus, credentials)
		faasHandlers.InfoHandler = auth.DecorateWithBasicAuth(faasHandlers.InfoHandler, credentials)
		faasHandlers.SecretHandler = auth.DecorateWithBasicAuth(faasHandlers.SecretHandler, credentials)
		faasHandlers.LogProxyHandler = auth.DecorateWithBasicAuth(faasHandlers.LogProxyHandler, credentials)
		faasHandlers.NamespaceListerHandler = auth.DecorateWithBasicAuth(faasHandlers.NamespaceListerHandler, credentials)
		faasHandlers.NamespaceMutatorHandler = auth.DecorateWithBasicAuth(faasHandlers.NamespaceMutatorHandler, credentials)
	}

	// 16. 注册 HTTP 路由
	r := mux.NewRouter()

	// 同步函数调用路由
	r.HandleFunc("/function/{name:["+NameExpression+"]+}", functionProxy)
	r.HandleFunc("/function/{name:["+NameExpression+"]+}/", functionProxy)
	r.HandleFunc("/function/{name:["+NameExpression+"]+}/{params:.*}", functionProxy)

	// 系统 API
	r.HandleFunc("/system/info", faasHandlers.InfoHandler).Methods(http.MethodGet)
	r.HandleFunc("/system/telemetry", faasHandlers.TelemetryHandler).Methods(http.MethodGet)
	r.HandleFunc("/system/alert", faasHandlers.Alert).Methods(http.MethodPost)

	r.HandleFunc("/system/function/{name:["+NameExpression+"]+}", faasHandlers.FunctionStatus).Methods(http.MethodGet)
	r.HandleFunc("/system/functions", faasHandlers.ListFunctions).Methods(http.MethodGet)
	r.HandleFunc("/system/functions", faasHandlers.DeployFunction).Methods(http.MethodPost)
	r.HandleFunc("/system/functions", faasHandlers.DeleteFunction).Methods(http.MethodDelete)
	r.HandleFunc("/system/functions", faasHandlers.UpdateFunction).Methods(http.MethodPut)
	r.HandleFunc("/system/scale-function/{name:["+NameExpression+"]+}", faasHandlers.ScaleFunction).Methods(http.MethodPost)

	r.HandleFunc("/system/secrets", faasHandlers.SecretHandler).Methods(http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete)
	r.HandleFunc("/system/logs", faasHandlers.LogProxyHandler).Methods(http.MethodGet)
	r.HandleFunc("/system/namespaces", faasHandlers.NamespaceListerHandler).Methods(http.MethodGet)
	r.HandleFunc("/system/namespace/{namespace:["+NameExpression+"]*}", faasHandlers.NamespaceMutatorHandler).
		Methods(http.MethodPost, http.MethodDelete, http.MethodPut, http.MethodGet)

	// 异步函数调用路由
	if faasHandlers.QueuedProxy != nil {
		r.HandleFunc("/async-function/{name:["+NameExpression+"]+}/", faasHandlers.QueuedProxy).Methods(http.MethodPost)
		r.HandleFunc("/async-function/{name:["+NameExpression+"]+}", faasHandlers.QueuedProxy).Methods(http.MethodPost)
		r.HandleFunc("/async-function/{name:["+NameExpression+"]+}/{params:.*}", faasHandlers.QueuedProxy).Methods(http.MethodPost)
	}

	// 17. 静态 UI 资源 + CORS
	fs := http.FileServer(http.Dir("./assets/"))
	allowedCORSHost := "raw.githubusercontent.com"
	fsCORS := handlers.DecorateWithCORS(fs, allowedCORSHost)
	uiHandler := http.StripPrefix("/ui", fsCORS)

	if credentials != nil {
		r.PathPrefix("/ui/").Handler(auth.DecorateWithBasicAuth(uiHandler.ServeHTTP, credentials)).Methods(http.MethodGet)
	} else {
		r.PathPrefix("/ui/").Handler(uiHandler).Methods(http.MethodGet)
	}

	// 18. 启动指标服务器（独立端口 8082）
	go runMetricsServer()

	// 健康检查
	r.HandleFunc("/healthz", handlers.MakeForwardingProxyHandler(reverseProxy, forwardingNotifiers, urlResolver, nilURLTransformer, serviceAuthInjector)).Methods(http.MethodGet)
	// 根路径重定向到 UI
	r.Handle("/", http.RedirectHandler("/ui/", http.StatusTemporaryRedirect)).Methods(http.MethodGet)

	// 19. 启动主网关服务（端口 8080）
	tcpPort := 8080
	s := &http.Server{
		Addr:           fmt.Sprintf(":%d", tcpPort),
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		MaxHeaderBytes: http.DefaultMaxHeaderBytes,
		Handler:        r,
	}

	log.Fatal(s.ListenAndServe())
}

// runMetricsServer 启动独立的指标 HTTP 服务（8082 端口）
// 仅用于内部网络暴露 Prometheus 指标
func runMetricsServer() {
	metricsHandler := metrics.PrometheusHandler()
	router := mux.NewRouter()
	router.Handle("/metrics", metricsHandler)
	router.HandleFunc("/healthz", handlers.HealthzHandler)

	port := 8082
	readTimeout := 5 * time.Second
	writeTimeout := 5 * time.Second

	s := &http.Server{
		Addr:           fmt.Sprintf(":%d", port),
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		MaxHeaderBytes: http.DefaultMaxHeaderBytes,
		Handler:        router,
	}

	log.Fatal(s.ListenAndServe())
}
