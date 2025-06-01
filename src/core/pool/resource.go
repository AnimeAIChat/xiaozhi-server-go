package pool

import (
    "context"
    "sync"
    "time"
    "xiaozhi-server-go/src/core/utils"
)

/*
* 资源池资源类，初始化创建资源池，生成最少的资源数量，
* 维护资源池的大小，提供获取和释放资源的接口。
* 支持资源的创建和销毁，资源池的动态扩展和收缩。
* 资源池的维护协程会定期检查当前资源数量，
* 如果资源数量低于设定的补充阈值，则创建新的资源，

*/

// ResourceFactory 资源工厂接口
type ResourceFactory interface {
    Create() (interface{}, error)
    Destroy(resource interface{}) error
}

// ResourcePool 通用资源池
type ResourcePool struct {
    factory     ResourceFactory
    pool        chan interface{}
    minSize     int
    maxSize     int
    currentSize int
    mutex       sync.RWMutex
    logger      *utils.Logger
    ctx         context.Context
    cancel      context.CancelFunc
}

// PoolConfig 资源池配置
type PoolConfig struct {
    MinSize     int           // 最小资源数量
    MaxSize     int           // 最大资源数量
    RefillSize  int           // 补充阈值
    CheckInterval time.Duration // 检查间隔
}

// NewResourcePool 创建资源池
func NewResourcePool(factory ResourceFactory, config PoolConfig, logger *utils.Logger) (*ResourcePool, error) {
    ctx, cancel := context.WithCancel(context.Background())
    
    pool := &ResourcePool{
        factory: factory,
        pool:    make(chan interface{}, config.MaxSize),
        minSize: config.MinSize,
        maxSize: config.MaxSize,
        logger:  logger,
        ctx:     ctx,
        cancel:  cancel,
    }
    
    // 预创建最小数量的资源
    if err := pool.initializePool(); err != nil {
        cancel()
        return nil, err
    }
    
    // 启动资源池维护协程
    go pool.maintain(config.RefillSize, config.CheckInterval)
    
    return pool, nil
}

// Get 获取资源
func (p *ResourcePool) Get() (interface{}, error) {
    select {
    case resource := <-p.pool:
        p.mutex.Lock()
        p.currentSize--
        p.mutex.Unlock()
        return resource, nil
    default:
        // 池中没有资源，直接创建
        return p.factory.Create()
    }
}

// initializePool 初始化资源池
func (p *ResourcePool) initializePool() error {
    for i := 0; i < p.minSize; i++ {
        resource, err := p.factory.Create()
        if err != nil {
            return err
        }
        p.pool <- resource
        p.currentSize++
    }
    return nil
}

// maintain 维护资源池
func (p *ResourcePool) maintain(refillSize int, checkInterval time.Duration) {
    ticker := time.NewTicker(checkInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-p.ctx.Done():
            return
        case <-ticker.C:
            p.refillPool(refillSize)
        }
    }
}

// refillPool 补充资源池
func (p *ResourcePool) refillPool(refillSize int) {
    p.mutex.RLock()
    currentSize := p.currentSize
    p.mutex.RUnlock()
    
    if currentSize < refillSize {
        needCreate := refillSize - currentSize
        for i := 0; i < needCreate && currentSize < p.maxSize; i++ {
            resource, err := p.factory.Create()
            if err != nil {
                p.logger.Error("创建资源失败: %v", err)
                continue
            }
            
            select {
            case p.pool <- resource:
                p.mutex.Lock()
                p.currentSize++
                p.mutex.Unlock()
            default:
                // 池满了，销毁资源
                p.factory.Destroy(resource)
            }
        }
    }
}

// Close 关闭资源池
func (p *ResourcePool) Close() {
    p.cancel()
    close(p.pool)
    
    // 销毁剩余资源
    for resource := range p.pool {
        p.factory.Destroy(resource)
    }
}

// GetStats 获取池状态
func (p *ResourcePool) GetStats() (available, total int) {
    p.mutex.RLock()
    defer p.mutex.RUnlock()
    return len(p.pool), p.currentSize
}