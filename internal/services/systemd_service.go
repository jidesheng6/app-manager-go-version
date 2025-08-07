package services

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// SystemdService systemd服务管理
type SystemdService struct {
	config *config.Config
}

// NewSystemdService 创建systemd服务实例
func NewSystemdService() *SystemdService {
	return &SystemdService{
		config: config.AppConfig,
	}
}

// ServiceTemplate systemd服务模板
const ServiceTemplate = `[Unit]
Description={{.Description}}
After=network.target

[Service]
Type=forking
User={{.User}}
Group={{.Group}}
WorkingDirectory={{.WorkingDirectory}}
ExecStart={{.ExecStart}}
ExecStop={{.ExecStop}}
PIDFile={{.PIDFile}}
Restart={{.Restart}}
RestartSec={{.RestartSec}}
TimeoutStartSec={{.TimeoutStartSec}}
TimeoutStopSec={{.TimeoutStopSec}}
StandardOutput=journal
StandardError=journal
{{if .Environment}}{{range .Environment}}Environment={{.}}
{{end}}{{end}}
[Install]
WantedBy=multi-user.target
`

// ServiceConfig 服务配置
type ServiceConfig struct {
	Name             string
	Description      string
	User             string
	Group            string
	WorkingDirectory string
	ExecStart        string
	ExecStop         string
	PIDFile          string
	Restart          string
	RestartSec       string
	TimeoutStartSec  string
	TimeoutStopSec   string
	Environment      []string
}

// CreateService 创建统一的app-manager systemd服务（Linux专用）
func (s *SystemdService) CreateService(serviceName string) error {
	if serviceName == "" {
		return fmt.Errorf("服务名称不能为空")
	}
	
	logger.Title("创建App-Manager Systemd服务")
	logger.Info("服务名称: %s", serviceName)
	
	// 生成统一的服务配置
	serviceConfig := s.generateUnifiedServiceConfig(serviceName)
	
	// 创建服务文件
	servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", serviceName)
	if err := s.writeServiceFile(servicePath, serviceConfig); err != nil {
		return fmt.Errorf("创建服务文件失败: %w", err)
	}
	
	logger.Success("服务文件已创建: %s", servicePath)
	
	// 重新加载systemd配置
	if err := s.reloadSystemd(); err != nil {
		logger.Warning("重新加载systemd配置失败: %v", err)
	}
	
	// 显示后续操作提示
	s.printServiceInstructions(serviceName)
	
	return nil
}

// CreateServiceWithDescription 创建带描述的统一app-manager systemd服务（Linux专用）
func (s *SystemdService) CreateServiceWithDescription(serviceName, description string) error {
	if serviceName == "" {
		return fmt.Errorf("服务名称不能为空")
	}
	
	logger.Title("创建App-Manager Systemd服务")
	logger.Info("服务名称: %s", serviceName)
	if description != "" {
		logger.Info("服务描述: %s", description)
	}
	
	// 生成统一的服务配置
	serviceConfig := s.generateUnifiedServiceConfigWithDescription(serviceName, description)
	
	// 创建服务文件
	servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", serviceName)
	if err := s.writeServiceFile(servicePath, serviceConfig); err != nil {
		return fmt.Errorf("创建服务文件失败: %w", err)
	}
	
	logger.Success("服务文件已创建: %s", servicePath)
	
	// 重新加载systemd配置
	if err := s.reloadSystemd(); err != nil {
		logger.Warning("重新加载systemd配置失败: %v", err)
	}
	
	// 显示后续操作提示
	s.printServiceInstructions(serviceName)
	
	return nil
}

// CreateAllServices 为所有应用创建systemd服务（Linux专用）
func (s *SystemdService) CreateAllServices() error {
	logger.Title("为所有应用创建Systemd服务")
	
	// 获取所有应用
	appService := NewApplicationService()
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	if len(apps) == 0 {
		logger.Info("没有找到任何应用")
		return nil
	}
	
	logger.Info("找到 %d 个应用", len(apps))
	
	successCount := 0
	failedCount := 0
	
	for _, app := range apps {
		logger.Info("为应用创建服务: %s", app.Name)
		
		// 生成服务配置
		serviceConfig := s.generateServiceConfig(app.Name, app.FilePath)
		
		// 创建服务文件
		servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", app.Name)
		if err := s.writeServiceFile(servicePath, serviceConfig); err != nil {
			logger.Error("创建服务文件失败 %s: %v", app.Name, err)
			failedCount++
		} else {
			logger.Success("服务文件已创建: %s", servicePath)
			successCount++
		}
		
		logger.Println("")
	}
	
	// 重新加载systemd配置
	if err := s.reloadSystemd(); err != nil {
		logger.Warning("重新加载systemd配置失败: %v", err)
	}
	
	logger.Info("创建完成: 成功 %d 个，失败 %d 个", successCount, failedCount)
	
	if failedCount > 0 {
		return fmt.Errorf("部分服务创建失败")
	}
	
	return nil
}

// generateUnifiedServiceConfigWithDescription 生成带描述的统一服务配置
func (s *SystemdService) generateUnifiedServiceConfigWithDescription(serviceName, description string) *ServiceConfig {
	pidFile := filepath.Join(config.GetPidsDir(), serviceName+".pid")
	
	// 动态获取当前程序路径
	executablePath, err := os.Executable()
	if err != nil {
		// 如果获取失败，使用默认路径
		projectRoot := config.GetProjectRoot()
		executablePath = filepath.Join(projectRoot, "app-manager-go-v1")
	}
	
	// 构建启动和停止命令
	execStart := fmt.Sprintf("%s start", executablePath)
	execStop := fmt.Sprintf("%s stop", executablePath)
	
	// 获取当前用户
	user := os.Getenv("USER")
	if user == "" {
		user = "root"
	}
	
	// 准备环境变量（只有在启用 Nacos 时才添加）
	var environment []string
	if config.IsNacosEnabled() {
		nacosEnvVars := config.GetNacosEnvironmentVars()
		for key, value := range nacosEnvVars {
			environment = append(environment, fmt.Sprintf("%s=%s", key, value))
		}
	}
	
	return &ServiceConfig{
		Name:             serviceName,
		Description:      description,
		User:             user,
		Group:            user,
		WorkingDirectory: config.GetProjectRoot(),
		ExecStart:        execStart,
		ExecStop:         execStop,
		PIDFile:          pidFile,
		Restart:          s.config.Systemd.ServiceRestart,
		RestartSec:       fmt.Sprintf("%d", s.config.Systemd.ServiceRestartSec),
		TimeoutStartSec:  fmt.Sprintf("%d", s.config.Systemd.ServiceTimeoutStartSec),
		TimeoutStopSec:   fmt.Sprintf("%d", s.config.Systemd.ServiceTimeoutStopSec),
		Environment:      environment,
	}
}

// generateServiceConfigWithDescription 生成带描述的服务配置（保留用于兼容性）
func (s *SystemdService) generateServiceConfigWithDescription(appName, appPath, description string) *ServiceConfig {
	config := s.generateServiceConfig(appName, appPath)
	if description != "" {
		config.Description = description
	}
	return config
}

// generateUnifiedServiceConfig 生成统一的app-manager服务配置
func (s *SystemdService) generateUnifiedServiceConfig(serviceName string) *ServiceConfig {
	pidFile := filepath.Join(config.GetPidsDir(), serviceName+".pid")
	
	// 动态获取当前程序路径
	executablePath, err := os.Executable()
	if err != nil {
		// 如果获取失败，使用默认路径
		projectRoot := config.GetProjectRoot()
		executablePath = filepath.Join(projectRoot, "app-manager-go-v1")
	}
	
	// 构建启动和停止命令
	execStart := fmt.Sprintf("%s start", executablePath)
	execStop := fmt.Sprintf("%s stop", executablePath)
	
	// 获取当前用户
	user := os.Getenv("USER")
	if user == "" {
		user = "root"
	}
	
	// 准备环境变量（只有在启用 Nacos 时才添加）
	var environment []string
	if config.IsNacosEnabled() {
		nacosEnvVars := config.GetNacosEnvironmentVars()
		for key, value := range nacosEnvVars {
			environment = append(environment, fmt.Sprintf("%s=%s", key, value))
		}
	}
	
	return &ServiceConfig{
		Name:             serviceName,
		Description:      fmt.Sprintf("App Manager Service - %s", serviceName),
		User:             user,
		Group:            user,
		WorkingDirectory: config.GetProjectRoot(),
		ExecStart:        execStart,
		ExecStop:         execStop,
		PIDFile:          pidFile,
		Restart:          s.config.Systemd.ServiceRestart,
		RestartSec:       fmt.Sprintf("%d", s.config.Systemd.ServiceRestartSec),
		TimeoutStartSec:  fmt.Sprintf("%d", s.config.Systemd.ServiceTimeoutStartSec),
		TimeoutStopSec:   fmt.Sprintf("%d", s.config.Systemd.ServiceTimeoutStopSec),
		Environment:      environment,
	}
}

// generateServiceConfig 生成服务配置（保留用于兼容性）
func (s *SystemdService) generateServiceConfig(appName, appPath string) *ServiceConfig {
	backendDir := config.GetBackendDir()
	pidFile := filepath.Join(config.GetPidsDir(), appName+".pid")
	
	// 构建启动命令
	javaPath := s.config.Global.JavaPath
	if javaPath == "" {
		javaPath = "java"
	}
	
	// 获取优化后的JVM参数（包含随机数优化）
	jvmParams := config.GetOptimizedJVMParams()
	if jvmParams == "" {
		jvmParams = "-Xms512m -Xmx1024m -Djava.security.egd=file:/dev/./urandom"
	}
	
	execStart := fmt.Sprintf("%s %s -jar %s", javaPath, jvmParams, appPath)
	execStop := fmt.Sprintf("/bin/kill -TERM $MAINPID")
	
	// 获取当前用户
	user := os.Getenv("USER")
	if user == "" {
		user = "root"
	}
	
	// 准备环境变量
	var environment []string
	if config.IsNacosEnabled() {
		nacosEnvVars := config.GetNacosEnvironmentVars()
		for key, value := range nacosEnvVars {
			environment = append(environment, fmt.Sprintf("%s=%s", key, value))
		}
	}
	
	return &ServiceConfig{
		Name:             appName,
		Description:      fmt.Sprintf("Java Application - %s", appName),
		User:             user,
		Group:            user,
		WorkingDirectory: backendDir,
		ExecStart:        execStart,
		ExecStop:         execStop,
		PIDFile:          pidFile,
		Restart:          s.config.Systemd.ServiceRestart,
		RestartSec:       fmt.Sprintf("%d", s.config.Systemd.ServiceRestartSec),
		TimeoutStartSec:  fmt.Sprintf("%d", s.config.Systemd.ServiceTimeoutStartSec),
		TimeoutStopSec:   fmt.Sprintf("%d", s.config.Systemd.ServiceTimeoutStopSec),
		Environment:      environment,
	}
}

// writeServiceFile 写入服务文件
func (s *SystemdService) writeServiceFile(servicePath string, config *ServiceConfig) error {
	// 解析模板
	tmpl, err := template.New("service").Parse(ServiceTemplate)
	if err != nil {
		return fmt.Errorf("解析服务模板失败: %w", err)
	}
	
	// 创建服务文件
	file, err := os.Create(servicePath)
	if err != nil {
		return fmt.Errorf("创建服务文件失败: %w", err)
	}
	defer file.Close()
	
	// 执行模板
	if err := tmpl.Execute(file, config); err != nil {
		return fmt.Errorf("生成服务文件失败: %w", err)
	}
	
	// 设置文件权限
	if err := os.Chmod(servicePath, 0644); err != nil {
		return fmt.Errorf("设置服务文件权限失败: %w", err)
	}
	
	return nil
}

// reloadSystemd 重新加载systemd配置
func (s *SystemdService) reloadSystemd() error {
	cmd := exec.Command("systemctl", "daemon-reload")
	return cmd.Run()
}

// printServiceInstructions 打印服务操作说明
func (s *SystemdService) printServiceInstructions(serviceName string) {
	logger.Println("")
	logger.Success("Systemd服务创建成功！")
	logger.Println("")
	
	// 重要提示
	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Printf("\033[31m\033[1m  ⚠️  重要提示 ⚠️\033[0m\n")
	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Println("")
	logger.Warning("服务创建后，请务必注意以下事项：")
	logger.Println("")
	logger.Printf("\033[33m\033[1m  🚫 程序文件不可删除\033[0m\n")
	// 动态获取当前程序路径
	executablePath, err := os.Executable()
	if err != nil {
		// 如果获取失败，使用默认路径
		projectRoot := config.GetProjectRoot()
		executablePath = filepath.Join(projectRoot, "app-manager-go-v1")
	}
	logger.Printf("     服务依赖的程序文件：\033[37m%s\033[0m\n", executablePath)
	logger.Printf("     删除此文件将导致systemd服务无法正常工作！\n")
	logger.Println("")
	logger.Printf("\033[33m\033[1m  📁 项目目录不可移动\033[0m\n")
	logger.Printf("     项目根目录：\033[37m%s\033[0m\n", config.GetProjectRoot())
	logger.Printf("     移动此目录将导致服务路径失效！\n")
	logger.Println("")
	
	logger.Info("Systemd服务操作说明：")
	logger.Println("")
	
	commands := []struct {
		desc string
		cmd  string
	}{
		{"启用服务（开机自启）", fmt.Sprintf("systemctl enable %s", serviceName)},
		{"禁用服务", fmt.Sprintf("systemctl disable %s", serviceName)},
		{"启动服务", fmt.Sprintf("systemctl start %s", serviceName)},
		{"停止服务", fmt.Sprintf("systemctl stop %s", serviceName)},
		{"重启服务", fmt.Sprintf("systemctl restart %s", serviceName)},
		{"查看服务状态", fmt.Sprintf("systemctl status %s", serviceName)},
		{"查看服务日志", fmt.Sprintf("journalctl -u %s -f", serviceName)},
		{"删除服务", fmt.Sprintf("systemctl stop %s && systemctl disable %s && rm /etc/systemd/system/%s.service && systemctl daemon-reload", serviceName, serviceName, serviceName)},
	}
	
	for _, cmd := range commands {
		logger.Printf("  %-20s: \033[37m%s\033[0m\n", cmd.desc, cmd.cmd)
	}
	
	logger.Println("")
	logger.Warning("注意：systemd服务操作需要root权限")
}

// RemoveService 删除systemd服务（Linux专用）
func (s *SystemdService) RemoveService(appName string) error {
	logger.Title("删除Systemd服务")
	logger.Info("应用名称: %s", appName)
	
	servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", appName)
	
	// 检查服务文件是否存在
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		logger.Warning("服务文件不存在: %s", servicePath)
		return nil
	}
	
	// 停止服务
	logger.Info("停止服务...")
	cmd := exec.Command("systemctl", "stop", appName)
	if err := cmd.Run(); err != nil {
		logger.Warning("停止服务失败: %v", err)
	}
	
	// 禁用服务
	logger.Info("禁用服务...")
	cmd = exec.Command("systemctl", "disable", appName)
	if err := cmd.Run(); err != nil {
		logger.Warning("禁用服务失败: %v", err)
	}
	
	// 删除服务文件
	logger.Info("删除服务文件...")
	if err := os.Remove(servicePath); err != nil {
		return fmt.Errorf("删除服务文件失败: %w", err)
	}
	
	// 重新加载systemd配置
	if err := s.reloadSystemd(); err != nil {
		logger.Warning("重新加载systemd配置失败: %v", err)
	}
	
	logger.Success("服务已删除: %s", appName)
	return nil
}

// ListServices 列出所有相关的systemd服务（Linux专用）
func (s *SystemdService) ListServices() error {
	logger.Title("Systemd服务列表")
	
	// 获取所有应用
	appService := NewApplicationService()
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	if len(apps) == 0 {
		logger.Info("没有找到任何应用")
		return nil
	}
	
	for _, app := range apps {
		servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", app.Name)
		
		logger.Printf("应用: \033[33m%s\033[0m\n", app.Name)
		
		if _, err := os.Stat(servicePath); os.IsNotExist(err) {
			logger.Printf("  服务状态: \033[31m未创建\033[0m\n")
		} else {
			// 获取服务状态
			status := s.getServiceStatus(app.Name)
			logger.Printf("  服务文件: \033[32m%s\033[0m\n", servicePath)
			logger.Printf("  服务状态: %s\n", status)
		}
		logger.Println("")
	}
	
	return nil
}

// getServiceStatus 获取服务状态
func (s *SystemdService) getServiceStatus(serviceName string) string {
	cmd := exec.Command("systemctl", "is-active", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return "\033[31m未知\033[0m"
	}
	
	status := strings.TrimSpace(string(output))
	switch status {
	case "active":
		return "\033[32m运行中\033[0m"
	case "inactive":
		return "\033[33m已停止\033[0m"
	case "failed":
		return "\033[31m失败\033[0m"
	default:
		return fmt.Sprintf("\033[37m%s\033[0m", status)
	}
}

// EnableService 启用服务（Linux专用）
func (s *SystemdService) EnableService(appName string) error {
	logger.Info("启用服务: %s", appName)
	
	cmd := exec.Command("systemctl", "enable", appName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("启用服务失败: %w", err)
	}
	
	logger.Success("服务已启用: %s", appName)
	return nil
}

// DisableService 禁用服务（Linux专用）
func (s *SystemdService) DisableService(appName string) error {
	logger.Info("禁用服务: %s", appName)
	
	cmd := exec.Command("systemctl", "disable", appName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("禁用服务失败: %w", err)
	}
	
	logger.Success("服务已禁用: %s", appName)
	return nil
}