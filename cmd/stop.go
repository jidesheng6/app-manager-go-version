package cmd

import (
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/services"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	stopAll   bool
	forceStop bool
)

// stopCmd 停止命令
var stopCmd = &cobra.Command{
	Use:   "stop [application_name]",
	Short: "停止Java应用程序",
	Long: `停止指定的Java应用程序或所有应用程序。

应用程序名称可以是完整的文件名（如 myapp.jar）或者应用名称（如 myapp）。
如果不指定应用名称，将停止所有正在运行的应用程序。
如果使用 --force 标志，将强制终止应用程序（不推荐）。

使用示例：
  app-manager stop                    # 停止所有正在运行的应用
  app-manager stop myapp              # 停止名为 myapp 的应用
  app-manager stop myapp.jar          # 停止 myapp.jar 应用
  app-manager stop --all              # 停止所有正在运行的应用（与不传参数效果相同）
  app-manager stop myapp --force      # 强制停止应用`,
	Args: func(cmd *cobra.Command, args []string) error {
		if stopAll && len(args) > 0 {
			return fmt.Errorf("不能同时指定应用名称和 --all 标志")
		}
		return nil
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		handleError(checkProjectInit(), 1)
	},
	Run: func(cmd *cobra.Command, args []string) {
		appService := services.NewApplicationService()
		
		if stopAll || len(args) == 0 {
			// 停止所有应用
			if err := stopAllApplications(appService); err != nil {
				handleError(err, 1)
			}
		} else {
			// 停止指定应用
			appName := args[0]
			if err := stopSingleApplication(appService, appName); err != nil {
				handleError(err, 1)
			}
		}
	},
}

func init() {
	stopCmd.Flags().BoolVarP(&stopAll, "all", "a", false, "停止所有正在运行的应用")
	stopCmd.Flags().BoolVarP(&forceStop, "force", "f", false, "强制停止应用（不推荐）")
}

// stopSingleApplication 停止单个应用
func stopSingleApplication(appService *services.ApplicationService, appName string) error {
	logger.Title(fmt.Sprintf("停止应用: %s", appName))
	
	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	// 查找指定应用（支持交互式选择）
	app := findApplicationWithInteractiveSelection(apps, appName)
	if app == nil {
		return fmt.Errorf("未找到应用: %s", appName)
	}
	
	// 更新应用状态
	app.UpdateStatus()
	
	// 检查应用状态
	if !app.IsRunning() {
		logger.Warning(fmt.Sprintf("应用 '%s' 当前状态为: %s", appName, app.Status))
		return nil
	}
	
	logger.Info("停止应用: %s (PID: %d)", app.Name, app.PID)
	
	if forceStop {
		logger.Warning("使用强制停止模式")
	}
	
	// 停止应用
	if err := appService.StopApplication(app); err != nil {
		return fmt.Errorf("停止应用失败: %w", err)
	}
	
	logger.Success("应用 %s 已停止", app.Name)
	return nil
}

// stopAllApplications 停止所有应用
func stopAllApplications(appService *services.ApplicationService) error {
	logger.Title("停止所有应用")
	
	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	if len(apps) == 0 {
		logger.Info("没有找到任何应用")
		return nil
	}
	
	// 更新所有应用的状态
	for _, app := range apps {
		app.UpdateStatus()
	}
	
	// 获取正在运行的应用
	runningApps := apps.Running()
	if len(runningApps) == 0 {
		logger.Info("没有正在运行的应用")
		return nil
	}
	
	logger.Info("找到 %d 个正在运行的应用", len(runningApps))
	
	if forceStop {
		logger.Warning("使用强制停止模式")
	}
	
	successCount := 0
	failedCount := 0
	
	for _, app := range runningApps {
		logger.Info("停止应用: %s (PID: %d)", app.Name, app.PID)
		
		if err := appService.StopApplication(app); err != nil {
			logger.Error("停止应用失败 %s: %v", app.Name, err)
			failedCount++
		} else {
			logger.Success("应用 %s 已停止", app.Name)
			successCount++
		}
	}
	
	logger.Println("")
	logger.Info("停止完成: 成功 %d 个，失败 %d 个", successCount, failedCount)
	
	if failedCount > 0 {
		return fmt.Errorf("部分应用停止失败")
	}
	
	return nil
}