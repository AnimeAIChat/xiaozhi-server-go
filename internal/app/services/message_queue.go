package services

import (
"context"
"sync"
"time"

"xiaozhi-server-go/internal/platform/logging"
coreproviders "xiaozhi-server-go/internal/core/providers"
"xiaozhi-server-go/internal/util"
)

// MessageQueueService 处理消息队列相关的业务逻辑
type MessageQueueService struct {
logger *logging.Logger

// 消息队列
clientTextQueue *util.Queue[string]
clientAudioQueue *util.Queue[[]byte]
asrResultQueue  *util.Queue[string]

// 提供者
asrProvider coreproviders.ASRProvider

// 控制信号
stopChan chan struct{}
running  bool
mu       sync.RWMutex
}

// MessageQueueConfig 消息队列服务配置
type MessageQueueConfig struct {
Logger      *logging.Logger
ASRProvider coreproviders.ASRProvider
}

// NewMessageQueueService 创建新的消息队列服务
func NewMessageQueueService(config *MessageQueueConfig) *MessageQueueService {
return &MessageQueueService{
logger:           config.Logger,
asrProvider:      config.ASRProvider,
clientTextQueue:  util.NewQueue[string](100),
clientAudioQueue: util.NewQueue[[]byte](100),
asrResultQueue:   util.NewQueue[string](100),
stopChan:         make(chan struct{}),
}
}

// Start 启动消息队列处理
func (s *MessageQueueService) Start() {
s.mu.Lock()
defer s.mu.Unlock()

if s.running {
return
}

s.running = true
go s.processQueues()
}

// Stop 停止消息队列处理
func (s *MessageQueueService) Stop() {
s.mu.Lock()
defer s.mu.Unlock()

if !s.running {
return
}

s.running = false
close(s.stopChan)
}

// GetStopChan 获取停止信号通道
func (s *MessageQueueService) GetStopChan() <-chan struct{} {
return s.stopChan
}

// EnqueueClientText 将客户端文本消息加入队列
func (s *MessageQueueService) EnqueueClientText(text string) {
s.clientTextQueue.Push(text)
}

// EnqueueClientAudio 将客户端音频数据加入队列
func (s *MessageQueueService) EnqueueClientAudio(audioData []byte) {
s.clientAudioQueue.Push(audioData)
}

// EnqueueASRResult 将ASR结果加入队列
func (s *MessageQueueService) EnqueueASRResult(result string) {
s.asrResultQueue.Push(result)
}

// GetAsrProvider 获取ASR提供者
func (s *MessageQueueService) GetAsrProvider() coreproviders.ASRProvider {
return s.asrProvider
}

// ClearQueues 清空所有队列
func (s *MessageQueueService) ClearQueues() {
s.clientTextQueue.Clear()
s.clientAudioQueue.Clear()
s.asrResultQueue.Clear()
}

// processQueues 处理队列中的消息
func (s *MessageQueueService) processQueues() {
ticker := time.NewTicker(100 * time.Millisecond)
defer ticker.Stop()

for {
select {
case <-s.stopChan:
return
case <-ticker.C:
s.processClientText()
s.processClientAudio()
s.processASRResult()
}
}
}

// processClientText 处理客户端文本消息
func (s *MessageQueueService) processClientText() {
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
defer cancel()

for {
text, err := s.clientTextQueue.Pop(ctx, -1) // non-blocking
if err != nil {
break
}

// 处理文本消息的逻辑可以在这里添加
// 目前只是示例实现
s.logger.Legacy().Debug("Processing client text: " + text)
}
}

// processClientAudio 处理客户端音频数据
func (s *MessageQueueService) processClientAudio() {
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
defer cancel()

for {
audioData, err := s.clientAudioQueue.Pop(ctx, -1) // non-blocking
if err != nil {
break
}

// 处理音频数据的逻辑可以在这里添加
// 目前只是示例实现
s.logger.Legacy().Debug("Processing client audio data, size: " + string(rune(len(audioData))))
}
}

// processASRResult 处理ASR结果
func (s *MessageQueueService) processASRResult() {
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
defer cancel()

for {
result, err := s.asrResultQueue.Pop(ctx, -1) // non-blocking
if err != nil {
break
}

// 处理ASR结果的逻辑可以在这里添加
// 目前只是示例实现
s.logger.Legacy().Debug("Processing ASR result: " + result)
}
}
