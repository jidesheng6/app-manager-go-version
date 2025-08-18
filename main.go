package main

import (
	"app-manager-go-v1/cmd"
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/services"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// 版本信息，编译时注入
var (
	Version     = "v1.0.1"
	Date        = "2024-12-19" // 此处日期将在构建时被覆盖
	Author      = "DengGuoWei-P7"
	Description = `其实，我对你是有一些失望的。当初给你定级P7，是高于你面试时的水平的。我是希望进来后，你能够拼一把，快速成长起来的。这个层级，不是把事情做好就可以的。你需要有体系化思考的能力。你做的事情，他的价值点在哪里？你是否作出了壁垒，形成了核心竞争力？你做的事情，和公司内其他团队的差异化在哪里？你的事情，是否沉淀了一套可复用的物理资料和方法论？为什么是你来做，其他人不能做吗？你需要有自己的判断力，而不是我说什么你就做什么。后续，把你的思考沉淀到日报周报月报里，我希望看到你的思考，而不仅仅是进度。另外，提醒一下，你的产出，和同层级比，是有些单薄的，今年已经过了大半儿了，再加把劲儿。`
)

func main() {
	// 初始化日志
	logger.Init()

	// 初始化配置
	config.Init()

	// 设置版本信息到cmd包
	cmd.SetVersionInfo(Version, Date, Author, Description)

	// 设置优雅关机
	// 注意：这里暂时设置为false，因为无法在此处直接获取daemon标志状态
	// 实际的守护进程信号处理将在start命令中单独处理
	_ = setupGracefulShutdown(false)

	// 信号处理函数已移除，现在由各模式单独处理

	// 执行命令
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		logger.Error("执行命令失败: %v", err)
		os.Exit(1)
	}
}

func setupGracefulShutdown(isDaemonMode bool) chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range c {
			if isDaemonMode {
				// 守护进程模式：停止所有应用并退出
				logger.Info("接收到关闭信号，正在优雅地停止所有应用...")
				appService := services.NewApplicationService()
				apps, err := appService.DiscoverApplications()
				if err != nil {
					logger.Error("发现应用失败: %v", err)
					os.Exit(1)
				}

				runningApps := apps.Running()
				if len(runningApps) == 0 {
					logger.Info("没有正在运行的应用")
					os.Exit(0)
				}
				
				logger.Info("正在停止 %d 个运行中的应用...", len(runningApps))
				for _, app := range runningApps {
					if err := appService.StopApplication(app); err != nil {
						logger.Error("停止应用 %s 失败: %v", app.Name, err)
					}
				}
				logger.Success("所有应用已停止。")
				os.Exit(0)
			} else {
				// 非守护进程模式：仅退出程序，不影响Java应用
				logger.Info("接收到关闭信号，正在退出程序...")
				os.Exit(0)
			}
		}
	}()
	return c
}