package main

import (
	"app-manager-go-v1/cmd"
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"fmt"
	"os"
)

// 版本信息，编译时注入
var (
	Version = "v1.0.0"
	Date    = "2024-12-19"
)

func main() {
	// 初始化日志
	logger.Init()

	// 初始化配置
	config.Init()

	// 设置版本信息到cmd包
	cmd.SetVersionInfo(Version, Date)

	// 执行命令
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		logger.Error("执行命令失败: %v", err)
		os.Exit(1)
	}
}