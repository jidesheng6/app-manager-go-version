package cmd

import (
	"app-manager-go-v1/internal/services"

	"github.com/spf13/cobra"
)

// backupCmd 备份命令
var backupCmd = &cobra.Command{
	Use:   "backup [prefix] [description]",
	Short: "备份项目文件",
	Long: `备份项目文件到备份目录。

备份内容包括：
- project_backend 目录（应用文件、配置文件等）
- project_frontend 目录（前端文件）
- project_cache 目录（缓存文件）

备份文件将保存为ZIP格式，文件名包含时间戳。
根据配置的保留天数，会自动清理旧的备份文件。

备份过程中会跳过：
- 日志文件（*.log, *.out）
- PID文件（*.pid）
- 临时文件（*.tmp, *.temp）
- 隐藏文件（除配置文件外）
- 大文件（超过100MB）
- 备份目录本身

使用示例：
  app-manager backup                           # 使用默认前缀备份
  app-manager backup "release-v1.0"            # 指定备份前缀
  app-manager backup "release-v1.0" "发布前备份"  # 指定备份前缀和说明`,
	Args: cobra.MaximumNArgs(2),
	PreRun: func(cmd *cobra.Command, args []string) {
		handleError(checkProjectInit(), 1)
	},
	Run: func(cmd *cobra.Command, args []string) {
		backupService := services.NewBackupService()
		
		var backupName, backupDescription string
		
		// 处理位置参数
		if len(args) > 0 {
			backupName = args[0]
		}
		if len(args) > 1 {
			backupDescription = args[1]
		}
		
		if err := backupService.BackupProject(backupName, backupDescription); err != nil {
			handleError(err, 1)
		}
	},
}