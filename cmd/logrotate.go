package cmd

import (
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/services"
	"os"

	"github.com/spf13/cobra"
)

// logrotateCmd 日志轮转命令
var logrotateCmd = &cobra.Command{
	Use:   "logrotate <config_name>",
	Short: "创建统一的日志轮转配置",
	Long: `创建统一的logrotate配置文件来管理所有应用的日志轮转。

此命令将创建一个logrotate配置文件，统一管理项目中所有应用的日志文件。

注意：
- 配置名称是必需的参数
- 需要root权限来创建logrotate配置文件
- 配置文件将创建在 /etc/logrotate.d/ 目录下

使用示例：
  app-manager logrotate my-app-logs                    # 创建名为my-app-logs的轮转配置
  app-manager logrotate my-app-logs "应用日志轮转配置"   # 创建带描述的轮转配置`,
	Args: cobra.RangeArgs(1, 2),
	PreRun: func(cmd *cobra.Command, args []string) {
		handleError(checkProjectInit(), 1)
	},
	Run: func(cmd *cobra.Command, args []string) {
		logService := services.NewLogService()
		
		configName := args[0]
		description := ""
		if len(args) > 1 {
			description = args[1]
		}
		
		if err := logService.CreateLogrotateConfig(configName, description); err != nil {
			logger.Error("创建日志轮转配置失败: %v", err)
			os.Exit(1)
		}
	},
}