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
- **日志留痕**: 每次启动按时间戳新建日志，历史日志不再被覆盖
- **自动归档**: stop/restart 时把当前日志改名为 `应用名-yyyy-MM-dd-HHmmss-停止.log`
- **日志查看**: 支持实时跟踪、历史运行周期回看
- **日志轮转**: 自动日志文件轮转和清理
- **彩色输出**: 根据日志级别自动着色显示

### 🌐 网络监控
- **端口连接统计**: 实时监控应用端口的连接数和状态
- **IP去重统计**: 自动统计连接到各端口的唯一IP地址数量
- **连接详情**: 显示端口监听状态、连接数、唯一IP数等详细信息

### 📈 资源监控
- **CPU两口径显示**: `ps` 的 pcpu 是"占单核百分比"，多核上可超过 100%；
  现在同时给出单核口径与整机口径（除以核数，与 top 一致），不再被截断到 100%
- **容器感知**: 核数优先取 cgroup 限额（v1/v2），容器里不会用宿主机核数错误归一化
- **端口连接统计**: 实时监控应用端口的连接数和状态

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

# 编译（推荐：脚本会把当前提交号/分支注入二进制）
pwsh ./build-linux.ps1                       # 产出 dist/app-manager-linux-{amd64,arm64} + BUILD-INFO.txt
pwsh ./build-windows.ps1                     # 产出 dist/app-manager.exe
pwsh ./build-linux.ps1 -Version v1.0.2       # 手动指定版本号

# 或者手动编译（不带 git 信息也能跑，只是 status/version 里看不到提交号）
go build -o app-manager main.go

# 安装到系统路径（可选）
sudo mv app-manager /usr/local/bin/
```

**为什么要用脚本编译**：脚本会把 `git rev-parse` 得到的提交号、分支、是否有未提交改动注入二进制。
部署到服务器后，`app-manager status` / `app-manager version` 就能直接显示这份二进制来自哪个提交，
再也不用靠"这个包是不是最新的"来猜。

```bash
$ app-manager version
app-manager version v1.0.2
Build:   10980d9 (feat/log-retention@10980d9, 工作区有未提交改动, 构建于 2026-09-16T16:36:40)
代码目录: /opt/party/project_backend
当前仓库: feat/log-retention@10980d9 [干净] 提交于 2026-09-16 16:33:08
注意: 二进制由 10980d9 构建，当前目录是 3f2a1b0 —— 建议重新编译后再操作   # 版本漂移时才出现
```

`app-manager status` 的系统信息段同样会打印这两行（不注入 git 信息时会提示如何构建）。

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
| `logs` | 查看应用日志（当前 / 历史运行周期） | `app-manager logs myapp -f` |

### 日志留痕

日志文件按“运行周期”留痕，文件名同时记录**启动时刻**和**停止时刻**，任何一次启动的输出都不会被覆盖。
目录里**只有普通文件**，软链接只在应用运行期间存在，停止即清理：

```
运行中：
project_backend/logs/
├── party-auth.log                                                  # 软链接 → 本次会话日志（仅运行期存在）
├── party-auth-2026-09-16-161016启动-2026-09-16-161142停止.log      # 第 1 次运行（已归档）
└── party-auth-2026-09-16-161117启动.log                           # 第 2 次运行（正在写入）

已停止：
project_backend/logs/
├── party-auth-2026-09-16-161016启动-2026-09-16-161142停止.log      # 第 1 次运行
└── party-auth-2026-09-16-161117启动-2026-09-16-161230停止.log      # 第 2 次运行（链接已清理）
```

- 每次 `start` 新建 `<应用名>-<启动时间>启动.log`（不会清空旧日志）
- `stop` / `restart` 时改名为 `<应用名>-<启动时间>启动-<停止时间>停止.log`，并删除 `<应用名>.log` 链接
- 停止时间取内容最后写入时刻，因此文件名能直接看出这次运行活了多久；同秒停止时顺延 1 秒避免歧义
- 归档永远作用于**真实文件**，不会对软链接做 `rename`（否则会把链接变成归档文件、滚出 `-停止(2).log` 这类链子）
- 同一次停止被重复触发（如 daemon 优雅退出 + `stop` 命令）时幂等：已归档文件不会被二次改名
- 程序被 `kill -9`、断电等异常情况下，下次 `start` 会自动补归档上一次的日志
- 旧命名（`<应用名>-<启动时间>.log` / `…-停止.log`）与旧版固定命名 `<应用名>.log` 都会被自动识别、归档迁移
- `<应用名>.log` 链接缺失时（应用已停止），`logs` / `status` 自动回退到最近一次运行的日志
- **保留份数**：每个应用最多保留最近 7 份归档日志（`application.log_keep_count`，见下方配置说明），
  超出后自动删除最旧的；正在写入的那份不计数、也不会被删

```bash
# 查看当前日志并实时跟踪
app-manager logs party-auth -f

# 列出该应用所有运行周期的日志
app-manager logs party-auth --history

# 查看倒数第 2 次运行的日志（序号见 --history 输出）
app-manager logs party-auth --tail 2 -n 500

# 日志目录诊断：逐个列出文件/软链接、归档状态、悬空链接，并自检软链接语义
app-manager status --debug-logs
app-manager status --debug-logs party-auth    # 只看某个应用
```

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
| `--history` | 列出该应用所有运行周期的日志（日志命令） |
| `--tail` | 查看指定序号的运行周期日志，1 为最新（日志命令） |
| `--debug-logs` | 打印日志目录诊断信息：文件/软链接、归档状态、软链接语义自检 |

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
# 说明：
#   - 不需要 http:// 前缀
#   - 未指定端口时自动添加默认端口 :8848
#   - 集群模式多个地址用英文逗号分隔
#   - 示例：nacos_server="192.168.1.100:8848" 或 "192.168.1.100"
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
# 每个应用保留的归档日志份数 [选填，默认：7]
# 正整数=保留最近 N 份；0=不留归档（每次停止后清理）；负数=不限制
# 超出份数后自动删除最旧的归档日志，正在写入的那份不计数也不会被删
log_keep_count=7

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