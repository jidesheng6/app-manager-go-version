package cmd

import (
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/services"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	restartAll bool
)

// restartCmd 重启命令
var restartCmd = &cobra.Command{
	Use:   "restart [application_name]",
	Short: "重启Java应用程序",
	Long: `重启指定的Java应用程序或所有应用程序。

应用程序名称可以是完整的文件名（如 myapp.jar）或者应用名称（如 myapp）。
如果不指定应用名称，将重启所有应用程序（包括已停止的）。

重启过程：
1. 如果应用正在运行，先优雅停止
2. 等待应用完全停止
3. 重新启动应用

使用示例：
  app-manager restart                 # 重启所有应用
  app-manager restart myapp           # 重启名为 myapp 的应用
  app-manager restart myapp.jar       # 重启 myapp.jar 应用
  app-manager restart --all           # 重启所有应用（与不传参数效果相同）`,
	Args: func(cmd *cobra.Command, args []string) error {
		if restartAll && len(args) > 0 {
			return fmt.Errorf("不能同时指定应用名称和 --all 标志")
		}
		return nil
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		handleError(checkProjectInit(), 1)
	},
	Run: func(cmd *cobra.Command, args []string) {
		appService := services.NewApplicationService()
		
		if restartAll || len(args) == 0 {
			// 重启所有应用
			if err := restartAllApplications(appService); err != nil {
				handleError(err, 1)
			}
		} else {
			// 重启指定应用
			appName := args[0]
			if err := restartSingleApplication(appService, appName); err != nil {
				handleError(err, 1)
			}
		}
	},
}

func init() {
	restartCmd.Flags().BoolVarP(&restartAll, "all", "a", false, "重启所有应用")
}

// restartSingleApplication 重启单个应用
func restartSingleApplication(appService *services.ApplicationService, appName string) error {
	logger.Title("重启应用")
	
	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	// 查找应用（支持交互式选择）
	app := findApplicationWithInteractiveSelection(apps, appName)
	if app == nil {
		return fmt.Errorf("未找到应用: %s", appName)
	}
	
	logger.Info("重启应用: %s", app.Name)
	logger.Info("应用文件: %s", app.FilePath)
	
	// 显示当前状态
	if app.IsRunning() {
		logger.Info("当前状态: 运行中 (PID: %d)", app.PID)
	} else {
		logger.Info("当前状态: 已停止")
	}
	
	// 重启应用
	if err := appService.RestartApplication(app); err != nil {
		return fmt.Errorf("重启应用失败: %w", err)
	}
	
	logger.Success("应用 %s 重启成功 (PID: %d)", app.Name, app.PID)
	
	// 显示日志文件位置
	if app.LogFile != "" {
		logger.Info("日志文件: %s", app.LogFile)
		logger.Info("查看日志: app-manager logs %s -f", app.Name)
	}
	
	return nil
}

// restartAllApplications 重启所有应用
func restartAllApplications(appService *services.ApplicationService) error {
	logger.Title("重启所有应用")
	
	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	if len(apps) == 0 {
		logger.Info("没有找到任何应用")
		return nil
	}
	
	logger.Info("找到 %d 个应用", len(apps))
	
	successCount := 0
	failedCount := 0
	
	for _, app := range apps {
		logger.Info("重启应用: %s", app.Name)
		
		// 显示当前状态
		if app.IsRunning() {
			logger.Info("  当前状态: 运行中 (PID: %d)", app.PID)
		} else {
			logger.Info("  当前状态: 已停止")
		}
		
		if err := appService.RestartApplication(app); err != nil {
			logger.Error("重启应用失败 %s: %v", app.Name, err)
			failedCount++
		} else {
			logger.Success("应用 %s 重启成功 (PID: %d)", app.Name, app.PID)
			successCount++
		}
		
		logger.Println("")
	}
	
	logger.Info("重启完成: 成功 %d 个，失败 %d 个", successCount, failedCount)
	
	if failedCount > 0 {
		return fmt.Errorf("部分应用重启失败")
	}
	
	return nil
}