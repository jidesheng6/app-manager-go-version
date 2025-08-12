package cmd

import (
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/services"
	"path/filepath"

	"github.com/spf13/cobra"
)

// initCmd 初始化命令
var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "初始化项目目录结构",
	Long: `初始化项目目录结构和配置文件。

如果不指定路径，将在当前目录创建项目结构。
如果指定路径，将在指定路径创建项目结构。

创建的目录结构：
  project_root/
  ├── project_backend/          # 后端应用目录
  │   ├── config/              # 配置文件目录
  │   ├── logs/                # 日志文件目录
  │   ├── pids/                # PID文件目录
  │   ├── file_upload/         # 文件上传目录
  │   └── rotate_logs/         # 轮转日志目录
  ├── project_frontend/         # 前端文件目录
  ├── project_backup/          # 备份文件目录
  ├── project_update/          # 更新文件目录
  ├── project_cache/           # 缓存目录
  └── .app-manager.yaml        # 配置文件

使用示例：
  app-manager init                    # 在当前目录初始化
  app-manager init /opt/myproject     # 在指定目录初始化`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var projectPath string
		
		if len(args) > 0 {
			projectPath = args[0]
		} else {
			// 使用当前目录
			projectPath = "."
		}
		
		// 转换为绝对路径
		absPath, err := filepath.Abs(projectPath)
		if err != nil {
			handleError(err, 1)
		}
		
		logger.Title("项目初始化")
		logger.Info("初始化路径: %s", absPath)
		
		// 创建项目服务（使用默认配置，不受现有配置文件影响）
		projectService := services.NewProjectServiceWithDefaults()
		
		// 执行初始化
		if err := projectService.InitProject(absPath); err != nil {
			handleError(err, 1)
		}
		
		logger.Success("项目初始化完成！")
	},
}