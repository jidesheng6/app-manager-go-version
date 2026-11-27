package cmd

import (
	"fmt"
	"runtime"

	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/version"

	"github.com/spf13/cobra"
)

// 版本信息
var (
	Version     = "v1.0.0"
	Date        = "unknown"
	Author      = "unknown"
	Description = "A Java application management tool"
)

// SetVersionInfo 设置版本信息
func SetVersionInfo(version, date, author, description string) {
	if version != "" {
		Version = version
	}
	if date != "" {
		Date = date
	}
	if author != "" {
		Author = author
	}
	if description != "" {
		Description = description
	}
}

// versionCmd 版本命令
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Long: `显示版本信息，包括构建时注入的 git 提交号/分支，以及当前目录的 git 状态。

部署后如果怀疑"跑的不是最新代码"，用本命令或 'status' 对比两个提交号即可。`,
	Run: func(cmd *cobra.Command, args []string) {
		programName := config.GetProgramName()

		fmt.Printf("%s version %s\n", programName, Version)
		fmt.Printf("Build:   %s\n", version.BuildLine(Version))
		fmt.Printf("Author:  %s\n", Author)
		fmt.Printf("Go:      %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Description: %s\n", Description)

		// 当前目录的 git 状态（只读）
		live := version.Live(config.GetProjectRoot())
		fmt.Printf("\n代码目录: %s\n", config.GetProjectRoot())
		fmt.Printf("当前仓库: %s\n", version.RepoLinePlain(live))
		if hint := version.DriftHint(version.Embedded(), live); hint != "" {
			fmt.Printf("\n注意: %s\n", hint)
		}
	},
}
