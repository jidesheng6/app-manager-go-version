@echo off
echo ===== 构建测试应用 =====
echo.

echo 1. 清理之前的构建...
call mvn clean

echo.
echo 2. 编译并打包应用...
call mvn package -DskipTests

echo.
echo 3. 检查构建结果...
if exist "target\test-app.jar" (
    echo ✓ 构建成功！
    echo 生成的JAR文件: target\test-app.jar
    echo.
    echo 使用方法:
    echo   启动应用: java -jar target\test-app.jar
    echo   访问地址: http://localhost:8888/test-app
    echo   健康检查: http://localhost:8888/test-app/actuator/health
) else (
    echo ✗ 构建失败！
    echo 请检查Maven配置和网络连接
)

echo.
echo ===== 构建完成 =====
pause