package cmd

import (
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/models"
	"bufio"
	"os"
	"strconv"
	"strings"
)

// selectApplicationFromMatches 从多个匹配的应用中让用户选择一个
// 返回选中的应用，如果用户取消则返回nil
func selectApplicationFromMatches(matches models.ApplicationList, originalName string) *models.Application {
	if len(matches) == 0 {
		return nil
	}
	
	if len(matches) == 1 {
		return matches[0]
	}
	
	// 显示匹配的应用列表
	logger.Warning("找到多个匹配 '%s' 的应用:", originalName)
	for i, app := range matches {
		logger.Info("  %d. %s", i+1, app.Name)
	}
	logger.Info("  0. 取消操作")
	logger.Println("")
	
	// 获取用户输入
	reader := bufio.NewReader(os.Stdin)
	for {
		logger.Printf("请选择要操作的应用 (0-%d): ", len(matches))
		input, err := reader.ReadString('\n')
		if err != nil {
			logger.Error("读取输入失败: %v", err)
			return nil
		}
		
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		
		// 解析用户选择
		choice, err := strconv.Atoi(input)
		if err != nil {
			logger.Error("请输入有效的数字")
			continue
		}
		
		// 检查选择范围
		if choice == 0 {
			logger.Info("操作已取消")
			return nil
		}
		
		if choice < 1 || choice > len(matches) {
			logger.Error("选择超出范围，请输入 0-%d 之间的数字", len(matches))
			continue
		}
		
		// 返回选中的应用
		selectedApp := matches[choice-1]
		logger.Success("已选择应用: %s", selectedApp.Name)
		return selectedApp
	}
}

// findApplicationWithInteractiveSelection 查找应用，支持交互式选择
// 首先尝试精确匹配，如果失败则尝试模糊匹配并提供交互式选择
func findApplicationWithInteractiveSelection(apps models.ApplicationList, appName string) *models.Application {
	// 首先尝试精确匹配
	if app := apps.FindByName(appName); app != nil {
		return app
	}
	
	// 尝试模糊匹配
	matchedApps := apps.FindByPattern(appName)
	if len(matchedApps) == 0 {
		return nil
	}
	
	// 如果只有一个匹配，直接返回
	if len(matchedApps) == 1 {
		logger.Info("智能匹配到应用: %s", matchedApps[0].Name)
		return matchedApps[0]
	}
	
	// 多个匹配，让用户选择
	return selectApplicationFromMatches(matchedApps, appName)
}