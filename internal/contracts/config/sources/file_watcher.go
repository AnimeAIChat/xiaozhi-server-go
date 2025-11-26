package sources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher 文件监听器
// 使用fsnotify库来监听文件变化
type FileWatcher struct {
	filePath string
	dirPath  string
	fileName string

	// fsnotify watcher
	watcher *fsnotify.Watcher

	// 回调函数
	onChange func()

	// 状态管理
	ctx      context.Context
	cancel   context.CancelFunc
	running bool
	mutex   sync.RWMutex
}

// FileChangeCallback 文件变化回调函数类型
type FileChangeCallback func()

// NewFileWatcher 创建文件监听器
func NewFileWatcher(filePath string, callback FileChangeCallback) (*FileWatcher, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	if callback == nil {
		return nil, fmt.Errorf("callback cannot be nil")
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	dirPath := filepath.Dir(absPath)
	fileName := filepath.Base(absPath)

	// 创建fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FileWatcher{
		filePath: absPath,
		dirPath:  dirPath,
		fileName: fileName,
		watcher:  watcher,
		onChange: callback,
		ctx:      ctx,
		cancel:   cancel,
	}

	return fw, nil
}

// Start 启动文件监听
func (fw *FileWatcher) Start(ctx context.Context) error {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	if fw.running {
		return fmt.Errorf("file watcher is already running")
	}

	// 监听目录变化
	if err := fw.watcher.Add(fw.dirPath); err != nil {
		return fmt.Errorf("failed to add directory to watcher: %w", err)
	}

	fw.running = true

	// 启动监听协程
	go fw.watchLoop(ctx)

	return nil
}

// Stop 停止文件监听
func (fw *FileWatcher) Stop() error {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	if !fw.running {
		return nil
	}

	fw.running = false
	fw.cancel()

	if fw.watcher != nil {
		fw.watcher.Close()
	}

	return nil
}

// watchLoop 监听循环
func (fw *FileWatcher) watchLoop(ctx context.Context) {
	// 防抖定时器
	var debounceTimer *time.Timer
	debounceDelay := 100 * time.Millisecond

	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fw.ctx.Done():
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// 只处理我们关心的文件
			if filepath.Base(event.Name) != fw.fileName {
				continue
			}

			// 过滤不需要的事件
			if event.Op&fsnotify.Write == fsnotify.Write {
				fw.handleWriteEvent(&debounceTimer, debounceDelay)
			} else if event.Op&fsnotify.Create == fsnotify.Create {
				fw.handleWriteEvent(&debounceTimer, debounceDelay)
			} else if event.Op&fsnotify.Remove == fsnotify.Remove {
				fw.handleRemoveEvent()
			} else if event.Op&fsnotify.Rename == fsnotify.Rename {
				// 重命名事件可能意味着文件被移动或删除
				fw.handleRenameEvent()
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			// 记录错误但继续监听
			fmt.Printf("File watcher error: %v\n", err)
		}
	}
}

// handleWriteEvent 处理写入事件（防抖）
func (fw *FileWatcher) handleWriteEvent(timerRef **time.Timer, delay time.Duration) {
	// 停止之前的定时器
	if *timerRef != nil {
		(*timerRef).Stop()
	}

	// 设置新的定时器
	*timerRef = time.AfterFunc(delay, func() {
		select {
		case <-fw.ctx.Done():
			return
		default:
			if fw.onChange != nil {
				fw.onChange()
			}
		}
	})
}

// handleRemoveEvent 处理删除事件
func (fw *FileWatcher) handleRemoveEvent() {
	select {
	case <-fw.ctx.Done():
		return
	default:
		if fw.onChange != nil {
			fw.onChange()
		}
	}
}

// handleRenameEvent 处理重命名事件
func (fw *FileWatcher) handleRenameEvent() {
	// 检查文件是否还存在
	if _, err := os.Stat(fw.filePath); os.IsNotExist(err) {
		// 文件不存在，可能是被删除或重命名
		fw.handleRemoveEvent()
	} else {
		// 文件仍然存在，可能是被修改
		fw.handleWriteEvent(nil, 50*time.Millisecond)
	}
}

// IsRunning 检查是否正在运行
func (fw *FileWatcher) IsRunning() bool {
	fw.mutex.RLock()
	defer fw.mutex.RUnlock()
	return fw.running
}

// GetFilePath 获取监听的文件路径
func (fw *FileWatcher) GetFilePath() string {
	return fw.filePath
}