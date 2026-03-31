package metrics

import "github.com/openfaas/faas-provider/auth"

// MetadataQuery 用于携带认证信息，查询服务元数据（如函数、命名空间、指标）
type MetadataQuery struct {
	Credentials *auth.BasicAuthCredentials // 基础认证凭证
}

// NewMetadataQuery 创建一个元数据查询器，传入认证信息
func NewMetadataQuery(credentials *auth.BasicAuthCredentials) *MetadataQuery {
	return &MetadataQuery{Credentials: credentials}
}
