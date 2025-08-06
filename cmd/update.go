package cmd

import (
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/services"

	"github.com/spf13/cobra"
)

// updateCmd 更新命令
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "准备应用更新",
	Long: `准备应用更新操作。

更新准备过程：
1. 检查是否有正在运行的应用，如有则提示用户手动停止
2. 创建更新前备份
3. 创建更新目录结构
4. 提供详细的更新流程指导

更新准备完成后，用户需要：
1. 将新的应用文件复制到 project_backend 目录
2. 更新配置文件（如果需要）
3. 使用 start 命令启动应用

注意事项：
- 更新前会自动创建备份，确保可以回滚
- 如有运行中的应用，需要先手动停止：app-manager-go-v1 stop
- 建议在维护窗口期间执行更新操作

使用示例：
  app-manager update                  # 准备更新操作`,
	PreRun: func(cmd *cobra.Command, args []string) {
		handleError(checkProjectInit(), 1)
	},
	Run: func(cmd *cobra.Command, args []string) {
		projectService := services.NewProjectService()
		
		if err := projectService.UpdatePrepare(); err != nil {
			handleError(err, 1)
		}
		
		logger.Success("更新准备完成！")
	},
}