package cmd

import (
	"fmt"
	"app-manager-go-v1/internal/config"
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
	Run: func(cmd *cobra.Command, args []string) {
		programName := config.GetProgramName()
		fmt.Printf("%s version %s\n", programName, Version)
		fmt.Printf("Description: %s\n", Description)
		fmt.Printf("Author: %s\n", Author)
		fmt.Printf("Build Date: %s\n", Date)
		fmt.Printf("Go version: %s\n", "go1.21+")
	},
}