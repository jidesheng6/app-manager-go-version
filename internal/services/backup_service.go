package services

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupService 备份服务
type BackupService struct {
	config *config.Config
}

// NewBackupService 创建备份服务实例
func NewBackupService() *BackupService {
	return &BackupService{
		config: config.AppConfig,
	}
}

// BackupProject 备份项目文件
func (s *BackupService) BackupProject(backupName, description string) error {
	// 生成备份文件名，始终添加时间戳（包含毫秒以避免重复）
	timestamp := time.Now().Format("20060102_150405_000")
	var finalBackupName string
	if backupName == "" {
		finalBackupName = fmt.Sprintf("%s_%s", s.config.Backup.BackupPrefix, timestamp)
	} else {
		finalBackupName = fmt.Sprintf("%s_%s", backupName, timestamp)
	}
	
	logger.Title("项目文件备份")
	logger.Info("备份名称: %s", finalBackupName)
	logger.Info("开始备份项目文件...")
	
	projectRoot := config.GetProjectRoot()
	backupDir := config.GetBackupDir()
	
	// 确保备份目录存在
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("创建备份目录失败: %w", err)
	}
	
	// 创建备份文件
	backupFile := filepath.Join(backupDir, finalBackupName+".zip")
	
	logger.Info("备份源目录: %s", projectRoot)
	logger.Info("备份目标文件: %s", backupFile)
	
	// 创建ZIP文件
	if err := s.createZipBackup(projectRoot, backupFile, finalBackupName, description); err != nil {
		return fmt.Errorf("创建备份文件失败: %w", err)
	}
	
	// 显示备份摘要
	s.displayBackupSummary(backupFile, finalBackupName, description, "项目文件")
	
	// 清理旧备份
	s.cleanOldBackups(backupDir)
	
	return nil
}

// createZipBackup 创建ZIP备份
func (s *BackupService) createZipBackup(sourceDir, targetFile, backupName, description string) error {
	// 创建ZIP文件
	zipFile, err := os.Create(targetFile)
	if err != nil {
		return fmt.Errorf("创建ZIP文件失败: %w", err)
	}
	defer zipFile.Close()
	
	// 创建ZIP写入器
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()
	
	// 首先添加readme.txt文件
	if err := s.addReadmeToZip(zipWriter, backupName, description); err != nil {
		logger.Warning("添加readme文件失败: %v", err)
	}
	
	// 需要备份的目录
	backupDirs := []string{
		"project_backend",
		"project_frontend",
		"project_cache",
	}
	
	fileCount := 0
	
	for _, dir := range backupDirs {
		dirPath := filepath.Join(sourceDir, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}
		
		logger.Info("备份目录: %s", dir)
		
		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			
			// 跳过某些文件和目录
			if s.shouldSkipFile(path, info) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			
			// 计算相对路径
			relPath, err := filepath.Rel(sourceDir, path)
			if err != nil {
				return err
			}
			
			// 标准化路径分隔符
			relPath = filepath.ToSlash(relPath)
			
			if info.IsDir() {
				// 创建目录条目
				_, err := zipWriter.Create(relPath + "/")
				return err
			}
			
			// 创建文件条目
			zipFileWriter, err := zipWriter.Create(relPath)
			if err != nil {
				return err
			}
			
			// 复制文件内容
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			
			_, err = io.Copy(zipFileWriter, file)
			if err != nil {
				return err
			}
			
			fileCount++
			if fileCount%100 == 0 {
				logger.Info("已备份 %d 个文件...", fileCount)
			}
			
			return nil
		})
		
		if err != nil {
			return fmt.Errorf("备份目录失败 %s: %w", dir, err)
		}
	}
	
	logger.Info("备份完成，共备份 %d 个文件", fileCount)
	return nil
}

// addReadmeToZip 添加readme.txt文件到ZIP中
func (s *BackupService) addReadmeToZip(zipWriter *zip.Writer, backupName, description string) error {
	// 创建readme.txt文件内容
	now := time.Now()
	readmeContent := fmt.Sprintf(`备份说明文档
═══════════════════════════════════════════════════════════

备份信息：
  备份名称: %s
  备份时间: %s
  备份日期: %s
  
备份说明：
  %s

备份内容：
  - project_backend/    # 后端应用目录
  - project_frontend/   # 前端应用目录  
  - project_cache/      # 缓存文件目录

注意事项：
  1. 此备份不包含日志文件和临时文件
  2. 超过1TB的大文件已被跳过
  3. 恢复时请确保目标目录为空或已备份
  4. 建议在恢复前停止相关应用服务

恢复方法：
  1. 解压备份文件到目标目录
  2. 检查配置文件是否正确
  3. 启动应用服务
  4. 验证应用功能是否正常

技术支持：
  如有问题请联系系统管理员

═══════════════════════════════════════════════════════════
备份工具: app-manager-go-v1
生成时间: %s
`, backupName, now.Format("15:04:05"), now.Format("2006-01-02"), description, now.Format("2006-01-02 15:04:05"))

	// 创建ZIP文件条目
	readmeWriter, err := zipWriter.Create("README.txt")
	if err != nil {
		return fmt.Errorf("创建readme文件条目失败: %w", err)
	}
	
	// 写入内容
	_, err = readmeWriter.Write([]byte(readmeContent))
	if err != nil {
		return fmt.Errorf("写入readme文件内容失败: %w", err)
	}
	
	logger.Info("已添加备份说明文档到ZIP文件")
	return nil
}

// shouldSkipFile 判断是否应该跳过文件
func (s *BackupService) shouldSkipFile(path string, info os.FileInfo) bool {
	name := info.Name()
	
	// 跳过隐藏文件（除了配置文件）
	if strings.HasPrefix(name, ".") && !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".ini") {
		return true
	}
	
	// 跳过临时文件
	if strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".temp") {
		return true
	}
	
	// 跳过日志文件（可选）
	if strings.Contains(path, "logs") && (strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".out")) {
		return true
	}
	
	// 跳过PID文件
	if strings.Contains(path, "pids") && strings.HasSuffix(name, ".pid") {
		return true
	}
	
	// 跳过备份目录本身
	if strings.Contains(path, "project_backup") {
		return true
	}
	
	// 跳过超过1TB的大文件（实际应用中的合理限制）
	if !info.IsDir() && info.Size() > 1024*1024*1024*1024 {
		logger.Warning("跳过大文件: %s (%.2f GB)", path, float64(info.Size())/(1024*1024*1024))
		return true
	}
	
	return false
}

// displayBackupSummary 显示备份摘要
func (s *BackupService) displayBackupSummary(backupFile, backupName, description, backupType string) {
	// 计算文件大小
	backupSize := "未知"
	if stat, err := os.Stat(backupFile); err == nil {
		size := stat.Size()
		if size < 1024 {
			backupSize = fmt.Sprintf("%d B", size)
		} else if size < 1024*1024 {
			backupSize = fmt.Sprintf("%.2f KB", float64(size)/1024)
		} else if size < 1024*1024*1024 {
			backupSize = fmt.Sprintf("%.2f MB", float64(size)/(1024*1024))
		} else {
			backupSize = fmt.Sprintf("%.2f GB", float64(size)/(1024*1024*1024))
		}
	}
	
	logger.Success("%s备份完成", backupType)
	logger.Println("")
	
	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Printf("\033[32m\033[1m  📦 备份创建成功！\033[0m\n")
	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Println("")
	
	logger.Info("备份详情：")
	logger.Printf("  备份名称: \033[33m%s\033[0m\n", filepath.Base(backupFile))
	logger.Printf("  备份类型: \033[33m%s\033[0m\n", backupType)
	logger.Printf("  存储路径: \033[33m%s\033[0m\n", backupFile)
	logger.Printf("  文件大小: \033[33m%s\033[0m\n", backupSize)
	logger.Printf("  备份时间: \033[33m%s\033[0m\n", time.Now().Format("2006-01-02 15:04:05"))
	
	if description != "" {
		logger.Printf("  备份说明: \033[33m%s\033[0m\n", description)
	}
	
	logger.Println("")
	logger.Info("操作建议：")
	logger.Printf("  查看内容: \033[37munzip -l '%s'\033[0m\n", backupFile)
	logger.Printf("  解压缩: \033[37munzip '%s' -d /path/to/restore/\033[0m\n", backupFile)
	logger.Println("")
}

// cleanOldBackups 清理旧备份
func (s *BackupService) cleanOldBackups(backupDir string) {
	if s.config.Backup.BackupRetentionDays <= 0 {
		return
	}
	
	logger.Info("清理 %d 天前的旧备份文件...", s.config.Backup.BackupRetentionDays)
	
	cutoffTime := time.Now().AddDate(0, 0, -s.config.Backup.BackupRetentionDays)
	
	files, err := os.ReadDir(backupDir)
	if err != nil {
		logger.Warning("读取备份目录失败: %v", err)
		return
	}
	
	deletedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		// 只处理备份文件
		if !strings.HasSuffix(file.Name(), ".zip") &&
			!strings.HasSuffix(file.Name(), ".gz") &&
			!strings.HasSuffix(file.Name(), ".tar") {
			continue
		}
		
		filePath := filepath.Join(backupDir, file.Name())
		stat, err := os.Stat(filePath)
		if err != nil {
			continue
		}
		
		if stat.ModTime().Before(cutoffTime) {
			if err := os.Remove(filePath); err != nil {
				logger.Warning("删除旧备份失败 %s: %v", file.Name(), err)
			} else {
				logger.Info("删除旧备份: %s", file.Name())
				deletedCount++
			}
		}
	}
	
	if deletedCount > 0 {
		logger.Info("清理完成，删除了 %d 个旧备份文件", deletedCount)
	} else {
		logger.Info("没有需要清理的旧备份文件")
	}
}