package cmd

import (
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/services"
	"fmt"

	"github.com/spf13/cobra"
)

// serviceCmd systemd服务命令
var serviceCmd = &cobra.Command{
	Use:   "service <service_name> [description]",
	Short: "创建统一的app-manager systemd服务",
	Long: `创建统一的app-manager systemd服务文件。

此服务将通过app-manager.sh脚本来管理所有应用的启停。

⚠️  重要提示：
- 服务创建后，程序文件和项目目录不可删除或移动
- 删除或移动将导致systemd服务无法正常工作
- 如需删除服务，请先使用相应的systemctl命令

注意：
- 此功能仅在Linux系统上可用
- 需要root权限来创建服务文件
- 服务文件将创建在 /etc/systemd/system/ 目录下
- 服务名称是必需的参数
- 创建服务前会自动检查并要求停止所有运行中的应用

使用示例：
  app-manager service my-app-manager                    # 创建名为my-app-manager的服务
  app-manager service my-app-manager "我的应用管理服务"   # 创建带描述的服务`,
	Args: cobra.RangeArgs(1, 2),
	PreRun: func(cmd *cobra.Command, args []string) {
		handleError(checkProjectInit(), 1)
	},
	Run: func(cmd *cobra.Command, args []string) {
		// 检查是否有正在运行的应用
		if err := checkRunningApplications(); err != nil {
			handleError(err, 1)
		}
		
		systemdService := services.NewSystemdService()
		
		serviceName := args[0]
		description := ""
		if len(args) > 1 {
			description = args[1]
		}
		
		// 创建服务
		var err error
		if description != "" {
			err = systemdService.CreateServiceWithDescription(serviceName, description)
		} else {
			err = systemdService.CreateService(serviceName)
		}
		
		if err != nil {
			handleError(err, 1)
		}
	},
}

// checkRunningApplications 检查是否有正在运行的应用
func checkRunningApplications() error {
	appService := services.NewApplicationService()
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	runningApps := apps.Running()
	if len(runningApps) > 0 {
		logger.Warning("检测到以下应用正在运行：")
		for _, app := range runningApps {
			logger.Printf("  • %s (PID: %d)", app.Name, app.PID)
		}
		logger.Println("")
		logger.Warning("创建systemd服务前，请先停止所有运行中的应用：")
		logger.Printf("  app-manager-go-v1 stop\n")
		logger.Println("")
		logger.Info("这样可以避免与systemd服务管理产生冲突")
		return fmt.Errorf("请先停止所有运行中的应用，然后重新创建服务")
	}
	
	return nil
}