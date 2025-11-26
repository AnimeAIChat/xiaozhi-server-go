package cache

import (
	"sync"
	"time"

	"xiaozhi-server-go/internal/contracts/config"
)

// MemoryCache 内存配置缓存实现
type MemoryCache struct {
	data       map[string]*cacheEntry
	mutex      sync.RWMutex
	defaultTTL time.Duration
	stats      config.CacheStats
	gcTicker   *time.Ticker
	gcDone     chan bool
}

// cacheEntry 缓存条目
type cacheEntry struct {
	value     interface{}
	expireTime time.Time
	accessTime time.Time
	hitCount   int64
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache() config.ConfigCache {
	return &MemoryCache{
		data:     make(map[string]*cacheEntry),
		gcDone:   make(chan bool),
	}
}

// Get 获取缓存的配置值
func (mc *MemoryCache) Get(key string) (interface{}, bool) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	entry, exists := mc.data[key]
	if !exists {
		mc.stats.Misses++
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(entry.expireTime) {
		delete(mc.data, key)
		mc.stats.Misses++
		return nil, false
	}

	// 更新访问时间和命中次数
	entry.accessTime = time.Now()
	entry.hitCount++
	mc.stats.Hits++

	return entry.value, true
}

// Set 设置配置值到缓存
func (mc *MemoryCache) Set(key string, value interface{}, ttl time.Duration) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	if key == "" {
		return nil
	}

	// 确定过期时间
	expireTime := time.Now().Add(ttl)
	if ttl <= 0 {
		expireTime = time.Now().Add(mc.defaultTTL)
	}

	// 如果缓存已满，执行LRU清理
	mc.evictIfNeeded()

	mc.data[key] = &cacheEntry{
		value:     value,
		expireTime: expireTime,
		accessTime: time.Now(),
		hitCount:   0,
	}

	mc.updateSize()
	return nil
}

// Delete 删除缓存项
func (mc *MemoryCache) Delete(key string) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	delete(mc.data, key)
	mc.updateSize()
	return nil
}

// Clear 清空缓存
func (mc *MemoryCache) Clear() error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	mc.data = make(map[string]*cacheEntry)
	mc.updateSize()
	return nil
}

// GetStats 获取缓存统计信息
func (mc *MemoryCache) GetStats() config.CacheStats {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	// 计算命中率
	total := mc.stats.Hits + mc.stats.Misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(mc.stats.Hits) / float64(total)
	}

	return config.CacheStats{
		Hits:      mc.stats.Hits,
		Misses:    mc.stats.Misses,
		Size:      int64(len(mc.data)),
		HitRate:   hitRate,
		Evictions: mc.stats.Evictions,
		LastGC:    mc.stats.LastGC,
	}
}

// SetTTL 设置默认TTL
func (mc *MemoryCache) SetTTL(ttl time.Duration) {
	mc.defaultTTL = ttl
}

// StartGC 启动垃圾回收
func (mc *MemoryCache) StartGC(interval time.Duration) {
	if mc.gcTicker != nil {
		return // 已经启动
	}

	mc.gcTicker = time.NewTicker(interval)
	go mc.gcLoop()
}

// StopGC 停止垃圾回收
func (mc *MemoryCache) StopGC() {
	if mc.gcTicker != nil {
		mc.gcTicker.Stop()
		mc.gcTicker = nil
	}

	// 等待GC循环结束
	select {
	case <-mc.gcDone:
	default:
	}
}

// 私有方法

// evictIfNeeded 如果需要，执行LRU清理
func (mc *MemoryCache) evictIfNeeded() {
	const maxSize = 1000 // 最大缓存项数

	if len(mc.data) < maxSize {
		return
	}

	// 找到最久未访问的项
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range mc.data {
		if oldestKey == "" || entry.accessTime.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.accessTime
		}
	}

	if oldestKey != "" {
		delete(mc.data, oldestKey)
		mc.stats.Evictions++
	}
}

// gcLoop 垃圾回收循环
func (mc *MemoryCache) gcLoop() {
	defer close(mc.gcDone)

	for {
		select {
		case <-mc.gcDone:
			return
		case <-mc.gcTicker.C:
			mc.performGC()
		}
	}
}

// performGC 执行垃圾回收
func (mc *MemoryCache) performGC() {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	now := time.Now()
	before := len(mc.data)

	// 清理过期项
	for key, entry := range mc.data {
		if now.After(entry.expireTime) {
			delete(mc.data, key)
		}
	}

	after := len(mc.data)
	mc.stats.LastGC = now

	// 如果清理了过期项，记录日志
	if before != after {
		// 可以在这里添加日志记录
	}
}

// updateSize 更新缓存大小统计
func (mc *MemoryCache) updateSize() {
	mc.stats.Size = int64(len(mc.data))
}