# App Manager Go v1

一个用于管理Java应用程序的命令行工具，基于Go语言开发，是原Shell脚本版本的现代化重写。

![App Manager Screenshot](https://i.imgur.com/example.png) <!-- 占位符，稍后替换为真实URL -->

## 功能特性

### 🚀 应用生命周期管理
- **启动应用**: 支持单个或批量启动Java应用
- **停止应用**: 优雅停止或强制终止应用
- **重启应用**: 安全重启应用程序
- **状态查询**: 实时查看应用运行状态、资源使用情况

### 📁 项目管理
- **项目初始化**: 自动创建标准化目录结构
- **配置管理**: 统一的YAML配置文件管理
- **更新准备**: 安全的应用更新流程

### 📋 日志管理
- **日志查看**: 支持实时跟踪和历史日志查看
- **日志轮转**: 自动日志文件轮转和清理
- **彩色输出**: 根据日志级别自动着色显示

### 🌐 网络监控
- **端口连接统计**: 实时监控应用端口的连接数和状态
- **IP去重统计**: 自动统计连接到各端口的唯一IP地址数量
- **连接详情**: 显示端口监听状态、连接数、唯一IP数等详细信息

### 💾 备份功能
- **项目备份**: 完整的项目文件备份
- **自动清理**: 根据配置自动清理过期备份
- **压缩存储**: ZIP格式压缩存储，节省空间

### ⚙️ 系统集成
- **Systemd服务**: 支持创建和管理systemd服务
- **开机自启**: 配置应用开机自动启动
- **进程监控**: 实时监控应用进程状态

## 安装

### 从源码编译

```bash
# 克隆项目
git clone <repository-url>
cd app-manager-go-v1

# 编译
go build -o app-manager main.go

# 安装到系统路径（可选）
sudo mv app-manager /usr/local/bin/
```

### 下载预编译版本

从[Releases页面](releases)下载适合您系统的预编译版本。

## 快速开始

### 1. 初始化项目

```bash
# 在当前目录初始化项目
app-manager init

# 或在指定目录初始化
app-manager init /opt/myproject
```

初始化后的目录结构：
```
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
```

### 2. 部署应用

将您的Java应用文件（.jar或.war）复制到`project_backend`目录：

```bash
cp myapp.jar project_backend/
```

### 3. 管理应用

```bash
# 查看所有应用状态
app-manager status

# 启动应用
app-manager start myapp

# 查看应用日志
app-manager logs myapp -f

# 停止应用
app-manager stop myapp

# 重启应用
app-manager restart myapp
```

## 命令参考

### 基础命令

| 命令 | 描述 | 示例 |
|------|------|------|
| `init` | 初始化项目目录结构 | `app-manager init` |
| `start` | 启动应用 | `app-manager start myapp` |
| `stop` | 停止应用 | `app-manager stop myapp` |
| `restart` | 重启应用 | `app-manager restart myapp` |
| `status` | 查看应用状态（支持交互模式，包含端口连接和IP去重统计） | `app-manager status` |
| `logs` | 查看应用日志 | `app-manager logs myapp -f` |

### 高级命令

| 命令 | 描述 | 示例 |
|------|------|------|
| `backup` | 备份项目文件 | `app-manager backup --name release-v1.0` |
| `update` | 准备应用更新 | `app-manager update` |
| `logrotate` | 执行日志轮转 | `app-manager logrotate` |
| `service` | 管理systemd服务 | `app-manager service create myapp` |
| `version` | 显示版本信息 | `app-manager version` |

### 常用选项

| 选项 | 描述 |
|------|------|
| `--config` | 指定配置文件路径 |
| `--verbose, -v` | 详细输出模式 |
| `--all, -a` | 应用于所有应用 |
| `--force, -f` | 强制执行操作 |
| `--follow, -f` | 实时跟踪（日志命令） |
| `--lines, -n` | 指定行数（日志命令） |

## 配置文件

配置文件`.app-manager.ini`包含以下主要配置项：

```ini
[global]
# 项目根目录 [必填]
project_root="/path/to/project"
# Java执行路径 [选填，默认：java]
java_path="java"

[nacos]
# Nacos服务器地址 [选填]
nacos_server=""
# Nacos命名空间 [选填]
nacos_namespace=""
# Nacos鉴权用户名 [选填，适用于 Nacos 2.5.1+ 开启鉴权]
nacos_auth_name=""
# Nacos鉴权密码 [选填，适用于 Nacos 2.5.1+ 开启鉴权]
nacos_auth_password=""

[application]
# 应用程序类型 [选填，默认：mixed]
# 可选值：jar, war, mixed
app_type="mixed"
# JVM参数 [选填]
jvm_params="-Xms512m -Xmx1024m -XX:+UseG1GC"
# 启动间隔时间(秒) [选填，默认：0]
startup_delay=0
# 默认优先级 [选填，默认：100]
default_priority=100

[applications]
# 单个应用的特殊配置，格式为：appName.priority 和 appName.startup_delay
# 例如:
# my-app-1.priority=10
# my-app-1.startup_delay=5
# my-app-2.priority=20

[monitoring]
# 启用磁盘I/O监控 [选填，默认：false]
enable_disk_io=false
# 启用网络监控 [选填，默认：false]
enable_network=false
# 启用端口状态监控 [选填，默认：false]
enable_port_stats=false
# 监控端口列表(逗号分隔) [选填]
monitor_ports="8080,8081,8082"

[backup]
# 备份文件前缀 [选填，默认：backup]
backup_prefix="backup"
# 备份保留天数 [选填，默认：7]
backup_retention_days=7

[logrotate]
# 日志文件大小阈值 [选填，默认：10MB]
logrotate_size="10MB"
# 日志轮转周期 [选填，默认：daily]
logrotate_cycle="daily"
# 保留的日志文件数量 [选填，默认：5]
logrotate_rotate=5

[systemd]
# 服务重启策略 [选填，默认：always]
service_restart="always"
# 服务重启间隔(秒) [选填，默认：10]
service_restart_sec=10
# 服务启动超时时间(秒) [选填，默认：60]
service_timeout_start_sec=60
# 服务停止超时时间(秒) [选填，默认：30]
service_timeout_stop_sec=30
```

## 使用场景

### 开发环境
- 快速启动/停止开发中的Java应用
- 实时查看应用日志进行调试
- 管理多个微服务应用

### 测试环境
- 批量管理测试应用
- 自动化测试前的环境准备
- 测试数据备份和恢复

### 生产环境
- 应用的安全启停和重启
- 定期备份和日志轮转
- 系统服务集成和监控

## 最佳实践

### 1. 配置管理
- 为不同环境维护不同的配置文件
- 使用环境变量覆盖敏感配置
- 定期备份配置文件

### 2. 日志管理
- 配置合适的日志轮转策略
- 定期清理过期日志文件
- 使用日志聚合工具收集日志

### 3. 备份策略
- 在重要操作前创建备份
- 定期验证备份文件的完整性
- 制定备份恢复流程

### 4. 监控告警
- 结合systemd服务实现自动重启
- 配置应用健康检查
- 设置资源使用告警

## 故障排除

### 常见问题

**Q: 应用启动失败**
```bash
# 检查应用状态
app-manager status myapp

# 查看详细日志
app-manager logs myapp -n 100

# 检查配置文件
cat project_backend/config/myapp.properties
```

**Q: 日志文件过大**
```bash
# 手动执行日志轮转
app-manager logrotate

# 检查轮转配置
grep -A 5 "logrotate:" .app-manager.yaml
```

**Q: 备份失败**
```bash
# 检查磁盘空间
df -h

# 检查备份目录权限
ls -la project_backup/
```

### 调试模式

使用`--verbose`选项获取详细输出：

```bash
app-manager --verbose start myapp
```

## 贡献

欢迎提交Issue和Pull Request来改进这个项目。

### 开发环境设置

```bash
# 克隆项目
git clone <repository-url>
cd app-manager-go-v1

# 安装依赖
go mod tidy

# 运行测试
go test ./...

# 构建
go build -o app-manager main.go
```

## 许可证

本项目采用MIT许可证，详见[LICENSE](LICENSE)文件。

## 更新日志

### v1.0.1 (2025-01-14)
- 🌐 **新增端口连接IP去重功能**
  - 在端口监控表格中新增"唯一IP"列，显示连接到各端口的去重IP数量
  - 支持IPv4和IPv6地址的智能解析和去重统计
  - 增强`ss`和`netstat`命令解析，提取远程IP地址进行统计
  - 优化端口描述信息，包含唯一IP数量显示

### v1.0.0 (2025-01-14)
- 🎉 初始版本发布
- ✨ 完整的Java应用生命周期管理（启动、停止、重启、状态查看）
- 🚀 项目初始化和INI格式配置管理
- 📊 交互式状态查看，支持实时系统信息显示
- 📝 日志查看和自动轮转功能
- 💾 项目备份和恢复功能
- 🔧 Systemd服务集成和管理
- 🎯 应用启动优先级控制
- 📈 系统监控功能（CPU、内存、磁盘、网络）
- 🌐 Nacos配置中心集成支持
- 🎨 彩色日志输出和用户友好的界面
- 🔒 安全的进程管理和PID文件处理

## 支持

如果您在使用过程中遇到问题，请：

1. 查看本文档的故障排除部分
2. 搜索已有的[Issues](issues)
3. 创建新的Issue并提供详细信息

---

**注意**: 本工具主要针对Linux环境设计，部分功能（如systemd服务）在Windows环境下不可用。