@echo off
echo ===== 启动测试应用 =====
echo.

if not exist "target\test-app.jar" (
    echo ✗ 找不到JAR文件！
    echo 请先运行 build.bat 构建应用
    pause
    exit /b 1
)

echo 启动应用...
echo 端口: 8888
echo 访问地址: http://localhost:8888/test-app
echo 按 Ctrl+C 停止应用
echo.

java -jar target\test-app.jar