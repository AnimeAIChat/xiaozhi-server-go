package core

import (
	"context"
	"fmt"
	"sync"
	"xiaozhi-server-go/internal/util/work"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/providers/tts"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"

	"github.com/sashabaranov/go-openai"
)

// AsyncTaskType 异步任务类型
type AsyncTaskType string

const (
	TaskTypeLLM AsyncTaskType = "llm"
	TaskTypeTTS AsyncTaskType = "tts"
)

// AsyncTask 异步任务结构
type AsyncTask struct {
	Type      AsyncTaskType
	Priority  int
	SessionID string
	Data      interface{}
	Callback  func(result interface{}, err error)
}

// AsyncTaskProcessor 异步任务处理器
type AsyncTaskProcessor struct {
	llmQueue  *work.WorkQueue[*AsyncTask]
	ttsQueue  *work.WorkQueue[*AsyncTask]
	providers interface {
		GetLLM() providers.LLMProvider
		GetTTS() providers.TTSProvider
	}
	logger *utils.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAsyncTaskProcessor 创建异步任务处理器
func NewAsyncTaskProcessor(llmProvider providers.LLMProvider, ttsProvider providers.TTSProvider, logger *utils.Logger) *AsyncTaskProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	processor := &AsyncTaskProcessor{
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	// 创建LLM工作队列，3个工作者
	processor.llmQueue = work.NewWorkQueue[*AsyncTask](3, processor.processLLMTask)

	// 创建TTS工作队列，2个工作者
	processor.ttsQueue = work.NewWorkQueue[*AsyncTask](2, processor.processTTSTask)

	// 存储提供者引用
	processor.providers = &providerWrapper{
		llm: llmProvider,
		tts: ttsProvider,
	}

	return processor
}

// providerWrapper 提供者包装器
type providerWrapper struct {
	llm providers.LLMProvider
	tts providers.TTSProvider
}

func (w *providerWrapper) GetLLM() providers.LLMProvider {
	return w.llm
}

func (w *providerWrapper) GetTTS() providers.TTSProvider {
	return w.tts
}

// SubmitLLMTask 提交LLM任务
func (p *AsyncTaskProcessor) SubmitLLMTask(sessionID string, messages []providers.Message, priority int, callback func(result interface{}, err error)) {
	task := &AsyncTask{
		Type:      TaskTypeLLM,
		Priority:  priority,
		SessionID: sessionID,
		Data: map[string]interface{}{
			"messages": messages,
		},
		Callback: callback,
	}

	p.llmQueue.SubmitWithRetries(task, priority, 3)
}

// SubmitTTSTask 提交TTS任务
func (p *AsyncTaskProcessor) SubmitTTSTask(sessionID string, text string, textIndex int, round int, priority int, callback func(result interface{}, err error)) {
	task := &AsyncTask{
		Type:      TaskTypeTTS,
		Priority:  priority,
		SessionID: sessionID,
		Data: map[string]interface{}{
			"text":      text,
			"textIndex": textIndex,
			"round":     round,
		},
		Callback: callback,
	}

	p.ttsQueue.SubmitWithRetries(task, priority, 2)
}

// processLLMTask 处理LLM任务
func (p *AsyncTaskProcessor) processLLMTask(task *AsyncTask) error {
	data := task.Data.(map[string]interface{})
	messages := data["messages"].([]providers.Message)

	llmProvider := p.providers.GetLLM()

	// 设置会话ID到LLM提供者
	if publisher := llm.GetEventPublisher(llmProvider); publisher != nil {
		publisher.SetSessionID(task.SessionID)
	}

	// 获取工具列表（如果需要）
	tools := []openai.Tool{} // 这里需要从某个地方获取工具列表

	// 调用LLM
	responses, err := llmProvider.ResponseWithFunctions(p.ctx, task.SessionID, messages, tools)
	if err != nil {
		p.logger.Error(fmt.Sprintf("LLM任务处理失败: %v", err))
		if task.Callback != nil {
			task.Callback(nil, err)
		}
		return err
	}

	// 收集所有响应
	var allResponses []types.Response
	for response := range responses {
		allResponses = append(allResponses, response)
	}

	// 调用回调
	if task.Callback != nil {
		task.Callback(allResponses, nil)
	}

	return nil
}

// processTTSTask 处理TTS任务
func (p *AsyncTaskProcessor) processTTSTask(task *AsyncTask) error {
	data := task.Data.(map[string]interface{})
	text := data["text"].(string)
	textIndex := data["textIndex"].(int)
	round := data["round"].(int)

	ttsProvider := p.providers.GetTTS()

	// 设置会话ID到TTS提供者
	if publisher := tts.GetEventPublisher(ttsProvider); publisher != nil {
		publisher.SetSessionID(task.SessionID)
	}

	// 生成语音文件
	filepath, err := ttsProvider.ToTTS(text)
	if err != nil {
		p.logger.Error(fmt.Sprintf("TTS任务处理失败: %v", err))
		if task.Callback != nil {
			task.Callback(nil, err)
		}
		return err
	}

	result := map[string]interface{}{
		"filepath": filepath,
		"text":     text,
		"textIndex": textIndex,
		"round":    round,
	}

	// 调用回调
	if task.Callback != nil {
		task.Callback(result, nil)
	}

	return nil
}

// Stop 停止异步任务处理器
func (p *AsyncTaskProcessor) Stop() {
	p.cancel()
	p.llmQueue.Stop()
	p.ttsQueue.Stop()
	p.wg.Wait()
}

// GetStats 获取队列统计信息
func (p *AsyncTaskProcessor) GetStats() map[string]interface{} {
	llmSize, llmEmpty := p.llmQueue.GetStats()
	ttsSize, ttsEmpty := p.ttsQueue.GetStats()

	return map[string]interface{}{
		"llm_queue_size": llmSize,
		"llm_queue_empty": llmEmpty,
		"tts_queue_size": ttsSize,
		"tts_queue_empty": ttsEmpty,
	}
}