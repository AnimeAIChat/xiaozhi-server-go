package infrastructure

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xiaozhi-server-go/internal/domain/asr/inter"
	"xiaozhi-server-go/internal/domain/asr/repository"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/internal/platform/errors"
)

// asrAdapter ASR适配器 - 桥接旧的provider接口到新的Repository接口
type asrAdapter struct {
	mu       sync.RWMutex
	provider providers.ASRProvider
	logger   *utils.Logger
}

// NewASRAdapter 创建ASR适配器
func NewASRAdapter(providerType string, config inter.ASRConfig, logger *utils.Logger) (repository.ASRRepository, error) {
	// 注意：这里需要一个工厂方法来创建ASRProvider
	// 暂时返回nil，后面需要实现
	return &asrAdapter{
		logger: logger,
	}, nil
}

func (a *asrAdapter) ProcessAudio(ctx context.Context, req repository.ProcessAudioRequest) (*repository.ProcessAudioResult, error) {
	if a.provider == nil {
		return nil, errors.New(errors.KindDomain, "asr.adapter", "provider not initialized")
	}

	// 使用Transcribe方法进行音频识别
	text, err := a.provider.Transcribe(ctx, req.AudioData)
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "asr.adapter.transcribe", "failed to transcribe audio", err)
	}

	result := &repository.ProcessAudioResult{
		Text:      text,
		IsFinal:   true,
		StartTime: time.Now().UnixMilli(),
		EndTime:   time.Now().UnixMilli(),
	}

	return result, nil
}

func (a *asrAdapter) StreamAudio(ctx context.Context, req repository.ProcessAudioRequest) (<-chan repository.AudioChunk, error) {
	if a.provider == nil {
		return nil, errors.New(errors.KindDomain, "asr.adapter", "provider not initialized")
	}

	chunkChan := make(chan repository.AudioChunk, 10)

	// 对于流式处理，我们使用AddAudio和SendLastAudio方法
	go func() {
		defer close(chunkChan)

		// 设置事件监听器
		listener := &asrStreamEventListener{
			chunkChan: chunkChan,
		}
		a.provider.SetListener(listener)

		// 添加音频数据
		if err := a.provider.AddAudio(req.AudioData); err != nil {
			chunk := repository.AudioChunk{
				Text: fmt.Sprintf("Error: %v", err),
				Done: true,
			}
			select {
			case chunkChan <- chunk:
			default:
			}
			return
		}

		// 发送最后一块数据
		if err := a.provider.SendLastAudio([]byte{}); err != nil {
			chunk := repository.AudioChunk{
				Text: fmt.Sprintf("Error: %v", err),
				Done: true,
			}
			select {
			case chunkChan <- chunk:
			default:
			}
			return
		}
	}()

	return chunkChan, nil
}

func (a *asrAdapter) ValidateProvider(ctx context.Context, config inter.ASRConfig) error {
	// 创建一个临时的提供者来验证连接
	tempAdapter, err := NewASRAdapter(config.Provider, config, a.logger)
	if err != nil {
		return err
	}

	// 清理临时提供者
	if closer, ok := tempAdapter.(interface{ Close() error }); ok {
		closer.Close()
	}

	return nil
}

func (a *asrAdapter) GetProviderInfo(provider string) (*repository.ProviderInfo, error) {
	switch provider {
	case "doubao":
		return &repository.ProviderInfo{
			Name:             "Doubao ASR",
			SupportedFormats: []string{"wav", "pcm", "opus"},
			MaxAudioLength:   300000, // 5分钟
			Features:         []string{"realtime", "streaming", "noise-reduction"},
		}, nil
	case "deepgram":
		return &repository.ProviderInfo{
			Name:             "Deepgram ASR",
			SupportedFormats: []string{"wav", "mp3", "flac"},
			MaxAudioLength:   600000, // 10分钟
			Features:         []string{"realtime", "streaming", "multilingual"},
		}, nil
	case "gosherpa":
		return &repository.ProviderInfo{
			Name:             "GoSherpa ASR",
			SupportedFormats: []string{"wav", "pcm"},
			MaxAudioLength:   180000, // 3分钟
			Features:         []string{"offline", "local"},
		}, nil
	default:
		return nil, errors.New(errors.KindDomain, "asr.adapter.info", fmt.Sprintf("unknown provider: %s", provider))
	}
}

func (a *asrAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.provider != nil {
		return a.provider.Cleanup()
	}
	return nil
}

// asrEventListener ASR事件监听器
type asrEventListener struct {
	resultChan chan repository.ProcessAudioResult
	errorChan  chan error
}

func (l *asrEventListener) OnAsrResult(text string, isFinal bool) bool {
	if isFinal {
		result := repository.ProcessAudioResult{
			Text:     text,
			IsFinal:  isFinal,
			StartTime: time.Now().UnixMilli(),
			EndTime:   time.Now().UnixMilli(),
		}
		select {
		case l.resultChan <- result:
		default:
		}
	}
	return true
}

func (l *asrEventListener) OnAsrError(err error) {
	select {
	case l.errorChan <- err:
	default:
	}
}

// asrStreamEventListener 流式ASR事件监听器
type asrStreamEventListener struct {
	chunkChan chan repository.AudioChunk
}

func (l *asrStreamEventListener) OnAsrResult(text string, isFinal bool) bool {
	chunk := repository.AudioChunk{
		Text:      text,
		IsFinal:   isFinal,
		StartTime: time.Now().UnixMilli(),
		EndTime:   time.Now().UnixMilli(),
		Done:      false,
	}

	select {
	case l.chunkChan <- chunk:
		return true
	default:
		return false
	}
}

func (l *asrStreamEventListener) OnAsrError(err error) {
	// 发送错误块
	chunk := repository.AudioChunk{
		Text: fmt.Sprintf("Error: %v", err),
		Done: true,
	}

	select {
	case l.chunkChan <- chunk:
	default:
	}
}