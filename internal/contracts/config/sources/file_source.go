package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/internal/contracts/config"
)

// FileSource 文件配置源
// 支持JSON、YAML、TOML等格式的配置文件
type FileSource struct {
	name      string
	filePath  string
	priority  int
	fileType  string
	watchMode bool

	// 监听相关
	lastModTime time.Time
	lastSize    int64
	eventChan   chan config.ConfigChangeEvent
	watcher     *FileWatcher
	mutex       sync.RWMutex

	// 缓存
	cachedConfig map[string]interface{}
	cachedTime   time.Time
	ttl          time.Duration
}

// FileSourceOption 文件配置源选项
type FileSourceOption func(*FileSource)

// NewFileSource 创建文件配置源
func NewFileSource(filePath string, options ...FileSourceOption) (*FileSource, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", filePath)
	}

	source := &FileSource{
		name:      fmt.Sprintf("file:%s", filepath.Base(filePath)),
		filePath:  filePath,
		priority:  50, // 默认优先级
		watchMode: true,
		eventChan: make(chan config.ConfigChangeEvent, 10),
		ttl:       5 * time.Minute, // 默认缓存5分钟
	}

	// 检测文件类型
	source.fileType = detectFileType(filePath)

	// 应用选项
	for _, option := range options {
		option(source)
	}

	// 初始化文件监听器
	if source.watchMode {
		watcher, err := NewFileWatcher(filePath, source.onFileChange)
		if err != nil {
			return nil, fmt.Errorf("failed to create file watcher: %w", err)
		}
		source.watcher = watcher
	}

	// 初始加载
	if err := source.initialLoad(); err != nil {
		return nil, fmt.Errorf("failed to initial load: %w", err)
	}

	return source, nil
}

// WithFilePriority 设置优先级
func WithFilePriority(priority int) FileSourceOption {
	return func(fs *FileSource) {
		fs.priority = priority
	}
}

// WithFileWatchMode 设置监听模式
func WithFileWatchMode(enable bool) FileSourceOption {
	return func(fs *FileSource) {
		fs.watchMode = enable
	}
}

// WithFileTTL 设置缓存TTL
func WithFileTTL(ttl time.Duration) FileSourceOption {
	return func(fs *FileSource) {
		fs.ttl = ttl
	}
}

// GetName 获取配置源名称
func (fs *FileSource) GetName() string {
	return fs.name
}

// GetPriority 获取配置源优先级
func (fs *FileSource) GetPriority() int {
	return fs.priority
}

// Load 加载配置数据
func (fs *FileSource) Load(ctx context.Context) (map[string]interface{}, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	// 检查缓存是否有效
	if fs.cachedConfig != nil && time.Since(fs.cachedTime) < fs.ttl {
		return fs.copyConfig(fs.cachedConfig), nil
	}

	// 重新加载文件
	config, err := fs.loadFromFile()
	if err != nil {
		return nil, err
	}

	// 更新缓存
	fs.cachedConfig = config
	fs.cachedTime = time.Now()

	return fs.copyConfig(config), nil
}

// Watch 监听配置变化
func (fs *FileSource) Watch(ctx context.Context) (<-chan config.ConfigChangeEvent, error) {
	if !fs.watchMode {
		return nil, fmt.Errorf("watch mode is disabled")
	}

	// 启动文件监听器
	if err := fs.watcher.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start file watcher: %w", err)
	}

	return fs.eventChan, nil
}

// IsAvailable 检查配置源是否可用
func (fs *FileSource) IsAvailable(ctx context.Context) bool {
	if _, err := os.Stat(fs.filePath); os.IsNotExist(err) {
		return false
	}

	// 尝试读取文件头部检查是否可读
	file, err := os.Open(fs.filePath)
	if err != nil {
		return false
	}
	file.Close()

	return true
}

// Close 关闭配置源
func (fs *FileSource) Close() error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	if fs.watcher != nil {
		if err := fs.watcher.Stop(); err != nil {
			return fmt.Errorf("failed to stop file watcher: %w", err)
		}
	}

	if fs.eventChan != nil {
		close(fs.eventChan)
		fs.eventChan = nil
	}

	return nil
}

// 私有方法

// detectFileType 检测文件类型
func detectFileType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".ini":
		return "ini"
	default:
		return "unknown"
	}
}

// initialLoad 初始加载
func (fs *FileSource) initialLoad() error {
	config, err := fs.loadFromFile()
	if err != nil {
		return err
	}

	fs.mutex.Lock()
	fs.cachedConfig = config
	fs.cachedTime = time.Now()

	// 获取文件信息用于变化检测
	if info, err := os.Stat(fs.filePath); err == nil {
		fs.lastModTime = info.ModTime()
		fs.lastSize = info.Size()
	}
	fs.mutex.Unlock()

	return nil
}

// loadFromFile 从文件加载配置
func (fs *FileSource) loadFromFile() (map[string]interface{}, error) {
	data, err := ioutil.ReadFile(fs.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var result map[string]interface{}

	switch fs.fileType {
	case "json":
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	case "yaml":
		// TODO: 实现YAML解析
		return nil, fmt.Errorf("YAML parsing not yet implemented")
	case "toml":
		// TODO: 实现TOML解析
		return nil, fmt.Errorf("TOML parsing not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported file type: %s", fs.fileType)
	}

	return result, nil
}

// copyConfig 复制配置（深拷贝）
func (fs *FileSource) copyConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}

	// 使用JSON序列化进行深拷贝
	data, err := json.Marshal(config)
	if err != nil {
		return make(map[string]interface{})
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result
}

// onFileChange 文件变化回调
func (fs *FileSource) onFileChange() {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	// 获取当前文件信息
	info, err := os.Stat(fs.filePath)
	if err != nil {
		return
	}

	// 检查文件是否真的发生了变化
	if info.ModTime().Equal(fs.lastModTime) && info.Size() == fs.lastSize {
		return
	}

	// 加载新配置
	newConfig, err := fs.loadFromFile()
	if err != nil {
		// 发送错误事件
		fs.sendEvent("", nil, fmt.Errorf("config load error: %w", err))
		return
	}

	// 比较配置变化
	if fs.cachedConfig != nil {
		fs.detectChangesAndSendEvents(fs.cachedConfig, newConfig)
	}

	// 更新缓存和文件信息
	fs.cachedConfig = newConfig
	fs.cachedTime = time.Now()
	fs.lastModTime = info.ModTime()
	fs.lastSize = info.Size()
}

// detectChangesAndSendEvents 检测变化并发送事件
func (fs *FileSource) detectChangesAndSendEvents(oldConfig, newConfig map[string]interface{}) {
	// 检测新增和修改的配置项
	for key, newValue := range newConfig {
		if oldValue, exists := oldConfig[key]; !exists {
			// 新增配置项
			fs.sendEvent(key, newValue, nil)
		} else if !isValueEqual(oldValue, newValue) {
			// 修改配置项
			fs.sendEvent(key, newValue, oldValue)
		}
	}

	// 检测删除的配置项
	for key := range oldConfig {
		if _, exists := newConfig[key]; !exists {
			// 删除配置项
			fs.sendEvent(key, nil, oldConfig[key])
		}
	}
}

// sendEvent 发送变更事件
func (fs *FileSource) sendEvent(key string, newValue, oldValue interface{}) {
	event := config.ConfigChangeEvent{
		Source:    fs.name,
		Key:       key,
		NewValue:  newValue,
		OldValue:  oldValue,
		Timestamp: time.Now(),
	}

	select {
	case fs.eventChan <- event:
	default:
		// 事件通道满了，丢弃事件
	}
}

// isValueEqual 比较两个值是否相等
func isValueEqual(a, b interface{}) bool {
	// 使用JSON序列化来比较复杂类型
	aJSON, errA := json.Marshal(a)
	if errA != nil {
		return false
	}

	bJSON, errB := json.Marshal(b)
	if errB != nil {
		return false
	}

	return string(aJSON) == string(bJSON)
}