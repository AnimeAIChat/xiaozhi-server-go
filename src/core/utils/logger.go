package utils

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
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
	colorTime   = "\x1b[93m" // 时间：浅黄色 (Bright Yellow)
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

	// 获取日志级别
	levelStr := r.Level.String()

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

	// 检查是否是特殊阶段日志
	var stageColor string
	var isStageLog bool
	msg := r.Message

	if strings.HasPrefix(msg, "[ASR]") || strings.HasPrefix(msg, "[ASR] ") {
		stageColor = colorASR
		isStageLog = true
	} else if strings.HasPrefix(msg, "[LLM]") || strings.HasPrefix(msg, "[LLM] ") {
		stageColor = colorLLM
		isStageLog = true
	} else if strings.HasPrefix(msg, "[TTS]") || strings.HasPrefix(msg, "[TTS] ") {
		stageColor = colorTTS
		isStageLog = true
	} else if strings.HasPrefix(msg, "[TIMING]") || strings.HasPrefix(msg, "[TIMING] ") {
		stageColor = colorTiming
		isStageLog = true
	}

	// 提取 source 属性（文件名:行号）
	var source string
	attrsFunc := func(a slog.Attr) bool {
		if a.Key == "source" {
			source = a.Value.String()
		}
		return true
	}
	r.Attrs(attrsFunc)

	// 构建输出
	var output string
	if isStageLog {
		// 阶段日志格式: [时间] [阶段] [行号] 消息内容
		// msg 已包含阶段前缀如 "[TTS] 消息内容"，需要提取消息内容放在行号后面
		stagePrefix := ""
		stageContent := msg
		if strings.HasPrefix(msg, "[ASR]") || strings.HasPrefix(msg, "[ASR] ") {
			stagePrefix = "[ASR]"
			stageContent = strings.TrimPrefix(msg, "[ASR]")
			stageContent = strings.TrimPrefix(stageContent, " ")
		} else if strings.HasPrefix(msg, "[LLM]") || strings.HasPrefix(msg, "[LLM] ") {
			stagePrefix = "[LLM]"
			stageContent = strings.TrimPrefix(msg, "[LLM]")
			stageContent = strings.TrimPrefix(stageContent, " ")
		} else if strings.HasPrefix(msg, "[TTS]") || strings.HasPrefix(msg, "[TTS] ") {
			stagePrefix = "[TTS]"
			stageContent = strings.TrimPrefix(msg, "[TTS]")
			stageContent = strings.TrimPrefix(stageContent, " ")
		} else if strings.HasPrefix(msg, "[TIMING]") || strings.HasPrefix(msg, "[TIMING] ") {
			stagePrefix = "[TIMING]"
			stageContent = strings.TrimPrefix(msg, "[TIMING]")
			stageContent = strings.TrimPrefix(stageContent, " ")
		}
		// 格式: [时间] [阶段] [行号] 消息内容
		output = colorTime + "[" + timeStr + "]" + colorReset + " " +
			stageColor + stagePrefix + colorReset + " " +
			colorTime + "[" + source + "]" + colorReset + " " +
			stageContent
	} else {
		// 普通日志格式: [时间] [级别] [source] 消息
		output = fmt.Sprintf("%s[%s]%s %s[%s]%s %s[%s]%s %s",
			colorTime, timeStr, colorReset,
			levelColor, levelStr, colorReset,
			colorTime, source, colorReset,
			msg)
	}

	// 添加其他属性（排除 source）
	if r.NumAttrs() > 0 {
		output += " {"
		r.Attrs(func(a slog.Attr) bool {
			if a.Key != "source" {
				output += fmt.Sprintf(" %s=%v", a.Key, a.Value)
			}
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
// skipFrames: 额外的跳过帧数，用于获取真实的调用者信息
func (l *Logger) log(level slog.Level, msg string, skipFrames int, fields ...interface{}) {
	// 使用读锁保护并发访问
	l.mu.RLock()
	defer l.mu.RUnlock()

	// 获取调用方信息（跳过 runtime.Callers + log + Debug/Info/Warn/Error + 额外的 skipFrames）
	pcs := make([]uintptr, 3+skipFrames)
	runtime.Callers(3+skipFrames, pcs)
	frames := runtime.CallersFrames(pcs)
	frame, _ := frames.Next()

	// 构建 source 属性
	source := fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)

	// 构建 slog 属性
	attrs := []slog.Attr{slog.String("source", source)}
	if len(fields) > 0 && fields[0] != nil {
		// 处理 fields 参数
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
			// 如果不是 map，直接作为 fields 字段
			attrs = append(attrs, slog.Any("fields", fields[0]))
		}
	}

	// 同时写入文件（JSON）和控制台（文本）
	ctx := context.Background()
	l.jsonLogger.LogAttrs(ctx, level, msg, attrs...)
	l.textLogger.LogAttrs(ctx, level, msg, attrs...)
}

// Debug 记录调试级别日志
// 参数支持:
//   - Debug(msg) - 仅消息
//   - Debug(msg, fieldsMap) - 消息 + 字段映射
//   - Debug(msg, skipFrames) - 消息 + 跳过帧数
//   - Debug(msg, skipFrames, fieldsMap) - 消息 + 跳过帧数 + 字段映射
func (l *Logger) Debug(msg string, args ...interface{}) {
	if l.config.LogLevel == "DEBUG" {
		skipFrames, fields := l.parseArgs(args)
		if fields != nil {
			l.log(slog.LevelDebug, msg, skipFrames, fields)
		} else if containsFormatPlaceholders(msg) && len(args) > 0 {
			l.log(slog.LevelDebug, fmt.Sprintf(msg, args...), skipFrames)
		} else {
			l.log(slog.LevelDebug, msg, skipFrames)
		}
	}
}

func containsFormatPlaceholders(s string) bool {
	return strings.Contains(s, "%")
}

// parseArgs 解析可变参数，支持 skipFrames 和 fieldsMap
// 返回 skipFrames 和 fieldsMap
func (l *Logger) parseArgs(args []interface{}) (int, interface{}) {
	var skipFrames int
	var fields interface{}

	if len(args) == 0 {
		return 0, nil
	}

	// 只有 1 个参数
	if len(args) == 1 {
		if skip, ok := args[0].(int); ok {
			// 只有 skipFrames
			return skip, nil
		}
		// 只有 fieldsMap
		return 0, args[0]
	}

	// 2 个参数：可能是 (skipFrames, fieldsMap) 或 (fieldsMap, 忽略)
	if len(args) >= 2 {
		if skip, ok := args[0].(int); ok {
			skipFrames = skip
			fields = args[1]
		}
	}

	return skipFrames, fields
}

// Info 记录信息级别日志
// 参数支持:
//   - Info(msg) - 仅消息
//   - Info(msg, fieldsMap) - 消息 + 字段映射
//   - Info(msg, skipFrames) - 消息 + 跳过帧数
//   - Info(msg, skipFrames, fieldsMap) - 消息 + 跳过帧数 + 字段映射
func (l *Logger) Info(msg string, args ...interface{}) {
	skipFrames, fields := l.parseArgs(args)
	if fields != nil {
		l.log(slog.LevelInfo, msg, skipFrames, fields)
	} else if containsFormatPlaceholders(msg) && len(args) > 0 {
		l.log(slog.LevelInfo, fmt.Sprintf(msg, args...), skipFrames)
	} else {
		l.log(slog.LevelInfo, msg, skipFrames)
	}
}

// Warn 记录警告级别日志
// 参数支持:
//   - Warn(msg) - 仅消息
//   - Warn(msg, fieldsMap) - 消息 + 字段映射
//   - Warn(msg, skipFrames) - 消息 + 跳过帧数
//   - Warn(msg, skipFrames, fieldsMap) - 消息 + 跳过帧数 + 字段映射
func (l *Logger) Warn(msg string, args ...interface{}) {
	skipFrames, fields := l.parseArgs(args)
	if fields != nil {
		l.log(slog.LevelWarn, msg, skipFrames, fields)
	} else if containsFormatPlaceholders(msg) && len(args) > 0 {
		l.log(slog.LevelWarn, fmt.Sprintf(msg, args...), skipFrames)
	} else {
		l.log(slog.LevelWarn, msg, skipFrames)
	}
}

// Error 记录错误级别日志
// 参数支持:
//   - Error(msg) - 仅消息
//   - Error(msg, fieldsMap) - 消息 + 字段映射
//   - Error(msg, skipFrames) - 消息 + 跳过帧数
//   - Error(msg, skipFrames, fieldsMap) - 消息 + 跳过帧数 + 字段映射
func (l *Logger) Error(msg string, args ...interface{}) {
	skipFrames, fields := l.parseArgs(args)
	if fields != nil {
		l.log(slog.LevelError, msg, skipFrames, fields)
	} else if containsFormatPlaceholders(msg) && len(args) > 0 {
		l.log(slog.LevelError, fmt.Sprintf(msg, args...), skipFrames)
	} else {
		l.log(slog.LevelError, msg, skipFrames)
	}
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
