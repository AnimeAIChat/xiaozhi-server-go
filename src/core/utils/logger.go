package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Logger 日志接口实现
type Logger struct {
	config  *configs.Config
	logFile *os.File
}

// LogEntry 日志条目结构
type LogEntry struct {
	Time    string      `json:"time"`
	Level   LogLevel    `json:"level"`
	Tag     string      `json:"tag,omitempty"`
	Message string      `json:"message"`
	Fields  interface{} `json:"fields,omitempty"`
}

// NewLogger 创建新的日志记录器
func NewLogger(config *configs.Config) (*Logger, error) {
	// 确保日志目录存在
	if err := os.MkdirAll(config.Log.LogDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 打开或创建日志文件
	logPath := filepath.Join(config.Log.LogDir, config.Log.LogFile)
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %v", err)
	}

	return &Logger{
		config:  config,
		logFile: file,
	}, nil
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// log 通用日志记录函数
func (l *Logger) log(level LogLevel, tag string, msg string, fields ...interface{}) {
	nowString := time.Now().Format("2006-01-02 15:04:05.000")
	entry := LogEntry{
		Time:    nowString,
		Level:   level,
		Tag:     tag,
		Message: msg,
	}

	if len(fields) > 0 {
		entry.Fields = fields[0]
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "日志序列化失败: %v\n", err)
		return
	}

	// 写入文件
	if _, err := l.logFile.Write(append(data, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "写入日志失败: %s %v\n", msg, err)
	}

	// 同时输出到控制台
	fmt.Printf("[%s] [%s] %s\n", nowString, level, msg)
}

// Debug 记录调试级别日志
func (l *Logger) Debug(msg string, fields ...interface{}) {
	if l.config.Log.LogLevel == "DEBUG" {
		l.log(DebugLevel, "", msg, fields...)
	}
}

// Info 记录信息级别日志
func (l *Logger) Info(msg string, fields ...interface{}) {
	l.log(InfoLevel, "", msg, fields...)
}

// Warn 记录警告级别日志
func (l *Logger) Warn(msg string, fields ...interface{}) {
	l.log(WarnLevel, "", msg, fields...)
}

// Error 记录错误级别日志
func (l *Logger) Error(msg string, fields ...interface{}) {
	l.log(ErrorLevel, "", msg, fields...)
}

// WithTag 添加标签的日志记录器
type TaggedLogger struct {
	*Logger
	tag string
}

// WithTag 创建带标签的日志记录器
func (l *Logger) WithTag(tag string) *TaggedLogger {
	return &TaggedLogger{
		Logger: l,
		tag:    tag,
	}
}

// Debug 记录带标签的调试级别日志
func (l *TaggedLogger) Debug(msg string, fields ...interface{}) {
	if l.config.Log.LogLevel == "DEBUG" {
		l.log(DebugLevel, l.tag, msg, fields...)
	}
}

// Info 记录带标签的信息级别日志
func (l *TaggedLogger) Info(msg string, fields ...interface{}) {
	l.log(InfoLevel, l.tag, msg, fields...)
}

// Warn 记录带标签的警告级别日志
func (l *TaggedLogger) Warn(msg string, fields ...interface{}) {
	l.log(WarnLevel, l.tag, msg, fields...)
}

// Error 记录带标签的错误级别日志
func (l *TaggedLogger) Error(msg string, fields ...interface{}) {
	l.log(ErrorLevel, l.tag, msg, fields...)
}
