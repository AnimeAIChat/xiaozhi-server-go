package utils

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/src/configs"
)

// LogLevel 日志级别
type LogLevel string

const (
	DebugLevel LogLevel = "debug"
	InfoLevel  LogLevel = "info"
	WarnLevel  LogLevel = "warn"
	ErrorLevel LogLevel = "error"
)

const (
	LogRetentionDays = 7 // 日志保留天数，硬编码7天
)

var DefaultLogger *Logger

// Logger 日志接口实现
type Logger struct {
	config      *configs.Config
	jsonLogger  *slog.Logger // 文件JSON输出
	textLogger  *slog.Logger // 控制台文本输出
	logFile     *os.File
	currentDate string        // 当前日期 YYYY-MM-DD
	mu          sync.RWMutex  // 读写锁保护
	ticker      *time.Ticker  // 定时器
	stopCh      chan struct{} // 停止信号
}

// configLogLevelToSlogLevel 将配置中的日志级别转换为slog.Level
func configLogLevelToSlogLevel(configLevel string) slog.Level {
	switch configLevel {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger 创建新的日志记录器
func NewLogger(config *configs.Config) (*Logger, error) {
	// 确保日志目录存在
	if err := os.MkdirAll(config.Log.LogDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 打开或创建日志文件
	logPath := filepath.Join(config.Log.LogDir, config.Log.LogFile)
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %v", err)
	}

	// 设置slog级别
	slogLevel := configLogLevelToSlogLevel(config.Log.LogLevel)

	// 创建JSON处理器（用于文件输出）
	jsonHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: slogLevel,
	})

	// 创建文本处理器（用于控制台输出）
	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})

	// 创建logger实例
	jsonLogger := slog.New(jsonHandler)
	textLogger := slog.New(textHandler)

	logger := &Logger{
		config:      config,
		jsonLogger:  jsonLogger,
		textLogger:  textLogger,
		logFile:     file,
		currentDate: time.Now().Format("2006-01-02"),
		stopCh:      make(chan struct{}),
	}

	// 启动日志轮转检查器
	logger.startRotationChecker()
	if DefaultLogger == nil {
		DefaultLogger = logger
	}

	return logger, nil
}

// startRotationChecker 启动定时检查器
func (l *Logger) startRotationChecker() {
	l.ticker = time.NewTicker(1 * time.Minute) // 每分钟检查一次
	go func() {
		for {
			select {
			case <-l.ticker.C:
				l.checkAndRotate()
			case <-l.stopCh:
				return
			}
		}
	}()
}

// checkAndRotate 检查并执行轮转
func (l *Logger) checkAndRotate() {
	today := time.Now().Format("2006-01-02")
	if today != l.currentDate {
		l.rotateLogFile(today)
		l.cleanOldLogs()
	}
}

// rotateLogFile 执行日志轮转
func (l *Logger) rotateLogFile(newDate string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 关闭当前日志文件
	if l.logFile != nil {
		l.logFile.Close()
	}

	// 构建旧文件名和新文件名
	logDir := l.config.Log.LogDir
	currentLogPath := filepath.Join(logDir, l.config.Log.LogFile)

	// 生成带日期的文件名
	baseFileName := strings.TrimSuffix(l.config.Log.LogFile, filepath.Ext(l.config.Log.LogFile))
	ext := filepath.Ext(l.config.Log.LogFile)
	archivedLogPath := filepath.Join(logDir, fmt.Sprintf("%s-%s%s", baseFileName, l.currentDate, ext))

	// 重命名当前日志文件为带日期的文件
	if _, err := os.Stat(currentLogPath); err == nil {
		if err := os.Rename(currentLogPath, archivedLogPath); err != nil {
			// 如果重命名失败，记录到控制台
			l.textLogger.Error("重命名日志文件失败", slog.String("error", err.Error()))
		}
	}

	// 创建新的日志文件
	file, err := os.OpenFile(currentLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		l.textLogger.Error("创建新日志文件失败", slog.String("error", err.Error()))
		return
	}

	// 更新logger配置
	l.logFile = file
	l.currentDate = newDate

	// 重新创建JSON处理器
	slogLevel := configLogLevelToSlogLevel(l.config.Log.LogLevel)
	jsonHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: slogLevel,
	})
	l.jsonLogger = slog.New(jsonHandler)

	// 记录轮转信息
	l.textLogger.Info("日志文件已轮转", slog.String("new_date", newDate))
}

// cleanOldLogs 清理旧日志文件
func (l *Logger) cleanOldLogs() {
	logDir := l.config.Log.LogDir

	// 读取日志目录
	entries, err := os.ReadDir(logDir)
	if err != nil {
		l.textLogger.Error("读取日志目录失败", slog.String("error", err.Error()))
		return
	}

	// 计算保留截止日期
	cutoffDate := time.Now().AddDate(0, 0, -LogRetentionDays)
	baseFileName := strings.TrimSuffix(l.config.Log.LogFile, filepath.Ext(l.config.Log.LogFile))
	ext := filepath.Ext(l.config.Log.LogFile)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		// 检查是否是带日期的日志文件格式：server-YYYY-MM-DD.log
		if strings.HasPrefix(fileName, baseFileName+"-") && strings.HasSuffix(fileName, ext) {
			// 提取日期部分
			dateStr := strings.TrimPrefix(fileName, baseFileName+"-")
			dateStr = strings.TrimSuffix(dateStr, ext)

			// 解析日期
			fileDate, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				continue // 如果日期格式不正确，跳过
			}

			// 如果文件日期早于截止日期，删除文件
			if fileDate.Before(cutoffDate) {
				filePath := filepath.Join(logDir, fileName)
				if err := os.Remove(filePath); err != nil {
					l.textLogger.Error("删除旧日志文件失败",
						slog.String("file", fileName),
						slog.String("error", err.Error()))
				} else {
					l.textLogger.Info("已删除旧日志文件", slog.String("file", fileName))
				}
			}
		}
	}
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	// 停止定时器
	if l.ticker != nil {
		l.ticker.Stop()
	}

	// 发送停止信号
	close(l.stopCh)

	// 关闭日志文件
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// log 通用日志记录函数（内部使用）
func (l *Logger) log(level slog.Level, msg string, fields ...interface{}) {
	// 使用读锁保护并发访问
	l.mu.RLock()
	defer l.mu.RUnlock()

	// 构建slog属性
	var attrs []slog.Attr
	if len(fields) > 0 && fields[0] != nil {
		// 处理fields参数
		if fieldsMap, ok := fields[0].(map[string]interface{}); ok {
			for k, v := range fieldsMap {
				attrs = append(attrs, slog.Any(k, v))
			}
		} else {
			// 如果不是map，直接作为fields字段
			attrs = append(attrs, slog.Any("fields", fields[0]))
		}
	}

	// 同时写入文件（JSON）和控制台（文本）
	ctx := context.Background()
	l.jsonLogger.LogAttrs(ctx, level, msg, attrs...)
	l.textLogger.LogAttrs(ctx, level, msg, attrs...)
}

// Debug 记录调试级别日志
func (l *Logger) Debug(msg string, args ...interface{}) {
	if l.config.Log.LogLevel == "DEBUG" {
		if len(args) > 0 && containsFormatPlaceholders(msg) {
			formattedMsg := fmt.Sprintf(msg, args...)
			l.log(slog.LevelDebug, formattedMsg)
		} else {
			l.log(slog.LevelDebug, msg, args...)
		}
	}
}

func containsFormatPlaceholders(s string) bool {
	return strings.Contains(s, "%")
}

// Info 记录信息级别日志
func (l *Logger) Info(msg string, args ...interface{}) {
	// 检测是否为格式化模式
	if len(args) > 0 && containsFormatPlaceholders(msg) {
		// 格式化模式：类似 Info
		formattedMsg := fmt.Sprintf(msg, args...)
		l.log(slog.LevelInfo, formattedMsg)
	} else {
		// 结构化模式：原有方式
		l.log(slog.LevelInfo, msg, args...)
	}
}

// Warn 记录警告级别日志
func (l *Logger) Warn(msg string, args ...interface{}) {
	if len(args) > 0 && containsFormatPlaceholders(msg) {
		formattedMsg := fmt.Sprintf(msg, args...)
		l.log(slog.LevelWarn, formattedMsg)
	} else {
		l.log(slog.LevelWarn, msg, args...)
	}
}

// Error 记录错误级别日志
func (l *Logger) Error(msg string, args ...interface{}) {
	if len(args) > 0 && containsFormatPlaceholders(msg) {
		formattedMsg := fmt.Sprintf(msg, args...)
		l.log(slog.LevelError, formattedMsg)
	} else {
		l.log(slog.LevelError, msg, args...)
	}
}
