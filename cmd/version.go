package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// 版本信息变量
var (
	Version = "v1.0.0"
	Date    = "2024-12-19"
)

// SetVersionInfo 设置版本信息
func SetVersionInfo(version, date string) {
	if version != "" {
		Version = version
	}
	if date != "" {
		Date = date
	}
}

// versionCmd 版本命令
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Long:  `显示app-manager的版本信息和构建日期。`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("app-manager version %s\n", Version)
		fmt.Printf("Build date: %s\n", Date)
		fmt.Printf("Go version: %s\n", "go1.21+")
		fmt.Printf("Platform: %s\n", "linux/windows")
		fmt.Println("")
		fmt.Println("Java应用管理工具")
		fmt.Println("Copyright (c) 2024")
	},
}