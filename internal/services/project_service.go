package services

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IncludeDatabaseConfigInInit 控制在init时是否包含数据库配置
const IncludeDatabaseConfigInInit = false

// ProjectService 项目服务
type ProjectService struct {
	config *config.Config
}

// NewProjectService 创建项目服务实例
func NewProjectService() *ProjectService {
	return &ProjectService{
		config: config.AppConfig,
	}
}

// NewProjectServiceWithDefaults 创建使用默认配置的项目服务实例
// 此方法专门用于 init 命令，确保使用纯净的默认配置
func NewProjectServiceWithDefaults() *ProjectService {
	generator := config.NewDefaultConfigGenerator()
	defaultConfig := generator.GenerateDefaultConfig()
	return &ProjectService{
		config: defaultConfig,
	}
}

// InitProject 初始化项目
func (s *ProjectService) InitProject(projectPath string) error {
	if projectPath == "" {
		var err error
		projectPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前目录失败: %w", err)
		}
		logger.Info("未指定项目路径，将在当前目录初始化")
	}

	// 转换为绝对路径
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("转换为绝对路径失败: %w", err)
	}

	projectPath = absPath
	logger.Title("Java应用管理系统 - 项目初始化")
	logger.Info("开始初始化项目目录: %s", projectPath)

	// 创建目录结构
	if err := s.createDirectoryStructure(projectPath); err != nil {
		return err
	}

	// 创建配置文件
	if err := s.createConfigFile(projectPath); err != nil {
		return err
	}

	// 复制程序文件到project_backend目录
	if err := s.copyProgramFiles(projectPath); err != nil {
		return err
	}

	logger.Success("项目初始化完成！")
	s.printDirectoryStructure(projectPath)
	s.printUsageInstructions(projectPath)

	return nil
}

// copyProgramFiles 复制程序文件到project_backend目录
func (s *ProjectService) copyProgramFiles(projectRoot string) error {
	logger.Info("复制程序文件到project_backend目录...")

	// 获取当前可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	// 获取可执行文件所在目录
	execDir := filepath.Dir(execPath)
	backendDir := filepath.Join(projectRoot, "project_backend")

	// Linux环境，复制Linux版本的可执行文件
	var filesToCopy []struct {
		src, dst, desc string
	}

	distDir := filepath.Join(execDir, "dist")
	programName := config.GetProgramName()

	// 检查dist目录是否存在
	if _, err := os.Stat(distDir); err == nil {
		// 复制Linux AMD64版本
		amd64File := filepath.Join(distDir, programName+"-linux-amd64")
		if _, err := os.Stat(amd64File); err == nil {
			filesToCopy = append(filesToCopy, struct {
				src, dst, desc string
			}{
				src:  amd64File,
				dst:  filepath.Join(backendDir, programName),
				desc: "Linux AMD64可执行文件",
			})
		}

		// 复制Linux ARM64版本（可选）
		arm64File := filepath.Join(distDir, programName+"-linux-arm64")
		if _, err := os.Stat(arm64File); err == nil {
			filesToCopy = append(filesToCopy, struct {
				src, dst, desc string
			}{
				src:  arm64File,
				dst:  filepath.Join(backendDir, programName+"-arm64"),
				desc: "Linux ARM64可执行文件",
			})
		}
	}

	// 如果没有找到dist目录中的文件，复制当前可执行文件
	if len(filesToCopy) == 0 {
		filesToCopy = append(filesToCopy, struct {
			src, dst, desc string
		}{
			src:  execPath,
			dst:  filepath.Join(backendDir, programName),
			desc: "当前可执行文件",
		})
	}

	// 执行文件复制
	for _, file := range filesToCopy {
		if err := s.copyFile(file.src, file.dst); err != nil {
			logger.Warning("复制%s失败: %v", file.desc, err)
			continue
		}

		// 设置可执行权限（Linux）
		if err := os.Chmod(file.dst, 0755); err != nil {
			logger.Warning("设置可执行权限失败 %s: %v", file.dst, err)
		}

		logger.Success("复制%s: %s", file.desc, filepath.Base(file.dst))
	}

	if len(filesToCopy) == 0 {
		logger.Warning("未找到可复制的程序文件")
		return nil
	}

	return nil
}

// copyFile 复制文件
func (s *ProjectService) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// getConfigTemplateSection 辅助函数，用于生成配置段
func (s *ProjectService) getConfigTemplateSection(title, content string) string {
	var sb strings.Builder
	sb.WriteString("\n# ═══════════════════════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("#                        %s\n", title))
	sb.WriteString("# ═══════════════════════════════════════════════════════════\n")
	sb.WriteString(content)
	return sb.String()
}

// getGlobalConfigTemplate 获取全局配置模板
func (s *ProjectService) getGlobalConfigTemplate() string {
	return `[global]

# 项目根目录 [必填]
# 说明：项目的根目录路径，必须使用绝对路径
# 格式：/path/to/project (Linux)
# 示例：/opt/myapp 或 /home/user/projects/myapp
# 注意：路径中不能包含空格和特殊字符，建议使用英文路径
project_root="{{PROJECT_ROOT}}"

# Java执行路径 [选填，默认：java]
# 说明：Java可执行文件的路径，如果java命令在PATH中可以直接使用"java"
# 如果需要指定特定版本的Java，请填写完整路径
# 格式：java 或 /path/to/java
# 示例：
#   java                           # 使用系统PATH中的java
#   /usr/local/java/bin/java       # 指定Java 8
#   /opt/jdk-11/bin/java          # 指定Java 11
#   /usr/lib/jvm/java-17-openjdk/bin/java  # 指定Java 17
# 注意：不需要引号包围
java_path="{{JAVA_PATH}}"`
}

// getNacosConfigTemplate 获取Nacos配置模板
func (s *ProjectService) getNacosConfigTemplate() string {
	return `[nacos]

# Nacos服务器地址 [选填]
# 说明：Nacos配置中心的服务器地址，留空表示不使用Nacos
# 格式：IP:PORT 或 域名:PORT
# 示例：
#   192.168.1.100:8848           # 单机模式
#   nacos.example.com:8848       # 域名模式
#   192.168.1.100:8848,192.168.1.101:8848  # 集群模式(逗号分隔)
# 注意：
#   - 必须包含端口号，默认端口为8848
#   - 集群模式下多个地址用英文逗号分隔
#   - 不需要http://前缀
#   - 不需要引号包围
nacos_server=

# Nacos命名空间 [选填，默认：public]
# 说明：Nacos的命名空间ID，留空使用默认命名空间(public)
# 格式：字符串，通常是UUID格式
# 示例：
#   dev                          # 开发环境
#   test                         # 测试环境
#   prod                         # 生产环境
#   550e8400-e29b-41d4-a716-446655440000  # UUID格式
# 注意：不需要引号包围
nacos_namespace=

# Nacos鉴权用户名 [选填，适用于 Nacos 2.5.1+ 开启鉴权]
# 说明：当Nacos启用鉴权功能时的用户名，留空表示不使用鉴权
# 格式：字符串
# 示例：
#   nacos                        # 默认用户
#   admin                        # 管理员用户
#   app_user                     # 应用专用用户
# 注意：
#   - 仅在Nacos 2.5.1+版本开启鉴权后需要配置
#   - 与nacos_auth_password配对使用
#   - 不需要引号包围
nacos_auth_name=

# Nacos鉴权密码 [选填，适用于 Nacos 2.5.1+ 开启鉴权]
# 说明：当Nacos启用鉴权功能时的密码，留空表示不使用鉴权
# 格式：字符串
# 示例：
#   nacos                        # 默认密码
#   your_secure_password         # 自定义密码
# 注意：
#   - 仅在Nacos 2.5.1+版本开启鉴权后需要配置
#   - 与nacos_auth_name配对使用
#   - 不需要引号包围
#   - 建议使用强密码
nacos_auth_password=`
}

// getApplicationsConfigTemplate 获取应用启动优先级配置模板
func (s *ProjectService) getApplicationsConfigTemplate() string {
	return `[applications]

# 说明：
#   本配置段用于定义特定应用程序的启动顺序和延迟。
#   对于有依赖关系的应用（如网关依赖于认证中心），此功能非常有用。
#   未在此处配置的应用将按默认顺序启动。
#
# 配置格式：
#   <app_name>.priority = <priority>
#   <app_name>.startup_delay = <delay_in_seconds>
#
# 参数说明：
#   <app_name>          - 应用程序的名称（不含.jar后缀）。
#   priority            - 启动优先级，数字越小，优先级越高（例如，1会比10先启动）。
#   startup_delay       - 启动前延迟的秒数，用于等待依赖项完全启动。
#
# 示例：
#   假设 party-gateway 依赖于 party-auth。
#   我们希望 party-auth 首先启动，并等待5秒后，再启动 party-gateway。
#
#   party-auth.priority = 10
#   party-auth.startup_delay = 0
#
#   party-gateway.priority = 20
#   party-gateway.startup_delay = 5
#
# 注意：
#   - 每个应用的 priority 和 startup_delay 必须成对出现。
#   - app_name 必须是唯一的。
#   - 如果多个应用的 priority 相同，它们的启动顺序将不确定。
#   - 此处无需填写任何内容，仅为示例说明。`
}

// getApplicationConfigTemplate 获取应用配置模板
func (s *ProjectService) getApplicationConfigTemplate() string {
	return `[application]

# 应用程序类型 [选填，默认：mixed]
# 说明：指定要启动的应用程序类型
# 可选值：
#   jar    - 只启动jar包应用（Spring Boot应用）
#   war    - 只启动war包应用（传统Web应用）
#   mixed  - 同时启动jar包和war包应用
# 示例：
#   app_type=jar     # 只管理jar包
#   app_type=war     # 只管理war包
#   app_type=mixed   # 管理所有类型
# 注意：不需要引号包围
app_type="{{APP_TYPE}}"

# JVM参数 [选填]
# 说明：传递给Java虚拟机的参数，用于控制内存、GC、系统属性等
# 格式：多个参数用空格分隔，整体用双引号包围
# 常用参数说明：
#   内存设置：
#     -Xms512m          # 初始堆内存512MB
#     -Xmx2g            # 最大堆内存2GB
#     -XX:MetaspaceSize=256m  # 元空间初始大小
#     -XX:MaxMetaspaceSize=512m  # 元空间最大大小
#   
#   垃圾收集器：
#     -XX:+UseG1GC      # 使用G1垃圾收集器(推荐)
#     -XX:+UseZGC       # 使用ZGC垃圾收集器(Java 11+)
#     -XX:+UseParallelGC # 使用并行垃圾收集器
#   
#   Spring配置：
#     -Dspring.profiles.active=prod    # 指定Spring环境
#     -Dserver.port=8080               # 指定服务端口
#   
#   系统优化：
#     -Djava.security.egd=file:/dev/./urandom  # 随机数优化
#     -Dfile.encoding=UTF-8            # 文件编码
#     -Duser.timezone=Asia/Shanghai    # 时区设置
#   
#   调试参数：
#     -Xdebug -Xrunjdwp:transport=dt_socket,server=y,suspend=n,address=5005
#
# 示例配置：
#   # 小型应用(1GB内存)
#   jvm_params="-Xms256m -Xmx1g -XX:+UseG1GC -Djava.security.egd=file:/dev/./urandom"
#   
#   # 中型应用(4GB内存)
#   jvm_params="-Xms1g -Xmx4g -XX:MetaspaceSize=256m -XX:+UseG1GC -Dspring.profiles.active=prod"
#   
#   # 大型应用(8GB内存)
#   jvm_params="-Xms2g -Xmx8g -XX:MetaspaceSize=512m -XX:+UseG1GC -XX:G1HeapRegionSize=16m"
#
# 注意：必须用双引号包围整个参数字符串
jvm_params="{{JVM_PARAMS}}"`
}

// getBackupConfigTemplate 获取备份配置模板
func (s *ProjectService) getBackupConfigTemplate() string {
	return `[backup]

# 备份文件前缀 [选填，默认：Application_Backup]
# 说明：生成备份文件时使用的前缀名称
# 格式：字符串，不能包含特殊字符 / \ : * ? " < > |
# 示例：
#   MyApp_Backup      # 自定义应用备份
#   Project_Backup    # 项目备份
#   Daily_Backup      # 日常备份
# 注意：不需要引号包围
backup_prefix="{{BACKUP_PREFIX}}"

# 备份保留天数 [选填，默认：30]
# 说明：自动清理多少天前的备份文件，0表示不自动清理
# 格式：正整数
# 示例：
#   7     # 保留7天内的备份
#   30    # 保留30天内的备份
#   90    # 保留90天内的备份
#   0     # 不自动清理备份
# 注意：不需要引号包围
backup_retention_days={{BACKUP_RETENTION_DAYS}}`
}

// getLogrotateConfigTemplate 获取日志轮转配置模板
func (s *ProjectService) getLogrotateConfigTemplate() string {
	return `[logrotate]

# 日志轮转大小 [选填，默认：30M]
# 说明：单个日志文件达到此大小时进行轮转
# 格式：数字+单位(k/m/g)，不区分大小写
# 示例：
#   10k   # 10KB
#   30M   # 30MB
#   1g    # 1GB
#   500M  # 500MB
# 注意：不需要引号包围
logrotate_size="{{LOGROTATE_SIZE}}"

# 日志轮转周期 [选填，默认：weekly]
# 说明：日志轮转的时间周期
# 可选值：
#   daily    - 每天轮转
#   weekly   - 每周轮转
#   monthly  - 每月轮转
#   yearly   - 每年轮转
# 示例：
#   daily    # 适合高并发应用
#   weekly   # 适合中等负载应用
#   monthly  # 适合低负载应用
# 注意：不需要引号包围
logrotate_cycle="{{LOGROTATE_CYCLE}}"

# 日志保留份数 [选填，默认：30]
# 说明：保留多少份轮转后的日志文件，0表示不限制
# 格式：正整数
# 示例：
#   10    # 保留10份日志文件
#   30    # 保留30份日志文件
#   0     # 不限制保留份数
# 注意：不需要引号包围
logrotate_rotate={{LOGROTATE_ROTATE}}`
}

// getSystemdConfigTemplate 获取Systemd服务配置模板
func (s *ProjectService) getSystemdConfigTemplate() string {
	return `[systemd]

# 服务重启策略 [选填，默认：always]
# 说明：systemd服务失败后的重启策略
# 可选值：
#   no         - 不重启
#   always     - 总是重启（推荐）
#   on-failure - 只在失败时重启
#   on-success - 只在成功退出时重启
#   on-abnormal - 只在异常退出时重启
# 示例：
#   always     # 生产环境推荐
#   on-failure # 开发环境推荐
#   no         # 调试时使用
# 注意：不需要引号包围
service_restart="{{SERVICE_RESTART}}"

# 服务重启间隔 [选填，默认：10]
# 说明：服务重启前等待的秒数，避免频繁重启
# 格式：正整数（秒）
# 示例：
#   5     # 5秒间隔，快速重启
#   10    # 10秒间隔，标准配置
#   30    # 30秒间隔，避免频繁重启
# 注意：不需要引号包围
service_restart_sec={{SERVICE_RESTART_SEC}}

# 服务启动超时时间 [选填，默认：0]
# 说明：服务启动超时时间，超过此时间认为启动失败
# 格式：正整数（秒），0表示使用系统默认值
# 示例：
#   30    # 30秒超时，适合轻量应用
#   60    # 60秒超时，适合标准应用
#   120   # 120秒超时，适合重型应用
#   0     # 使用系统默认值
# 注意：不需要引号包围
service_timeout_start_sec={{SERVICE_TIMEOUT_START_SEC}}

# 服务停止超时时间 [选填，默认：0]
# 说明：服务停止超时时间，超过此时间强制杀死进程
# 格式：正整数（秒），0表示使用系统默认值
# 示例：
#   15    # 15秒超时，快速停止
#   30    # 30秒超时，标准配置
#   60    # 60秒超时，给应用更多时间清理
#   0     # 使用系统默认值
# 注意：不需要引号包围
service_timeout_stop_sec={{SERVICE_TIMEOUT_STOP_SEC}}`
}

// getMonitoringConfigTemplate 获取监控配置模板
func (s *ProjectService) getMonitoringConfigTemplate() string {
	return `[monitoring]

# 是否启用磁盘I/O监控 [选填，默认：true]
# 说明：是否在status命令中显示磁盘读写速率信息
# 可选值：true, false
# 示例：
#   true    # 启用磁盘I/O监控
#   false   # 禁用磁盘I/O监控
# 注意：不需要引号包围
enable_disk_io=true

# 是否启用网络监控 [选填，默认：true]
# 说明：是否在status命令中显示网络流量统计信息
# 可选值：true, false
# 示例：
#   true    # 启用网络监控
#   false   # 禁用网络监控
# 注意：不需要引号包围
enable_network=true

# 是否启用端口连接监控 [选填，默认：true]
# 说明：是否在status命令中显示端口连接状态信息
# 可选值：true, false
# 示例：
#   true    # 启用端口连接监控
#   false   # 禁用端口连接监控
# 注意：不需要引号包围
enable_port_stats=true

# 监控端口列表 [选填]
# 说明：指定需要独立监控的端口列表，多个端口用逗号分隔
# 格式：端口号列表，用英文逗号分隔
# 示例：
#   monitor_ports=80,443,8080,8081    # 监控多个端口
#   monitor_ports=80                  # 监控单个端口
#   monitor_ports=                    # 不监控特定端口
# 注意：
#   - 端口号范围：1-65535
#   - 多个端口用英文逗号分隔，不要有空格
#   - 不需要引号包围
#   - 留空表示不监控特定端口
monitor_ports=

# OpenResty/Nginx前端端口 [可选功能]
# 说明：如果您使用OpenResty或Nginx作为前端代理，可以配置此端口用于统计在线人数
# 格式：端口号
# 示例：
#   openresty_port=80     # HTTP端口
#   openresty_port=443    # HTTPS端口
#   openresty_port=8080   # 自定义端口
# 注意：
#   - 如果您不使用OpenResty/Nginx，请保持此项为空
#   - 端口号范围：1-65535
#   - 不需要引号包围
#   - 留空表示不启用此功能
openresty_port=

# 监控数据采样间隔 [选填，默认：5]
# 说明：监控数据采样的时间间隔（秒）
# 格式：正整数
# 示例：
#   1     # 1秒采样，高精度监控
#   5     # 5秒采样，标准配置
#   10    # 10秒采样，低频监控
# 注意：
#   - 间隔越小，监控精度越高，但系统开销也越大
#   - 不需要引号包围
sample_interval=5

# 监控数据保留时间 [选填，默认：300]
# 说明：监控数据在内存中的保留时间（秒）
# 格式：正整数
# 示例：
#   60    # 保留1分钟数据
#   300   # 保留5分钟数据
#   600   # 保留10分钟数据
# 注意：
#   - 保留时间越长，内存占用越大
#   - 不需要引号包围
retention_time=300`
}

// getDatabaseDmConfigTemplate 获取达梦数据库备份配置模板
func (s *ProjectService) getDatabaseDmConfigTemplate() string {
	return `[database-dm]

# 是否启用达梦数据库备份 [选填，默认：false]
# 说明：是否启用达梦数据库的自动备份功能
# 可选值：true, false
# 注意：启用后必须配置下面的连接信息
enabled=false

# 数据库主机地址 [必填，如果enabled=true]
# 说明：达梦数据库服务器的IP地址或域名
# 格式：IP地址或域名，不包含端口号
# 示例：
#   localhost         # 本地数据库
#   192.168.1.100     # 内网数据库
#   db.example.com    # 域名访问
# 注意：不需要引号包围
host="localhost"

# 数据库端口 [选填，默认：5236]
# 说明：达梦数据库的端口号
# 格式：正整数，范围1-65535
# 示例：
#   5236    # 达梦数据库默认端口
#   5237    # 自定义端口
# 注意：不需要引号包围
port=5236

# 数据库用户名 [必填，如果enabled=true]
# 说明：连接达梦数据库的用户名
# 格式：字符串
# 示例：
#   SYSDBA    # 系统管理员
#   APP_USER  # 应用用户
# 注意：不需要引号包围
username="SYSDBA"

# 数据库密码 [必填，如果enabled=true]
# 说明：连接达梦数据库的密码
# 格式：字符串，可以为空
# 示例：
#   password=""       # 空密码
#   password="123456" # 有密码
# 注意：必须用双引号包围，即使是空密码
password=""

# 数据库名称 [必填，如果enabled=true]
# 说明：要备份的数据库名称
# 格式：字符串
# 示例：
#   DMSERVER    # 默认数据库
#   MYAPP_DB    # 应用数据库
# 注意：不需要引号包围
database_name="DMSERVER"

# 达梦数据库导出工具路径 [必填，如果enabled=true]
# 说明：dexp工具的完整路径
# 格式：绝对路径
# 常见路径：
#   /opt/dmdbms/bin/dexp          # 标准安装路径
#   /home/dmdba/dmdbms/bin/dexp   # 用户安装路径
#   /usr/local/dm8/bin/dexp       # 自定义安装路径
#   /dm8/bin/dexp                 # Docker环境路径
# 注意：不需要引号包围
backup_tool_path="/opt/dmdbms/bin/dexp"

# 备份存储目录 [选填，默认：project_backup/SQL_Backup]
# 说明：数据库备份文件的存储目录，留空使用默认目录
# 格式：相对路径或绝对路径
# 示例：
#   backup_dir=""                    # 使用默认目录
#   backup_dir="custom_backup"       # 自定义相对路径
#   backup_dir="/opt/backups/dm"     # 自定义绝对路径
# 注意：必须用双引号包围
backup_dir=""

# 连接超时时间 [选填，默认：30]
# 说明：连接数据库的超时时间（秒）
# 格式：正整数
# 示例：
#   10    # 10秒超时
#   30    # 30秒超时
#   60    # 60秒超时
# 注意：不需要引号包围
connection_timeout=30

# 备份保留天数 [选填，默认：7]
# 说明：数据库备份文件的保留天数，0表示不自动清理
# 格式：正整数
# 示例：
#   3     # 保留3天
#   7     # 保留7天
#   30    # 保留30天
#   0     # 不自动清理
# 注意：不需要引号包围
backup_retention_days=7

# 备份参数 [选填]
# 说明：传递给dexp工具的额外参数
# 格式：字符串，多个参数用空格分隔
# 常用参数：
#   COMPRESS=Y       # 启用压缩（推荐）
#   GRANTS=Y         # 导出权限
#   INDEXES=Y        # 导出索引
#   TRIGGERS=Y       # 导出触发器
#   STATISTICS=Y     # 导出统计信息
#   ROWS=Y           # 导出数据行
#   OWNER=schema_name # 只导出指定模式
# 示例：
#   backup_params="COMPRESS=Y GRANTS=Y INDEXES=Y"
#   backup_params="COMPRESS=Y OWNER=APP_SCHEMA"
# 注意：必须用双引号包围
backup_params="COMPRESS=Y GRANTS=Y INDEXES=Y TRIGGERS=Y"`
}

// getDatabaseMysqlConfigTemplate 获取MySQL数据库备份配置模板
func (s *ProjectService) getDatabaseMysqlConfigTemplate() string {
	return `[database-mysql]

# 是否启用MySQL数据库备份 [选填，默认：false]
# 说明：是否启用MySQL数据库的自动备份功能
# 可选值：true, false
# 注意：启用后必须配置下面的连接信息
enabled=false

# 数据库主机地址 [必填，如果enabled=true]
# 说明：MySQL数据库服务器的IP地址或域名
# 格式：IP地址或域名，不包含端口号
# 示例：
#   localhost         # 本地数据库
#   192.168.1.100     # 内网数据库
#   mysql.example.com # 域名访问
# 注意：不需要引号包围
host="localhost"

# 数据库端口 [选填，默认：3306]
# 说明：MySQL数据库的端口号
# 格式：正整数，范围1-65535
# 示例：
#   3306    # MySQL默认端口
#   3307    # 自定义端口
# 注意：不需要引号包围
port=3306

# 数据库用户名 [必填，如果enabled=true]
# 说明：连接MySQL数据库的用户名
# 格式：字符串
# 示例：
#   root      # 管理员用户
#   app_user  # 应用用户
# 注意：不需要引号包围
username="root"

# 数据库密码 [必填，如果enabled=true]
# 说明：连接MySQL数据库的密码
# 格式：字符串，可以为空
# 示例：
#   password=""       # 空密码
#   password="123456" # 有密码
# 注意：必须用双引号包围，即使是空密码
password=""

# 数据库名称 [必填，如果enabled=true]
# 说明：要备份的数据库名称，留空表示备份所有数据库
# 格式：字符串
# 示例：
#   database_name=""        # 备份所有数据库
#   database_name="myapp"   # 只备份myapp数据库
# 注意：必须用双引号包围
database_name=""

# MySQL备份工具路径 [必填，如果enabled=true]
# 说明：mysqldump工具的完整路径
# 格式：绝对路径
# 常见路径：
#   /usr/bin/mysqldump              # Ubuntu/Debian标准路径
#   /usr/local/mysql/bin/mysqldump  # MySQL官方安装路径
#   /opt/mysql/bin/mysqldump        # 自定义安装路径
#   /usr/local/bin/mysqldump        # Homebrew安装路径(macOS)
# 注意：不需要引号包围
backup_tool_path="/usr/bin/mysqldump"

# 备份存储目录 [选填，默认：project_backup/SQL_Backup]
# 说明：数据库备份文件的存储目录，留空使用默认目录
# 格式：相对路径或绝对路径
# 示例：
#   backup_dir=""                      # 使用默认目录
#   backup_dir="mysql_backup"          # 自定义相对路径
#   backup_dir="/opt/backups/mysql"    # 自定义绝对路径
# 注意：必须用双引号包围
backup_dir=""

# 连接超时时间 [选填，默认：30]
# 说明：连接数据库的超时时间（秒）
# 格式：正整数
# 示例：
#   10    # 10秒超时
#   30    # 30秒超时
#   60    # 60秒超时
# 注意：不需要引号包围
connection_timeout=30

# 备份保留天数 [选填，默认：7]
# 说明：数据库备份文件的保留天数，0表示不自动清理
# 格式：正整数
# 示例：
#   3     # 保留3天
#   7     # 保留7天
#   30    # 保留30天
#   0     # 不自动清理
# 注意：不需要引号包围
backup_retention_days=7

# 备份参数 [选填]
# 说明：传递给mysqldump工具的额外参数
# 格式：字符串，多个参数用空格分隔
# 常用参数：
#   --single-transaction    # 一致性备份(InnoDB推荐)
#   --routines             # 备份存储过程和函数
#   --triggers             # 备份触发器
#   --events               # 备份事件
#   --lock-tables=false    # 不锁表
#   --quick                # 快速备份，逐行检索
#   --add-drop-database    # 添加DROP DATABASE语句
#   --complete-insert      # 使用完整的INSERT语句
# 示例：
#   backup_params="--single-transaction --routines --triggers"
#   backup_params="--quick --lock-tables=false --complete-insert"
# 注意：必须用双引号包围
backup_params="--single-transaction --routines --triggers --events --quick"`
}

// getDatabaseCommonConfigTemplate 获取数据库通用配置模板
func (s *ProjectService) getDatabaseCommonConfigTemplate() string {
	return `[database-common]

# 备份前是否测试连接 [选填，默认：true]
# 说明：在执行备份前先测试数据库连接是否正常
# 可选值：true, false
# 示例：
#   true    # 推荐，确保连接正常
#   false   # 跳过连接测试，直接备份
# 注意：不需要引号包围
test_connection_before_backup=true

# 是否压缩备份文件为ZIP [选填，默认：true]
# 说明：是否将备份文件压缩为ZIP格式以节省空间
# 可选值：true, false
# 注意：达梦.dmp和MySQL.sql文件都会被打包成ZIP压缩包
# 示例：
#   true    # 推荐，节省存储空间
#   false   # 不压缩，保持原始格式
# 注意：不需要引号包围
compress_backup=true

# 压缩级别 [选填，默认：6]
# 说明：ZIP压缩的压缩级别，范围1-9
# 格式：整数，1=最快压缩，9=最高压缩率，6=平衡
# 示例：
#   1     # 最快压缩，文件较大
#   6     # 平衡模式，推荐
#   9     # 最高压缩，耗时较长
# 注意：不需要引号包围
compression_level=6

# 是否启用并行备份 [选填，默认：false]
# 说明：是否启用并行备份以提高备份速度
# 可选值：true, false
# 注意：并行备份可能会增加数据库负载
# 示例：
#   true    # 启用并行备份，适合大型数据库
#   false   # 单线程备份，适合小型数据库
# 注意：不需要引号包围
parallel_backup=false`
}

// getFooterTemplate 获取配置文件脚部模板
func (s *ProjectService) getFooterTemplate() string {
	return `# 1. 必填参数：如果不填写，脚本将无法正常工作
# 2. 选填参数：如果不需要可以删除整行，或者设置为空值
# 3. 空值设置：如果要设置为空，请使用 parameter="" 格式
# 4. 引号使用：
#    - 字符串参数建议使用双引号包围
#    - 数字参数可以不用引号
#    - 布尔参数(true/false)不需要引号
# 5. 路径格式：所有路径必须使用Linux格式的绝对路径
# 6. 修改配置：修改配置后无需重启，脚本会自动读取最新配置
# 7. 配置验证：可以使用 app-manager config validate 命令验证配置
# 8. 配置示例：可以使用 app-manager config example 命令查看配置示例
#
# 如有问题，请参考文档或联系技术支持。`
}

// getDetailedConfigTemplate 获取详细的配置模板
func (s *ProjectService) getDetailedConfigTemplate() (string, error) {
	var sb strings.Builder

	// 文件头
	sb.WriteString(`# Java应用管理系统配置文件 (详细版)
# 本文件为INI格式配置文件，请按照要求填写各项参数
# 注意：带有#号的行为注释行，不会被程序读取
# 
# 配置说明：
# [必填] - 必须填写的配置项，不填写将导致程序无法正常运行
# [选填] - 可选配置项，可以留空或删除整行
# [默认] - 如果不填写，程序将使用的默认值
`)

	// 拼接各个配置段
	sb.WriteString(s.getConfigTemplateSection("全局配置", s.getGlobalConfigTemplate()))
	sb.WriteString(s.getConfigTemplateSection("Nacos配置中心", s.getNacosConfigTemplate()))
	sb.WriteString(s.getConfigTemplateSection("应用启动优先级配置", s.getApplicationsConfigTemplate()))
	sb.WriteString(s.getConfigTemplateSection("应用配置", s.getApplicationConfigTemplate()))
	sb.WriteString(s.getConfigTemplateSection("监控配置", s.getMonitoringConfigTemplate()))
	sb.WriteString(s.getConfigTemplateSection("备份配置", s.getBackupConfigTemplate()))
	sb.WriteString(s.getConfigTemplateSection("日志轮转配置", s.getLogrotateConfigTemplate()))
	sb.WriteString(s.getConfigTemplateSection("系统服务配置", s.getSystemdConfigTemplate()))

	if IncludeDatabaseConfigInInit {
		sb.WriteString(s.getConfigTemplateSection("达梦数据库备份配置", s.getDatabaseDmConfigTemplate()))
		sb.WriteString(s.getConfigTemplateSection("MySQL数据库备份配置", s.getDatabaseMysqlConfigTemplate()))
		sb.WriteString(s.getConfigTemplateSection("数据库通用配置", s.getDatabaseCommonConfigTemplate()))
	}

	// 文件尾部说明
	sb.WriteString("\n# ═══════════════════════════════════════════════════════════\n")
	sb.WriteString("#                        配置文件说明\n")
	sb.WriteString("# ═══════════════════════════════════════════════════════════\n")
	sb.WriteString(s.getFooterTemplate())

	return sb.String(), nil
}

// createDirectoryStructure 创建目录结构
func (ps *ProjectService) createDirectoryStructure(projectRoot string) error {
	directories := []string{
		"project_backend",
		"project_frontend",
		"project_backup",
		"project_update",
		"project_cache",
		".tmp",
		"project_backend/config",
		"project_backend/logs",
		"project_backend/pids",
		"project_backend/file_upload",
		"project_backend/rotate_logs",
	}

	for _, dir := range directories {
		dirPath := filepath.Join(projectRoot, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", dir, err)
		}

		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			logger.Info("创建目录: %s", dir)
		} else {
			logger.Warning("目录已存在: %s", dir)
		}
	}

	return nil
}

// createConfigFile 创建配置文件
func (s *ProjectService) createConfigFile(projectRoot string) error {
	configFileName := config.GetConfigFileName()
	configPath := filepath.Join(projectRoot, "project_backend", configFileName)

	logger.Info("创建配置文件: %s", configPath)

	// 更新配置
	s.config.Global.ProjectRoot = projectRoot

	// 创建INI格式的配置文件内容
	if err := s.createINIConfigFile(configPath); err != nil {
		return fmt.Errorf("创建配置文件失败: %w", err)
	}

	return nil
}

// createINIConfigFile 创建INI格式的配置文件
func (s *ProjectService) createINIConfigFile(configPath string) error {
	// 使用内嵌的详细模板
	templateContent, err := s.getDetailedConfigTemplate()
	if err != nil {
		return fmt.Errorf("获取配置模板失败: %w", err)
	}

	// 替换模板中的占位符
	content := strings.ReplaceAll(templateContent, "{{PROJECT_ROOT}}", s.config.Global.ProjectRoot)
	content = strings.ReplaceAll(content, "{{JAVA_PATH}}", s.config.Global.JavaPath)
	content = strings.ReplaceAll(content, "{{APP_TYPE}}", s.config.Application.AppType)
	content = strings.ReplaceAll(content, "{{JVM_PARAMS}}", s.config.Application.JVMParams)
	content = strings.ReplaceAll(content, "{{BACKUP_PREFIX}}", s.config.Backup.BackupPrefix)
	content = strings.ReplaceAll(content, "{{BACKUP_RETENTION_DAYS}}", fmt.Sprintf("%d", s.config.Backup.BackupRetentionDays))
	content = strings.ReplaceAll(content, "{{LOGROTATE_SIZE}}", s.config.Logrotate.LogrotateSize)
	content = strings.ReplaceAll(content, "{{LOGROTATE_CYCLE}}", s.config.Logrotate.LogrotateCycle)
	content = strings.ReplaceAll(content, "{{LOGROTATE_ROTATE}}", fmt.Sprintf("%d", s.config.Logrotate.LogrotateRotate))
	content = strings.ReplaceAll(content, "{{SERVICE_RESTART}}", s.config.Systemd.ServiceRestart)
	content = strings.ReplaceAll(content, "{{SERVICE_RESTART_SEC}}", fmt.Sprintf("%d", s.config.Systemd.ServiceRestartSec))
	content = strings.ReplaceAll(content, "{{SERVICE_TIMEOUT_START_SEC}}", fmt.Sprintf("%d", s.config.Systemd.ServiceTimeoutStartSec))
	content = strings.ReplaceAll(content, "{{SERVICE_TIMEOUT_STOP_SEC}}", fmt.Sprintf("%d", s.config.Systemd.ServiceTimeoutStopSec))

	return os.WriteFile(configPath, []byte(content), 0644)
}

// printDirectoryStructure 打印目录结构
func (s *ProjectService) printDirectoryStructure(projectRoot string) {
	logger.Println("")
	logger.Info("目录结构如下：")

	configFileName := config.GetConfigFileName()
	structure := fmt.Sprintf(`%s/
├── project_backend/     # 后端应用目录
│   ├── config/          # 配置文件目录
│   ├── logs/            # 日志文件目录
│   ├── pids/            # PID文件目录
│   ├── file_upload/     # 文件上传目录
│   ├── rotate_logs/     # 日志轮转目录
│   └── %s # 管理配置文件
├── project_frontend/    # 前端应用目录
├── project_backup/      # 备份文件目录
├── project_update/      # 更新文件目录
└── project_cache/       # 缓存文件目录`, projectRoot, configFileName)

	logger.Printf("\033[36m%s\033[0m\n", structure)
}

// printUsageInstructions 打印使用说明
func (s *ProjectService) printUsageInstructions(projectRoot string) {
	logger.Println("")
	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Printf("\033[32m\033[1m  🎉 初始化成功！配置文件已创建 🎉\033[0m\n")
	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Println("")

	logger.Info("接下来请执行以下命令开始使用：")
	logger.Println("")

	logger.Printf("\033[33m\033[1m  🚀 进入工作目录：\033[0m\n")
	logger.Printf("     cd %s/project_backend\n", projectRoot)
	logger.Println("")

	programName := config.GetProgramName()
	logger.Printf("\033[33m\033[1m  📖 查看帮助信息：\033[0m\n")
	logger.Printf("     %s help\n", programName)
	logger.Println("")

	logger.Printf("\033[33m\033[1m  🔧 常用操作命令：\033[0m\n")
	logger.Printf("     %s start              # 启动应用\n", programName)
	logger.Printf("     %s status             # 查看状态\n", programName)
	logger.Printf("     %s logs <应用名>      # 查看日志\n", programName)
	logger.Println("")

	logger.Warning("请先将您的JAR/WAR文件复制到project_backend目录，然后开始使用应用管理")
}

// UpdatePrepare 更新准备
func (s *ProjectService) UpdatePrepare(autoBackup bool) error {
	logger.Title("应用更新准备")

	projectRoot := config.GetProjectRoot()
	updateDir := filepath.Join(projectRoot, "project_update")

	// 确保更新目录存在
	if err := os.MkdirAll(updateDir, 0755); err != nil {
		return fmt.Errorf("创建更新目录失败: %w", err)
	}

	// 创建时间目录
	timeDir := filepath.Join(updateDir, time.Now().Format("20060102_150405"))
	if err := os.MkdirAll(timeDir, 0755); err != nil {
		return fmt.Errorf("创建时间目录失败: %w", err)
	}

	// 在时间目录下创建TempUnzip目录
	tempUnzipDir := filepath.Join(timeDir, "TempUnzip")
	if err := os.MkdirAll(tempUnzipDir, 0755); err != nil {
		return fmt.Errorf("创建TempUnzip目录失败: %w", err)
	}

	logger.Info("创建更新目录结构:")
	logger.Info("  更新根目录: %s", updateDir)
	logger.Info("  时间目录: %s", timeDir)
	logger.Info("  临时解压目录: %s", tempUnzipDir)

	// 创建更新前备份
	if autoBackup || s.promptForBackup() {
		backupService := NewBackupService()
		backupPrefix := s.config.Backup.BackupPrefix
		if backupPrefix == "" {
			backupPrefix = "update_backup" // 如果未配置，则使用默认值
		}
		backupName := backupPrefix

		logger.Info("创建更新前备份...")
		if err := backupService.BackupProject(backupName, "更新前自动备份"); err != nil {
			logger.Warning("备份失败，但继续更新准备: %v", err)
		} else {
			logger.Success("备份创建成功")
		}
	}

	// 检查运行中的应用并提示用户手动停止
	appService := NewApplicationService()
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}

	runningApps := apps.Running()
	if len(runningApps) > 0 {
		logger.Warning("检测到以下应用正在运行：")
		for _, app := range runningApps {
			logger.Printf("  • %s (PID: %d)", app.Name, app.PID)
		}
		logger.Println("")
		logger.Warning("更新应用后，请记得使用 'app-manager restart' 或 'app-manager start' 命令来重启或启动应用。")
		logger.Println("")
	}

	// 显示更新流程说明
	s.printUpdateInstructions(timeDir, tempUnzipDir)

	return nil
}

// printUpdateInstructions 打印更新流程说明
func (s *ProjectService) printUpdateInstructions(timeDir, tempUnzipDir string) {
	logger.Success("更新准备完成！")
	logger.Println("")
	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Printf("\033[32m\033[1m  🚀 更新流程说明 🚀\033[0m\n")
	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Println("")

	logger.Info("更新步骤：")
	logger.Println("")

	logger.Printf("\033[33m\033[1m  步骤1: 传输更新文件\033[0m\n")
	logger.Printf("     使用scp传输更新包到服务器：\n")
	logger.Printf("     \033[37mscp update.zip user@server:%s/\033[0m\n", timeDir)
	logger.Println("")

	logger.Printf("\033[33m\033[1m  步骤2: 解压更新包\033[0m\n")
	logger.Printf("     解压ZIP包到临时目录：\n")
	logger.Printf("     \033[37mcd %s\033[0m\n", timeDir)
	logger.Printf("     \033[37munzip update.zip -d TempUnzip/\033[0m\n")
	logger.Println("")

	logger.Printf("\033[33m\033[1m  步骤3: 备份当前版本\033[0m\n")
	logger.Printf("     当前版本已自动备份，如需手动备份：\n")
	logger.Printf("     \033[37m%s backup manual_backup_before_update\033[0m\n", config.GetProgramName())
	logger.Println("")

	logger.Printf("\033[33m\033[1m  步骤4: 更新应用文件\033[0m\n")
	logger.Printf("     将新文件复制到应用目录：\n")
	logger.Printf("     \033[37mcp TempUnzip/* %s/project_backend/\033[0m\n", config.GetProjectRoot())
	logger.Println("")

	logger.Printf("\033[33m\033[1m  步骤5: 启动应用\033[0m\n")
	logger.Printf("     启动更新后的应用：\n")
	logger.Printf("     \033[37m%s start\033[0m\n", config.GetProgramName())
	logger.Println("")

	logger.Printf("\033[33m\033[1m  步骤6: 验证更新\033[0m\n")
	logger.Printf("     检查应用状态和功能：\n")
	logger.Printf("     \033[37m%s status\033[0m\n", config.GetProgramName())
	logger.Printf("     \033[37m%s logs <应用名>\033[0m\n", config.GetProgramName())
	logger.Println("")

	logger.Warning("注意事项：")
	logger.Printf("  • 更新前已自动创建备份，如有问题可回滚\n")
	logger.Printf("  • 建议在维护窗口期间执行更新操作\n")
	logger.Printf("  • 更新完成后请验证所有功能是否正常\n")
	logger.Printf("  • 如遇问题，可使用备份文件快速恢复\n")
	logger.Println("")

	logger.Info("目录信息：")
	logger.Printf("  时间目录: \033[33m%s\033[0m\n", timeDir)
	logger.Printf("  解压目录: \033[33m%s\033[0m\n", tempUnzipDir)
	logger.Printf("  应用目录: \033[33m%s/project_backend\033[0m\n", config.GetProjectRoot())
}

// promptForBackup 提示用户是否需要备份
func (s *ProjectService) promptForBackup() bool {
	logger.Println("")
	logger.Question("在更新前是否需要创建备份？(yes/no) [yes]: ")

	var response string
	fmt.Scanln(&response)

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "yes" || response == "y" || response == ""
}
