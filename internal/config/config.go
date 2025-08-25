package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用配置结构
type Config struct {
	Global       GlobalConfig           `mapstructure:"global" ini:"global"`
	Nacos        NacosConfig            `mapstructure:"nacos" ini:"nacos"`
	Application  ApplicationConfig      `mapstructure:"application" ini:"application"`
	Applications map[string]AppPriority `mapstructure:"applications" ini:"applications"` // 应用优先级配置
	Monitoring   MonitoringConfig       `mapstructure:"monitoring" ini:"monitoring"`
	Backup       BackupConfig           `mapstructure:"backup" ini:"backup"`
	Logrotate    LogrotateConfig        `mapstructure:"logrotate" ini:"logrotate"`
	Systemd      SystemdConfig          `mapstructure:"systemd" ini:"systemd"`
}

// GlobalConfig 全局配置
type GlobalConfig struct {
	ProjectRoot string `mapstructure:"project_root" ini:"project_root"`
	JavaPath    string `mapstructure:"java_path" ini:"java_path"`
}

// NacosConfig Nacos配置
type NacosConfig struct {
	NacosServer       string `mapstructure:"nacos_server" ini:"nacos_server"`
	NacosNamespace    string `mapstructure:"nacos_namespace" ini:"nacos_namespace"`
	NacosAuthName     string `mapstructure:"nacos_auth_name" ini:"nacos_auth_name"`
	NacosAuthPassword string `mapstructure:"nacos_auth_password" ini:"nacos_auth_password"`
}

// ApplicationConfig 应用配置
type ApplicationConfig struct {
	AppType         string `mapstructure:"app_type" ini:"app_type"`
	JVMParams       string `mapstructure:"jvm_params" ini:"jvm_params"`
	StartupDelay    int    `mapstructure:"startup_delay" ini:"startup_delay"`       // 启动间隔时间(秒)
	DefaultPriority int    `mapstructure:"default_priority" ini:"default_priority"` // 默认优先级
}

// ApplicationsConfig 应用优先级配置
type ApplicationsConfig map[string]int

// AppPriority 单个应用的优先级配置
type AppPriority struct {
	Priority     int `mapstructure:"priority"`
	StartupDelay int `mapstructure:"startup_delay"`
}

// MonitoringConfig 监控配置
type MonitoringConfig struct {
	EnableDiskIO     bool   `mapstructure:"enable_disk_io" ini:"enable_disk_io"`
	EnableNetwork    bool   `mapstructure:"enable_network" ini:"enable_network"`
	EnablePortStats  bool   `mapstructure:"enable_port_stats" ini:"enable_port_stats"`
	MonitorPorts     string `mapstructure:"monitor_ports" ini:"monitor_ports"`         // 监控端口列表(逗号分隔)
	OpenRestyPort    int    `mapstructure:"openresty_port" ini:"openresty_port"`       // OpenResty前端端口
	SampleInterval   int    `mapstructure:"sample_interval" ini:"sample_interval"`     // 采样间隔(秒)
	RetentionTime    int    `mapstructure:"retention_time" ini:"retention_time"`       // 数据保留时间(秒)
	DiskIOThreshold  uint64 `mapstructure:"disk_io_threshold" ini:"disk_io_threshold"` // 磁盘I/O阈值(字节/秒)
	NetworkThreshold uint64 `mapstructure:"network_threshold" ini:"network_threshold"` // 网络阈值(字节/秒)
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

// DefaultConfigGenerator 默认配置生成器
type DefaultConfigGenerator struct{}

// NewDefaultConfigGenerator 创建新的默认配置生成器
func NewDefaultConfigGenerator() *DefaultConfigGenerator {
	return &DefaultConfigGenerator{}
}

// GenerateDefaultConfig 生成纯净的默认配置
func (g *DefaultConfigGenerator) GenerateDefaultConfig() *Config {
	return &Config{
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
		Monitoring: MonitoringConfig{
			EnableDiskIO:     true,
			EnableNetwork:    true,
			EnablePortStats:  true,
			MonitorPorts:     "",
			OpenRestyPort:    0,
			SampleInterval:   5,
			RetentionTime:    300,
			DiskIOThreshold:  1048576, // 1MB/s
			NetworkThreshold: 1048576, // 1MB/s
		},
		Backup: BackupConfig{
			BackupPrefix:        "Application_Backup",
			BackupRetentionDays: 30,
		},
		Logrotate: LogrotateConfig{
			LogrotateSize:   "30M",
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

// Init 初始化配置
func Init() {
	generator := NewDefaultConfigGenerator()
	AppConfig = generator.GenerateDefaultConfig()
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

	// 手动加载启动优先级
	loadStartupPriority()

	// 清理配置值中的引号
	cleanConfigValues()

	return nil
}

// loadStartupPriority 手动加载启动优先级
func loadStartupPriority() {
	// 直接从 applications section 加载配置
	applicationsSettings := viper.GetStringMap("applications")
	if AppConfig.Applications == nil {
		AppConfig.Applications = make(map[string]AppPriority)
	}

	for key, value := range applicationsSettings {
		parts := strings.Split(key, ".")
		if len(parts) != 2 {
			continue
		}
		appName, prop := parts[0], parts[1]

		// 获取或创建应用优先级配置
		appPriority, ok := AppConfig.Applications[appName]
		if !ok {
			appPriority = AppPriority{
				Priority:     AppConfig.Application.DefaultPriority,
				StartupDelay: AppConfig.Application.StartupDelay,
			}
		}

		// 解析并设置值
		switch prop {
		case "priority":
			if p, err := strconv.Atoi(fmt.Sprintf("%v", value)); err == nil {
				appPriority.Priority = p
			}
		case "startup_delay":
			if d, err := strconv.Atoi(fmt.Sprintf("%v", value)); err == nil {
				appPriority.StartupDelay = d
			}
		}
		AppConfig.Applications[appName] = appPriority
	}
}

// cleanConfigValues 清理配置值中的引号
func cleanConfigValues() {
	// 清理全局配置
	AppConfig.Global.ProjectRoot = cleanQuotes(AppConfig.Global.ProjectRoot)
	AppConfig.Global.JavaPath = cleanQuotes(AppConfig.Global.JavaPath)

	// 清理Nacos配置
	AppConfig.Nacos.NacosServer = cleanQuotes(AppConfig.Nacos.NacosServer)
	AppConfig.Nacos.NacosNamespace = cleanQuotes(AppConfig.Nacos.NacosNamespace)
	AppConfig.Nacos.NacosAuthName = cleanQuotes(AppConfig.Nacos.NacosAuthName)
	AppConfig.Nacos.NacosAuthPassword = cleanQuotes(AppConfig.Nacos.NacosAuthPassword)

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
	searchDirs := getSearchDirs()

	// 获取程序名称
	programName := GetProgramName()

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

// getSearchDirs 获取所有需要搜索的目录列表，已去重
func getSearchDirs() []string {
	// 使用 map 来自动去重
	uniqueDirs := make(map[string]struct{})

	// 1. 添加可执行文件所在目录及其 project_backend 子目录
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		uniqueDirs[exeDir] = struct{}{}

		// 检查可执行文件目录下的 project_backend 子目录
		exeBackendDir := filepath.Join(exeDir, "project_backend")
		if _, err := os.Stat(exeBackendDir); err == nil {
			uniqueDirs[exeBackendDir] = struct{}{}
		}
	}

	// 2. 添加当前工作目录及其相关目录
	if pwd, err := os.Getwd(); err == nil {
		uniqueDirs[pwd] = struct{}{}

		// 检查 project_backend 子目录
		backendDir := filepath.Join(pwd, "project_backend")
		if _, err := os.Stat(backendDir); err == nil {
			uniqueDirs[backendDir] = struct{}{}
		}

		// 如果当前在 project_backend，也添加其父目录
		if filepath.Base(pwd) == "project_backend" {
			uniqueDirs[filepath.Dir(pwd)] = struct{}{}
		}

		// 向上递归搜索父目录
		for dir := pwd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			uniqueDirs[dir] = struct{}{}
			// 检查父目录下的 project_backend
			parentBackendDir := filepath.Join(dir, "project_backend")
			if _, err := os.Stat(parentBackendDir); err == nil {
				uniqueDirs[parentBackendDir] = struct{}{}
			}
		}

		// 3. 搜索当前目录下的所有子目录，查找可能的项目目录
		// 这解决了在项目根目录运行时无法找到子目录中配置文件的问题
		if entries, err := os.ReadDir(pwd); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					subDir := filepath.Join(pwd, entry.Name())
					// 检查子目录下的 project_backend
					subBackendDir := filepath.Join(subDir, "project_backend")
					if _, err := os.Stat(subBackendDir); err == nil {
						uniqueDirs[subBackendDir] = struct{}{}
						// 也添加子目录本身
						uniqueDirs[subDir] = struct{}{}
					}
				}
			}
		}
	}

	// 将 map 的键转换为切片
	dirs := make([]string, 0, len(uniqueDirs))
	for dir := range uniqueDirs {
		dirs = append(dirs, dir)
	}

	return dirs
}

// GetProgramName 获取程序名称
func GetProgramName() string {
	programPath, err := os.Executable()
	if err != nil {
		return "app-manager" // Fallback
	}
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

// GetRotateLogsDir 获取日志轮转目录
func GetRotateLogsDir() string {
	return filepath.Join(GetBackendDir(), "rotate_logs")
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

		// 添加 Nacos 2.5.1 鉴权支持
		if AppConfig.Nacos.NacosAuthName != "" {
			envVars["NACOS_AUTH_NAME"] = AppConfig.Nacos.NacosAuthName
			envVars["NACOS_USERNAME"] = AppConfig.Nacos.NacosAuthName // 兼容性别名
		}

		if AppConfig.Nacos.NacosAuthPassword != "" {
			envVars["NACOS_AUTH_PASSWORD"] = AppConfig.Nacos.NacosAuthPassword
			envVars["NACOS_PASSWORD"] = AppConfig.Nacos.NacosAuthPassword // 兼容性别名
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
	programName := GetProgramName()
	return "." + programName + ".ini"
}

// IsProjectInitialized 检查项目是否已初始化
func IsProjectInitialized() bool {
	// 首先检查是否已经加载了配置文件
	var configFile string
	if ConfigFile != "" {
		// 使用已加载的配置文件
		configFile = ConfigFile
	} else {
		// 尝试查找配置文件
		configFile = findConfigFile()
		if configFile == "" {
			return false
		}
	}

	// 检查配置文件是否真实存在
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
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

// GetTempDir 获取项目临时目录路径
// 返回 {PROJECT_ROOT}/.tmp 目录，如果目录不存在则自动创建
func GetTempDir() string {
	tempDir := filepath.Join(GetProjectRoot(), ".tmp")
	// 确保临时目录存在
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		// 如果创建失败，记录错误但不中断程序
		fmt.Printf("Warning: Failed to create temp directory %s: %v\n", tempDir, err)
	}
	return tempDir
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
