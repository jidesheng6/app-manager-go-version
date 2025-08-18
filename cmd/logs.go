package cmd

import (
	"app-manager-go-v1/internal/services"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	logLines  int
	logFollow bool
	logList   bool
)

// logsCmd 日志命令
var logsCmd = &cobra.Command{
	Use:   "logs [application_name]",
	Short: "查看应用程序日志",
	Long: `查看指定应用程序的日志文件。

如果不指定应用名称且使用 --list 标志，将列出所有应用的日志文件。
支持实时跟踪日志文件变化。

日志显示特性：
- 根据日志级别自动着色（ERROR/WARN/INFO/DEBUG）
- 支持指定显示行数
- 支持实时跟踪模式

使用示例：
  app-manager logs myapp              # 查看应用日志（默认最后50行）
  app-manager logs myapp -n 100       # 查看最后100行日志
  app-manager logs myapp -f           # 实时跟踪日志
  app-manager logs myapp -n 20 -f     # 显示最后20行并实时跟踪
  app-manager logs --list             # 列出所有应用的日志文件`,
	Args: func(cmd *cobra.Command, args []string) error {
		if logList && len(args) > 0 {
			return fmt.Errorf("不能同时指定应用名称和 --list 标志")
		}
		if !logList && len(args) != 1 {
			return fmt.Errorf("请指定应用名称或使用 --list 标志")
		}
		return nil
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		handleError(checkProjectInit(), 1)
	},
	Run: func(cmd *cobra.Command, args []string) {
		logService := services.NewLogService()

		if logList {
			// 列出所有日志文件
			if err := logService.ListLogs(); err != nil {
				handleError(err, 1)
			}
		} else {
			// 查看指定应用日志
			appName := args[0]

			// 智能匹配应用名称
			appService := services.NewApplicationService()
			apps, err := appService.DiscoverApplications()
			if err != nil {
				handleError(fmt.Errorf("发现应用失败: %w", err), 1)
			}

			// 查找应用（支持交互式选择）
			app := findApplicationWithInteractiveSelection(apps, appName)
			if app == nil {
				handleError(fmt.Errorf("未找到应用: %s", appName), 1)
			}
			appName = app.Name

			if err := logService.ViewLog(appName, logLines, logFollow); err != nil {
				handleError(err, 1)
			}
		}
	},
}

func init() {
	logsCmd.Flags().IntVarP(&logLines, "lines", "n", 800, "显示的日志行数")
	logsCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "实时跟踪日志文件")
	logsCmd.Flags().BoolVarP(&logList, "list", "l", false, "列出所有应用的日志文件")
}
