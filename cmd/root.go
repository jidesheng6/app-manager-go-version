package cmd

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
)

// rootCmd 根命令
var rootCmd = &cobra.Command{
	Use:   "app-manager-go",
	Short: "一个用于管理Java应用的CLI工具",
	Long: `一个功能更全面、性能更好、更易于扩展的Java应用管理工具。
它旨在取代旧的app-manager.sh脚本，提供更一致和强大的功能。`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// 初始化配置
		config.Init()
		
		// 加载配置文件
		if err := config.LoadConfig(cfgFile); err != nil {
			logger.Warning("加载配置文件失败: %v", err)
		}
		
		// 设置详细输出模式
		if verbose {
			logger.Info("详细输出模式已启用")
		}
	},
}

// Signal-handling functions, to be set by the main package
var (
	DisableSignalHandling func()
	EnableSignalHandling  func()
)

// Execute 执行根命令
func Execute() error {
	// 在执行前加载配置文件
	return rootCmd.Execute()
}

func init() {
	// 全局标志
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认查找 .app-manager.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "详细输出")
	
	// 添加子命令
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(logrotateCmd)
	rootCmd.AddCommand(versionCmd)
}

// checkProjectInit 检查项目是否已初始化
func checkProjectInit() error {
	if !config.IsProjectInitialized() {
		return fmt.Errorf(`项目尚未初始化。

请先运行以下命令初始化项目：
  app-manager init

或者在已有项目目录中运行：
  app-manager init /path/to/project`)
	}
	return nil
}

// handleError 统一错误处理
func handleError(err error, exitCode int) {
	if err != nil {
		logger.Error("%v", err)
		os.Exit(exitCode)
	}
}