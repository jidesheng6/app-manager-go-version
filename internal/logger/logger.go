package logger

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fatih/color"
)

// Logger 日志记录器
type Logger struct {
	infoColor    *color.Color
	successColor *color.Color
	warningColor *color.Color
	errorColor   *color.Color
	titleColor   *color.Color
}

var (
	// DefaultLogger 默认日志记录器
	DefaultLogger *Logger
)

// Init 初始化日志系统
func Init() {
	DefaultLogger = &Logger{
		infoColor:    color.New(color.FgBlue, color.Bold),
		successColor: color.New(color.FgGreen, color.Bold),
		warningColor: color.New(color.FgYellow, color.Bold),
		errorColor:   color.New(color.FgRed, color.Bold),
		titleColor:   color.New(color.FgMagenta, color.Bold),
	}

	// 设置标准日志格式
	log.SetFlags(log.LstdFlags)
}

// Info 输出信息日志
func Info(format string, args ...interface{}) {
	DefaultLogger.Info(format, args...)
}

// Success 输出成功日志
func Success(format string, args ...interface{}) {
	DefaultLogger.Success(format, args...)
}

// Warning 输出警告日志
func Warning(format string, args ...interface{}) {
	DefaultLogger.Warning(format, args...)
}

// Error 输出错误日志
func Error(format string, args ...interface{}) {
	DefaultLogger.Error(format, args...)
}

// Title 输出标题日志
func Title(format string, args ...interface{}) {
	DefaultLogger.Title(format, args...)
}

// Info 输出信息日志
func (l *Logger) Info(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	l.infoColor.Printf("ℹ [%s] %s\n", timestamp, message)
}

// Success 输出成功日志
func (l *Logger) Success(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	l.successColor.Printf("✓ [%s] %s\n", timestamp, message)
}

// Warning 输出警告日志
func (l *Logger) Warning(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	l.warningColor.Printf("⚠ [%s] %s\n", timestamp, message)
}

// Error 输出错误日志
func (l *Logger) Error(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	l.errorColor.Printf("✗ [%s] %s\n", timestamp, message)
}

// Title 输出标题日志
func (l *Logger) Title(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	line := "═══════════════════════════════════════════════════════════"
	l.titleColor.Println(line)
	l.titleColor.Printf("                    %s\n", message)
	l.titleColor.Println(line)
}

// Fatal 输出致命错误并退出
func Fatal(format string, args ...interface{}) {
	Error(format, args...)
	os.Exit(1)
}

// Printf 普通输出
func Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// Println 普通输出
func Println(args ...interface{}) {
	fmt.Println(args...)
}