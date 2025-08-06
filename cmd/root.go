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
	Use:   "app-manager",
	Short: "Java应用管理工具",
	Long: `app-manager 是一个用于管理Java应用程序的命令行工具。

支持的功能：
  • 应用生命周期管理（启动、停止、重启、状态查询）
  • 项目初始化和目录结构管理
  • 应用更新和备份
  • 日志查看和轮转
  • Systemd服务集成
  • 配置文件管理

使用示例：
  app-manager init                    # 初始化项目
  app-manager start myapp             # 启动应用
  app-manager status                  # 查看所有应用状态
  app-manager logs myapp -f           # 实时查看应用日志
  app-manager backup                  # 备份项目文件`,
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

// Execute 执行根命令
func Execute() error {
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