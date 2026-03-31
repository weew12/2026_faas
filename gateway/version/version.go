// Package version 管理应用版本信息
package version

var (
	// Version 正式发布版本号
	Version string

	// GitCommitSHA 最新提交的Git SHA
	GitCommitSHA string

	// GitCommitMessage Git提交信息
	GitCommitMessage = "See GitHub for latest changes"

	// DevVersion 开发版本标识
	DevVersion = "dev"
)

// BuildVersion 返回当前版本，未设置正式版本则返回开发版本
func BuildVersion() string {
	if len(Version) == 0 {
		return DevVersion
	}
	return Version
}
