#!/bin/bash

# App Manager Go v1 安装脚本

set -e

# 配置
REPO_URL="https://github.com/your-org/app-manager-go-v1"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/app-manager"
SERVICE_DIR="/etc/systemd/system"
BINARY_NAME="app-manager"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 打印函数
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查是否为root用户
check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root"
        exit 1
    fi
}

# 检测系统架构
detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            print_error "Unsupported architecture: $arch"
            exit 1
            ;;
    esac
}

# 检测操作系统
detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        echo "linux"
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        echo "darwin"
    else
        print_error "Unsupported operating system: $OSTYPE"
        exit 1
    fi
}

# 获取最新版本
get_latest_version() {
    # 这里应该从GitHub API获取最新版本
    # 暂时使用固定版本
    echo "v1.0.0"
}

# 下载二进制文件
download_binary() {
    local version=$1
    local os=$2
    local arch=$3
    local binary_url="${REPO_URL}/releases/download/${version}/${BINARY_NAME}-${os}-${arch}"
    
    if [[ "$os" == "darwin" ]]; then
        binary_url="${binary_url}.tar.gz"
    else
        binary_url="${binary_url}.tar.gz"
    fi
    
    print_info "Downloading $BINARY_NAME $version for $os/$arch..."
    
    local temp_dir=$(mktemp -d)
    local temp_file="$temp_dir/${BINARY_NAME}.tar.gz"
    
    if command -v curl >/dev/null 2>&1; then
        curl -L -o "$temp_file" "$binary_url"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "$temp_file" "$binary_url"
    else
        print_error "Neither curl nor wget is available"
        exit 1
    fi
    
    # 解压文件
    tar -xzf "$temp_file" -C "$temp_dir"
    
    # 移动二进制文件
    local binary_file="$temp_dir/${BINARY_NAME}-${os}-${arch}"
    if [[ ! -f "$binary_file" ]]; then
        binary_file="$temp_dir/$BINARY_NAME"
    fi
    
    if [[ -f "$binary_file" ]]; then
        mv "$binary_file" "$INSTALL_DIR/$BINARY_NAME"
        chmod +x "$INSTALL_DIR/$BINARY_NAME"
        print_success "Binary installed to $INSTALL_DIR/$BINARY_NAME"
    else
        print_error "Binary file not found in archive"
        exit 1
    fi
    
    # 清理临时文件
    rm -rf "$temp_dir"
}

# 创建配置目录
create_config_dir() {
    print_info "Creating configuration directory..."
    mkdir -p "$CONFIG_DIR"
    
    # 创建默认配置文件
    cat > "$CONFIG_DIR/config.yaml" << 'EOF'
# App Manager 配置文件

# 项目配置
project:
  # 项目根目录
  root_dir: "/opt/app-manager"
  
  # 后端应用目录
  backend_dir: "project_backend"
  
  # 前端应用目录
  frontend_dir: "project_frontend"
  
  # 备份目录
  backup_dir: "project_backup"
  
  # 更新目录
  update_dir: "project_update"
  
  # 缓存目录
  cache_dir: "project_cache"

# Java 配置
java:
  # Java 可执行文件路径
  java_path: "/usr/bin/java"
  
  # 默认 JVM 参数
  default_jvm_args: "-Xms512m -Xmx1024m -XX:+UseG1GC"

# 日志配置
logging:
  # 日志级别
  level: "info"
  
  # 日志文件路径
  file: "/var/log/app-manager/app-manager.log"
  
  # 日志轮转配置
  rotation:
    # 最大文件大小
    max_size: "100MB"
    
    # 保留文件数量
    max_files: 10
    
    # 轮转周期
    period: "daily"

# 备份配置
backup:
  # 自动备份
  auto_backup: true
  
  # 备份保留天数
  retention_days: 30
  
  # 备份压缩
  compression: true

# 监控配置
monitoring:
  # 启用监控
  enabled: true
  
  # 检查间隔（秒）
  interval: 30
  
  # 内存使用率阈值
  memory_threshold: 80
  
  # CPU使用率阈值
  cpu_threshold: 80
EOF
    
    print_success "Configuration directory created at $CONFIG_DIR"
}

# 创建systemd服务
create_systemd_service() {
    if [[ ! -d "$SERVICE_DIR" ]]; then
        print_warning "Systemd not available, skipping service creation"
        return
    fi
    
    print_info "Creating systemd service..."
    
    cat > "$SERVICE_DIR/app-manager.service" << EOF
[Unit]
Description=App Manager Go v1
Documentation=https://github.com/your-org/app-manager-go-v1
After=network.target

[Service]
Type=simple
User=root
Group=root
ExecStart=$INSTALL_DIR/$BINARY_NAME daemon
ExecReload=/bin/kill -HUP \$MAINPID
KillMode=process
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    print_success "Systemd service created"
}

# 创建日志目录
create_log_dir() {
    print_info "Creating log directory..."
    mkdir -p "/var/log/app-manager"
    print_success "Log directory created"
}

# 创建工作目录
create_work_dir() {
    print_info "Creating work directory..."
    mkdir -p "/opt/app-manager"
    print_success "Work directory created at /opt/app-manager"
}

# 验证安装
verify_installation() {
    print_info "Verifying installation..."
    
    if [[ -f "$INSTALL_DIR/$BINARY_NAME" ]]; then
        local version=$("$INSTALL_DIR/$BINARY_NAME" version --short 2>/dev/null || echo "unknown")
        print_success "App Manager installed successfully (version: $version)"
    else
        print_error "Installation verification failed"
        exit 1
    fi
}

# 显示安装后信息
show_post_install_info() {
    echo ""
    print_success "Installation completed successfully!"
    echo ""
    echo "Next steps:"
    echo "1. Initialize a project: $BINARY_NAME init /path/to/project"
    echo "2. Configure the application: edit $CONFIG_DIR/config.yaml"
    echo "3. Start managing applications: $BINARY_NAME --help"
    echo ""
    echo "For systemd integration:"
    echo "- Enable service: systemctl enable app-manager"
    echo "- Start service: systemctl start app-manager"
    echo "- Check status: systemctl status app-manager"
    echo ""
    echo "Documentation: https://github.com/your-org/app-manager-go-v1"
}

# 主安装函数
main() {
    print_info "Starting App Manager Go v1 installation..."
    
    # 检查权限
    check_root
    
    # 检测系统信息
    local os=$(detect_os)
    local arch=$(detect_arch)
    local version=$(get_latest_version)
    
    print_info "Detected system: $os/$arch"
    print_info "Installing version: $version"
    
    # 执行安装步骤
    download_binary "$version" "$os" "$arch"
    create_config_dir
    create_log_dir
    create_work_dir
    create_systemd_service
    verify_installation
    show_post_install_info
}

# 卸载函数
uninstall() {
    print_info "Uninstalling App Manager Go v1..."
    
    # 停止服务
    if systemctl is-active --quiet app-manager 2>/dev/null; then
        systemctl stop app-manager
        print_info "Service stopped"
    fi
    
    # 禁用服务
    if systemctl is-enabled --quiet app-manager 2>/dev/null; then
        systemctl disable app-manager
        print_info "Service disabled"
    fi
    
    # 删除文件
    rm -f "$INSTALL_DIR/$BINARY_NAME"
    rm -f "$SERVICE_DIR/app-manager.service"
    rm -rf "$CONFIG_DIR"
    rm -rf "/var/log/app-manager"
    
    # 重新加载systemd
    if command -v systemctl >/dev/null 2>&1; then
        systemctl daemon-reload
    fi
    
    print_success "App Manager Go v1 uninstalled successfully"
}

# 处理命令行参数
case "${1:-install}" in
    "install")
        main
        ;;
    "uninstall")
        check_root
        uninstall
        ;;
    *)
        echo "Usage: $0 [install|uninstall]"
        echo "  install   - Install App Manager Go v1 (default)"
        echo "  uninstall - Uninstall App Manager Go v1"
        exit 1
        ;;
esac