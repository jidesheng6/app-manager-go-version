package cmd

import (
	"app-manager-go-v1/internal/services"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	logLines   int
	logFollow  bool
	logList    bool
	logHistory bool
	logTail    int
)

// logsCmd 日志命令
var logsCmd = &cobra.Command{
	Use:   "logs [application_name]",
	Short: "查看应用程序日志",
	Long: `查看指定应用程序的日志文件。

日志留痕说明：
  每次 start 都会新建 <应用名>-yyyy-MM-dd-HHmmss.log，stop/restart 时把当前日志
  改名为 <应用名>-yyyy-MM-dd-HHmmss-停止.log 归档，<应用名>.log 是指向当前日志的软链接。
  因此历史日志不会被覆盖，可通过 --history / --tail 回看。

如果不指定应用名称且使用 --list 标志，将列出所有应用的日志文件。
支持实时跟踪日志文件变化。

日志显示特性：
- 根据日志级别自动着色（ERROR/WARN/INFO/DEBUG）
- 支持指定显示行数
- 支持实时跟踪模式
- 支持查看历史运行周期的日志

使用示例：
  app-manager logs myapp              # 查看应用当前日志（默认最后800行）
  app-manager logs myapp -n 100       # 查看最后100行日志
  app-manager logs myapp -f           # 实时跟踪日志
  app-manager logs myapp -n 20 -f     # 显示最后20行并实时跟踪
  app-manager logs myapp --history    # 列出该应用所有运行周期的日志
  app-manager logs myapp --tail 2     # 查看倒数第2次运行的日志（1=最新）
  app-manager logs myapp --tail 2 -f  # 实时跟踪倒数第2次运行的日志
  app-manager logs --list             # 列出所有应用的日志文件`,
	Args: func(cmd *cobra.Command, args []string) error {
		if logList && len(args) > 0 {
			return fmt.Errorf("不能同时指定应用名称和 --list 标志")
		}
		if logHistory && logList {
			return fmt.Errorf("不能同时指定 --history 和 --list 标志")
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
			return
		}

		// 查看指定应用日志
		appName := args[0]

		// 智能匹配应用名称
		appService := services.NewApplicationService()
		apps, err := appService.DiscoverApplications()
		if err != nil {
			handleError(fmt.Errorf("发现应用失败: %w", err), 1)
		}

		// 首先尝试精确匹配
		if app := apps.FindByName(appName); app != nil {
			appName = app.Name
		} else {
			// 如果精确匹配失败，尝试模糊匹配
			matches := apps.FindByPattern(appName)
			if len(matches) == 0 {
				handleError(fmt.Errorf("未找到匹配的应用: %s", appName), 1)
			} else if len(matches) == 1 {
				appName = matches[0].Name
				fmt.Printf("智能匹配到应用: %s\n", appName)
			} else {
				// 多个匹配结果，显示候选列表
				fmt.Printf("找到多个匹配的应用:\n")
				for i, app := range matches {
					fmt.Printf("  %d. %s\n", i+1, app.Name)
				}
				handleError(fmt.Errorf("请使用更具体的应用名称"), 1)
			}
		}

		// 列出该应用的日志留痕记录
		if logHistory {
			if err := logService.ListAppSessions(appName); err != nil {
				handleError(err, 1)
			}
			return
		}

		// 查看指定序号的运行周期日志
		if logTail > 0 {
			if err := logService.TailLogSession(appName, logTail, logLines, logFollow); err != nil {
				handleError(err, 1)
			}
			return
		}

		if err := logService.ViewLog(appName, logLines, logFollow); err != nil {
			handleError(err, 1)
		}
	},
}

func init() {
	logsCmd.Flags().IntVarP(&logLines, "lines", "n", 800, "显示的日志行数")
	logsCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "实时跟踪日志文件")
	logsCmd.Flags().BoolVarP(&logList, "list", "l", false, "列出所有应用的日志文件")
	logsCmd.Flags().BoolVar(&logHistory, "history", false, "列出该应用所有运行周期的日志（留痕记录）")
	logsCmd.Flags().IntVar(&logTail, "tail", 0, "查看指定序号的运行周期日志（1=最新，序号见 --history）")
}
