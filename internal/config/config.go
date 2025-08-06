package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用配置结构
type Config struct {
	Global      GlobalConfig      `mapstructure:"global" ini:"global"`
	Nacos       NacosConfig       `mapstructure:"nacos" ini:"nacos"`
	Application ApplicationConfig `mapstructure:"application" ini:"application"`
	Backup      BackupConfig      `mapstructure:"backup" ini:"backup"`
	Logrotate   LogrotateConfig   `mapstructure:"logrotate" ini:"logrotate"`
	Systemd     SystemdConfig     `mapstructure:"systemd" ini:"systemd"`
}

// GlobalConfig 全局配置
type GlobalConfig struct {
	ProjectRoot string `mapstructure:"project_root" ini:"project_root"`
	JavaPath    string `mapstructure:"java_path" ini:"java_path"`
}

// NacosConfig Nacos配置
type NacosConfig struct {
	NacosServer    string `mapstructure:"nacos_server" ini:"nacos_server"`
	NacosNamespace string `mapstructure:"nacos_namespace" ini:"nacos_namespace"`
}

// ApplicationConfig 应用配置
type ApplicationConfig struct {
	AppType   string `mapstructure:"app_type" ini:"app_type"`
	JVMParams string `mapstructure:"jvm_params" ini:"jvm_params"`
}

// BackupConfig 备份配置
type BackupConfig struct {
	BackupPrefix        string `mapstructure:"backup_prefix" ini:"backup_prefix"`
	BackupRetentionDays int    `mapstructure:"backup_retention_days" ini:"backup_retention_days"`
}

// LogrotateConfig 日志轮转配置
type LogrotateConfig struct {
	LogrotateSize   string `mapstructure:"logrotate_size" ini:"logrotate_size"`
	LogrotateCycle  string `mapstructure:"logrotate_cycle" ini:"logrotate_cycle"`
	LogrotateRotate int    `mapstructure:"logrotate_rotate" ini:"logrotate_rotate"`
}

// SystemdConfig systemd配置
type SystemdConfig struct {
	ServiceRestart         string `mapstructure:"service_restart" ini:"service_restart"`
	ServiceRestartSec      int    `mapstructure:"service_restart_sec" ini:"service_restart_sec"`
	ServiceTimeoutStartSec int    `mapstructure:"service_timeout_start_sec" ini:"service_timeout_start_sec"`
	ServiceTimeoutStopSec  int    `mapstructure:"service_timeout_stop_sec" ini:"service_timeout_stop_sec"`
}

var (
	// AppConfig 全局配置实例
	AppConfig *Config
	// ConfigFile 配置文件路径
	ConfigFile string
)

// Init 初始化配置
func Init() {
	AppConfig = &Config{
		Global: GlobalConfig{
			JavaPath: "java",
		},
		Nacos: NacosConfig{
			NacosServer:    "",
			NacosNamespace: "",
		},
		Application: ApplicationConfig{
			AppType:   "mixed",
			JVMParams: "",
		},
		Backup: BackupConfig{
			BackupPrefix:        "Application_Backup",
			BackupRetentionDays: 30,
		},
		Logrotate: LogrotateConfig{
			LogrotateSize:   "30m",
			LogrotateCycle:  "weekly",
			LogrotateRotate: 30,
		},
		Systemd: SystemdConfig{
			ServiceRestart:         "always",
			ServiceRestartSec:      10,
			ServiceTimeoutStartSec: 0,
			ServiceTimeoutStopSec:  0,
		},
	}
}

// LoadConfig 加载配置文件
func LoadConfig(configPath string) error {
	if configPath == "" {
		// 自动查找配置文件
		configPath = findConfigFile()
	}

	if configPath == "" {
		return fmt.Errorf("未找到配置文件，请先运行 init 命令初始化项目")
	}

	ConfigFile = configPath

	viper.SetConfigFile(configPath)
	viper.SetConfigType("ini")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := viper.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 清理配置值中的引号
	cleanConfigValues()

	return nil
}

// cleanConfigValues 清理配置值中的引号
func cleanConfigValues() {
	// 清理全局配置
	AppConfig.Global.ProjectRoot = cleanQuotes(AppConfig.Global.ProjectRoot)
	AppConfig.Global.JavaPath = cleanQuotes(AppConfig.Global.JavaPath)
	
	// 清理Nacos配置
	AppConfig.Nacos.NacosServer = cleanQuotes(AppConfig.Nacos.NacosServer)
	AppConfig.Nacos.NacosNamespace = cleanQuotes(AppConfig.Nacos.NacosNamespace)
	
	// 清理应用配置
	AppConfig.Application.JVMParams = cleanQuotes(AppConfig.Application.JVMParams)
	AppConfig.Application.AppType = cleanQuotes(AppConfig.Application.AppType)
	
	// 清理备份配置
	AppConfig.Backup.BackupPrefix = cleanQuotes(AppConfig.Backup.BackupPrefix)
	
	// 清理日志轮转配置
	AppConfig.Logrotate.LogrotateSize = cleanQuotes(AppConfig.Logrotate.LogrotateSize)
	AppConfig.Logrotate.LogrotateCycle = cleanQuotes(AppConfig.Logrotate.LogrotateCycle)
	
	// 清理systemd配置
	AppConfig.Systemd.ServiceRestart = cleanQuotes(AppConfig.Systemd.ServiceRestart)
}

// cleanQuotes 清理字符串中的引号
func cleanQuotes(value string) string {
	if value == "" {
		return value
	}
	
	// 去除首尾空格
	value = strings.TrimSpace(value)
	
	// 如果字符串被双引号包围，则移除
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1]
	}
	
	// 如果字符串被单引号包围，则移除
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	
	return value
}

// SaveConfig 保存配置文件
func SaveConfig(configPath string) error {
	viper.Set("global", AppConfig.Global)
	viper.Set("nacos", AppConfig.Nacos)
	viper.Set("application", AppConfig.Application)
	viper.Set("backup", AppConfig.Backup)
	viper.Set("logrotate", AppConfig.Logrotate)
	viper.Set("systemd", AppConfig.Systemd)

	return viper.WriteConfigAs(configPath)
}

// findConfigFile 查找配置文件
func findConfigFile() string {
	// 获取当前工作目录
	pwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// 获取程序名称
	programName := getProgramName()

	// 可能的配置文件名
	configNames := []string{
		"." + programName + ".ini",
		programName + ".ini",
		".app-manager.ini",
		"app-manager.ini",
		// 兼容旧格式
		".app-manager.yaml",
		".app-manager.yml",
		"app-manager.yaml",
		"app-manager.yml",
	}

	// 搜索策略：
	// 1. 当前目录
	// 2. 当前目录的 project_backend 子目录
	// 3. 父目录及其 project_backend 子目录
	// 4. 如果当前目录是 project_backend，则搜索当前目录和父目录
	
	searchDirs := []string{}
	
	// 添加当前目录
	searchDirs = append(searchDirs, pwd)
	
	// 如果当前目录是 project_backend，优先搜索当前目录
	if filepath.Base(pwd) == "project_backend" {
		// 当前目录是 project_backend，也搜索父目录
		parentDir := filepath.Dir(pwd)
		searchDirs = append(searchDirs, parentDir)
	} else {
		// 当前目录不是 project_backend，搜索 project_backend 子目录
		backendDir := filepath.Join(pwd, "project_backend")
		if _, err := os.Stat(backendDir); err == nil {
			searchDirs = append(searchDirs, backendDir)
		}
		
		// 向上搜索父目录
		for dir := filepath.Dir(pwd); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			searchDirs = append(searchDirs, dir)
			
			// 也搜索父目录的 project_backend 子目录
			parentBackendDir := filepath.Join(dir, "project_backend")
			if _, err := os.Stat(parentBackendDir); err == nil {
				searchDirs = append(searchDirs, parentBackendDir)
			}
		}
	}

	// 在所有搜索目录中查找配置文件
	for _, dir := range searchDirs {
		for _, name := range configNames {
			configPath := filepath.Join(dir, name)
			if _, err := os.Stat(configPath); err == nil {
				return configPath
			}
		}
	}

	return ""
}

// getProgramName 获取程序名称
func getProgramName() string {
	programPath := os.Args[0]
	programName := filepath.Base(programPath)
	
	// 移除扩展名
	if ext := filepath.Ext(programName); ext != "" {
		programName = strings.TrimSuffix(programName, ext)
	}
	
	return programName
}

// GetProjectRoot 获取项目根目录
func GetProjectRoot() string {
	if AppConfig.Global.ProjectRoot != "" {
		return AppConfig.Global.ProjectRoot
	}

	// 如果配置文件中没有设置，尝试从配置文件路径推断
	if ConfigFile != "" {
		dir := filepath.Dir(ConfigFile)
		if strings.HasSuffix(dir, "project_backend") {
			return filepath.Dir(dir)
		}
		return dir
	}

	// 智能推断项目根目录
	return findProjectRoot()
}

// findProjectRoot 智能查找项目根目录
func findProjectRoot() string {
	pwd, err := os.Getwd()
	if err != nil {
		return "."
	}

	// 检查当前目录及其父目录，寻找项目根目录的标志
	for dir := pwd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		// 检查是否存在 project_backend 目录（项目根目录标志）
		backendDir := filepath.Join(dir, "project_backend")
		if _, err := os.Stat(backendDir); err == nil {
			return dir
		}
		
		// 检查当前目录是否就是 project_backend
		if filepath.Base(dir) == "project_backend" {
			return filepath.Dir(dir)
		}
	}

	// 如果没有找到明确的项目根目录，检查当前目录是否包含应用文件
	if hasApplicationFiles(pwd) {
		// 如果当前目录包含应用文件，可能是在 project_backend 目录中运行
		// 检查父目录是否有其他 project_* 目录
		parentDir := filepath.Dir(pwd)
		if hasProjectStructure(parentDir) {
			return parentDir
		}
		// 否则当前目录就是项目根目录
		return pwd
	}

	// 默认使用当前目录
	return pwd
}

// hasApplicationFiles 检查目录是否包含应用文件
func hasApplicationFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	
	for _, entry := range entries {
		if !entry.IsDir() {
			name := entry.Name()
			if strings.HasSuffix(name, ".jar") || strings.HasSuffix(name, ".war") {
				return true
			}
		}
	}
	return false
}

// hasProjectStructure 检查目录是否具有项目结构
func hasProjectStructure(dir string) bool {
	// 检查是否存在多个 project_* 目录
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	
	projectDirs := 0
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "project_") {
			projectDirs++
		}
	}
	
	return projectDirs >= 2 // 至少有两个 project_* 目录才认为是项目根目录
}

// GetBackendDir 获取后端目录
func GetBackendDir() string {
	return filepath.Join(GetProjectRoot(), "project_backend")
}

// GetLogsDir 获取日志目录
func GetLogsDir() string {
	return filepath.Join(GetBackendDir(), "logs")
}

// GetPidsDir 获取PID目录
func GetPidsDir() string {
	return filepath.Join(GetBackendDir(), "pids")
}

// GetConfigDir 获取配置目录
func GetConfigDir() string {
	return filepath.Join(GetBackendDir(), "config")
}

// IsNacosEnabled 检查是否启用了Nacos
func IsNacosEnabled() bool {
	return AppConfig.Nacos.NacosServer != ""
}

// GetNacosEnvironmentVars 获取Nacos环境变量
func GetNacosEnvironmentVars() map[string]string {
	envVars := make(map[string]string)
	
	if IsNacosEnabled() {
		envVars["NACOS_SERVER"] = AppConfig.Nacos.NacosServer
		envVars["NACOS_ADDRESS"] = AppConfig.Nacos.NacosServer
		
		if AppConfig.Nacos.NacosNamespace != "" {
			envVars["NACOS_NAMESPACE"] = AppConfig.Nacos.NacosNamespace
		}
	}
	
	return envVars
}

// GetOptimizedJVMParams 获取优化后的JVM参数（包含随机数优化）
func GetOptimizedJVMParams() string {
	jvmParams := AppConfig.Application.JVMParams
	
	// 随机数优化参数
	randomOptimization := "-Djava.security.egd=file:/dev/./urandom"
	
	// 检查是否已经包含随机数优化参数
	if !strings.Contains(jvmParams, "java.security.egd") {
		if jvmParams == "" {
			jvmParams = randomOptimization
		} else {
			jvmParams = jvmParams + " " + randomOptimization
		}
	}
	
	return jvmParams
}

// GetConfigFileName 获取配置文件名称
func GetConfigFileName() string {
	programName := getProgramName()
	return "." + programName + ".ini"
}

// GetProgramName 获取程序名称（公开函数）
func GetProgramName() string {
	return getProgramName()
}

// IsProjectInitialized 检查项目是否已初始化
func IsProjectInitialized() bool {
	// 检查配置文件是否存在
	configFile := findConfigFile()
	if configFile == "" {
		return false
	}
	
	// 检查基本目录结构是否存在
	backendDir := GetBackendDir()
	if _, err := os.Stat(backendDir); os.IsNotExist(err) {
		return false
	}
	
	return true
}

// GetSystemInfo 获取系统信息
func GetSystemInfo() map[string]string {
	info := make(map[string]string)
	
	// 主机名
	if hostname, err := os.Hostname(); err == nil {
		info["hostname"] = hostname
	} else {
		info["hostname"] = "未知"
	}
	
	// IP地址（获取所有网卡IP）
	info["ip"] = getAllNetworkIPs()
	
	// 项目目录
	info["project_root"] = GetProjectRoot()
	
	// Nacos地址
	if IsNacosEnabled() {
		info["nacos_server"] = AppConfig.Nacos.NacosServer
		info["nacos_namespace"] = AppConfig.Nacos.NacosNamespace
	} else {
		info["nacos_server"] = "未启用"
		info["nacos_namespace"] = "未启用"
	}
	
	// JVM参数
	jvmParams := GetOptimizedJVMParams()
	if jvmParams != "" {
		info["jvm_params"] = jvmParams
	} else {
		info["jvm_params"] = "默认"
	}
	
	return info
}

// GetBackupDir 获取备份目录
func GetBackupDir() string {
	return filepath.Join(GetProjectRoot(), "project_backup")
}

// getAllNetworkIPs 获取所有网卡的IP地址
func getAllNetworkIPs() string {
	var ips []string
	
	// 获取所有网络接口
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	
	for _, iface := range interfaces {
		// 跳过回环接口和未启用的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		
		// 获取接口的地址
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		
		for _, addr := range addrs {
			// 只处理IP地址
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				// 只获取IPv4地址
				if ipNet.IP.To4() != nil {
					ips = append(ips, ipNet.IP.String())
				}
			}
		}
	}
	
	// 如果没有找到任何IP，返回默认值
	if len(ips) == 0 {
		return "127.0.0.1"
	}
	
	// 返回所有IP地址，用逗号分隔
	return strings.Join(ips, ", ")
}