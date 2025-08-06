# 测试应用 (Test App)

这是一个用于测试 app-manager 端口检测功能的简单 Spring Boot 应用。

## 功能特性

- 占用端口 8888
- 提供简单的 REST API
- 包含健康检查端点
- 启动时输出日志信息

## 快速开始

### 1. 构建应用

```bash
# Windows
build.bat

# 或者使用 Maven 命令
mvn clean package -DskipTests
```

### 2. 启动应用

```bash
# Windows
start.bat

# 或者直接运行
java -jar target/test-app.jar
```

### 3. 测试访问

- 主页: http://localhost:8888/test-app
- 健康检查: http://localhost:8888/test-app/actuator/health
- 应用信息: http://localhost:8888/test-app/info

## 测试 app-manager

启动此应用后，可以使用 app-manager 来检测端口占用：

```bash
# 检查应用状态（应该能检测到端口 8888）
./app-manager.exe status

# 或者直接检查端口
netstat -ano | findstr :8888
```

## 应用配置

- **端口**: 8888
- **上下文路径**: /test-app
- **JVM 最小内存**: 默认
- **日志级别**: INFO

## 文件结构

```
test-app/
├── src/main/java/com/test/
│   └── TestApplication.java          # 主应用类
├── src/main/resources/
│   └── application.yml               # 应用配置
├── pom.xml                          # Maven 配置
├── build.bat                        # 构建脚本
├── start.bat                        # 启动脚本
└── README.md                        # 说明文档
```

## 注意事项

1. 确保端口 8888 未被其他应用占用
2. 需要安装 Java 8 或更高版本
3. 需要安装 Maven（如果使用 mvn 命令）
4. 应用启动后会持续运行，按 Ctrl+C 停止