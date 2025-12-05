package utils

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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

type LogCfg struct {
	LogLevel string `yaml:"log_level" json:"log_level"`
	LogDir   string `yaml:"log_dir" json:"log_dir"`
	LogFile  string `yaml:"log_file" json:"log_file"`
}

// CustomTextHandler 自定义文本处理器，支持彩色输出和格式化
type CustomTextHandler struct {
	writer io.Writer
	level  slog.Level
	mu     sync.Mutex
}

var (
	colorReset  = "\x1b[0m"
	colorTime   = "\x1b[90m" // 时间：灰色
	colorDebug  = "\x1b[36m" // DEBUG：青色
	colorInfo   = "\x1b[32m" // INFO：绿色
	colorWarn   = "\x1b[33m" // WARN：黄色
	colorError  = "\x1b[31m" // ERROR：红色
	colorASR    = "\x1b[35m" // ASR：品红
	colorLLM    = "\x1b[34m" // LLM：蓝色
	colorTTS    = "\x1b[95m" // TTS：亮品红
	colorTiming = "\x1b[92m" // Timing：亮绿色
)

func (h *CustomTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *CustomTextHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 获取时间戳
	timeStr := r.Time.Format("2006-01-02 15:04:05.000")

	// 获取日志级别中文描述
	var levelStr string
	switch r.Level {
	case slog.LevelDebug:
		levelStr = "调试"
	case slog.LevelInfo:
		levelStr = "信息"
	case slog.LevelWarn:
		levelStr = "警告"
	case slog.LevelError:
		levelStr = "错误"
	default:
		levelStr = "信息"
	}

	// 应用颜色
	var levelColor string
	switch r.Level {
	case slog.LevelDebug:
		levelColor = colorDebug
	case slog.LevelInfo:
		levelColor = colorInfo
	case slog.LevelWarn:
		levelColor = colorWarn
	case slog.LevelError:
		levelColor = colorError
	default:
		levelColor = colorReset
	}

	// 检查是否是特殊阶段日志或模块日志
	var moduleColor string
	var isModuleLog bool
	msg := r.Message

	// 检测各种模块标签
	if strings.HasPrefix(msg, "[引导]") {
		moduleColor = "\x1b[96m" // 引导：亮青色
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[传输]") {
		moduleColor = "\x1b[94m" // 传输：亮蓝色
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[HTTP]") {
		moduleColor = "\x1b[95m" // HTTP：亮品红
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[WebSocket]") {
		moduleColor = "\x1b[92m" // WebSocket：亮绿色
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[ASR]") {
		moduleColor = colorASR
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[LLM]") {
		moduleColor = colorLLM
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[TTS]") {
		moduleColor = colorTTS
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[TIMING]") {
		moduleColor = colorTiming
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[MCP]") {
		moduleColor = "\x1b[36m" // MCP：青蓝色
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[认证]") {
		moduleColor = "\x1b[94m" // 认证：亮红色
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[视觉]") {
		moduleColor = "\x1b[95m" // 视觉：亮品红
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[OTA]") {
		moduleColor = "\x1b[97m" // OTA：亮白色
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[WebAPI]") {
		moduleColor = "\x1b[96m" // WebAPI：亮青色
		isModuleLog = true
	} else if strings.HasPrefix(msg, "[OBSERVABILITY]") {
		moduleColor = "\x1b[90m" // 可观测性：灰色
		isModuleLog = true
	}

	// 构建输出
	var output string
	if isModuleLog {
		// 模块日志格式: [时间] [模块] 消息
		output = fmt.Sprintf("%s[%s]%s %s%s%s",
			colorTime, timeStr, colorReset,
			moduleColor, msg, colorReset)
	} else {
		// 普通日志格式: [时间] [级别] 消息
		output = fmt.Sprintf("%s[%s]%s %s[%s]%s %s",
			colorTime, timeStr, colorReset,
			levelColor, levelStr, colorReset,
			msg)
	}

	// 添加属性（如果有）
	if r.NumAttrs() > 0 {
		output += " {"
		r.Attrs(func(a slog.Attr) bool {
			output += fmt.Sprintf(" %s=%v", a.Key, a.Value)
			return true
		})
		output += " }"
	}
	output += "\n"

	_, err := h.writer.Write([]byte(output))
	return err
}

func (h *CustomTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h // 简化实现
}

func (h *CustomTextHandler) WithGroup(name string) slog.Handler {
	return h // 简化实现
}

// Logger 日志接口实现
type Logger struct {
	config      *LogCfg
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
	switch strings.ToLower(configLevel) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger 创建新的日志记录器
func NewLogger(config *LogCfg) (*Logger, error) {
	// 确保日志目录存在
	if err := os.MkdirAll(config.LogDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 打开或创建日志文件
	logPath := filepath.Join(config.LogDir, config.LogFile)
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %v", err)
	}

	// 设置slog级别
	slogLevel := configLogLevelToSlogLevel(config.LogLevel)

	// 创建JSON处理器（用于文件输出）
	jsonHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: slogLevel,
	})

	// 创建自定义文本处理器（用于控制台输出）
	customHandler := &CustomTextHandler{
		writer: os.Stdout,
		level:  slogLevel,
	}

	// 创建logger实例
	jsonLogger := slog.New(jsonHandler)
	textLogger := slog.New(customHandler)

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
	logDir := l.config.LogDir
	currentLogPath := filepath.Join(logDir, l.config.LogFile)

	// 生成带日期的文件名
	baseFileName := strings.TrimSuffix(l.config.LogFile, filepath.Ext(l.config.LogFile))
	ext := filepath.Ext(l.config.LogFile)
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
	slogLevel := configLogLevelToSlogLevel(l.config.LogLevel)
	jsonHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: slogLevel,
	})
	l.jsonLogger = slog.New(jsonHandler)

	// 记录轮转信息
	l.textLogger.Info("日志文件已轮转", slog.String("new_date", newDate))
}

// cleanOldLogs 清理旧日志文件
func (l *Logger) cleanOldLogs() {
	logDir := l.config.LogDir

	// 读取日志目录
	entries, err := os.ReadDir(logDir)
	if err != nil {
		l.textLogger.Error("读取日志目录失败", slog.String("error", err.Error()))
		return
	}

	// 计算保留截止日期
	cutoffDate := time.Now().AddDate(0, 0, -LogRetentionDays)
	baseFileName := strings.TrimSuffix(l.config.LogFile, filepath.Ext(l.config.LogFile))
	ext := filepath.Ext(l.config.LogFile)

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
			// 提取并排序键
			keys := make([]string, 0, len(fieldsMap))
			for k := range fieldsMap {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			// 按排序后的键顺序添加属性
			for _, k := range keys {
				attrs = append(attrs, slog.Any(k, fieldsMap[k]))
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
	if l.config.LogLevel == "DEBUG" {
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

// FormatLog 构造带单一分类标签的日志消息。例如：FormatLog("引导", "服务已启动") -> "[引导] 服务已启动"
// 如果传入的 message 已经以 "[" 开头（表示可能已包含标签），则直接返回原文。
func FormatLog(tag, message string) string {
	tag = strings.TrimSpace(tag)
	message = strings.TrimSpace(message)
	if tag == "" {
		return message
	}
	if strings.HasPrefix(message, "[") {
		return message
	}
	return fmt.Sprintf("[%s] %s", tag, message)
}

func (l *Logger) logWithTag(level slog.Level, tag, msg string, args ...interface{}) {
	switch level {
	case slog.LevelDebug:
		l.Debug(FormatLog(tag, msg), args...)
	case slog.LevelInfo:
		l.Info(FormatLog(tag, msg), args...)
	case slog.LevelWarn:
		l.Warn(FormatLog(tag, msg), args...)
	case slog.LevelError:
		l.Error(FormatLog(tag, msg), args...)
	default:
		l.Info(FormatLog(tag, msg), args...)
	}
}

// DebugTag 记录带分类标签的调试日志
func (l *Logger) DebugTag(tag, msg string, args ...interface{}) {
	if l == nil {
		return
	}
	l.logWithTag(slog.LevelDebug, tag, msg, args...)
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

// InfoTag 记录带分类标签的信息日志
func (l *Logger) InfoTag(tag, msg string, args ...interface{}) {
	if l == nil {
		return
	}
	l.logWithTag(slog.LevelInfo, tag, msg, args...)
}

// WarnTag 记录带分类标签的警告日志
func (l *Logger) WarnTag(tag, msg string, args ...interface{}) {
	if l == nil {
		return
	}
	l.logWithTag(slog.LevelWarn, tag, msg, args...)
}

// ErrorTag 记录带分类标签的错误日志
func (l *Logger) ErrorTag(tag, msg string, args ...interface{}) {
	if l == nil {
		return
	}
	l.logWithTag(slog.LevelError, tag, msg, args...)
}

// InfoASR 记录ASR阶段信息日志
func (l *Logger) InfoASR(msg string, args ...interface{}) {
	prefixedMsg := "[ASR] " + msg
	l.Info(prefixedMsg, args...)
}

// InfoLLM 记录LLM阶段信息日志
func (l *Logger) InfoLLM(msg string, args ...interface{}) {
	prefixedMsg := "[LLM] " + msg
	l.Info(prefixedMsg, args...)
}

// InfoTTS 记录TTS阶段信息日志
func (l *Logger) InfoTTS(msg string, args ...interface{}) {
	prefixedMsg := "[TTS] " + msg
	l.Info(prefixedMsg, args...)
}

// InfoTiming 记录计时信息日志
func (l *Logger) InfoTiming(msg string, args ...interface{}) {
	prefixedMsg := "[TIMING] " + msg
	l.Info(prefixedMsg, args...)
}

// Slog exposes the underlying slog text logger for structured integrations.
func (l *Logger) Slog() *slog.Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.textLogger
}
