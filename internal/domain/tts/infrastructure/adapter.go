package infrastructure

import (
	"context"
	"fmt"
	"io/ioutil"
	"sync"

	"xiaozhi-server-go/internal/domain/tts/inter"
	"xiaozhi-server-go/internal/domain/tts/repository"
	"xiaozhi-server-go/internal/core/providers"
	"xiaozhi-server-go/internal/utils"
	"xiaozhi-server-go/internal/platform/errors"
)

// ttsAdapter TTS适配器 - 桥接旧的provider接口到新的Repository接口
type ttsAdapter struct {
	mu       sync.RWMutex
	provider providers.TTSProvider
	logger   *utils.Logger
}

// NewTTSAdapter 创建TTS适配器
func NewTTSAdapter(providerType string, config inter.TTSConfig, logger *utils.Logger) (repository.TTSRepository, error) {
	// 注意：这里需要一个工厂方法来创建TTSProvider
	// 暂时返回nil，后面需要实现
	return &ttsAdapter{
		logger: logger,
	}, nil
}

func (t *ttsAdapter) SynthesizeText(ctx context.Context, req repository.SynthesizeRequest) (*repository.SynthesizeResult, error) {
	if t.provider == nil {
		return nil, errors.New(errors.KindDomain, "tts.adapter", "provider not initialized")
	}

	// 设置语音
	if err, _ := t.provider.SetVoice(req.Config.Voice); err != nil {
		return nil, errors.Wrap(errors.KindDomain, "tts.adapter.voice", "failed to set voice", err)
	}

	// 合成语音
	filePath, err := t.provider.ToTTS(req.Text)
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "tts.adapter.synthesize", "failed to synthesize text", err)
	}

	// 读取音频文件内容
	audioData, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "tts.adapter.read", "failed to read audio file", err)
	}

	result := &repository.SynthesizeResult{
		AudioData: audioData,
		FilePath:  filePath,
		Format:    req.Config.Format,
		SampleRate: req.Config.SampleRate,
		Duration:  0, // TODO: 计算实际时长
	}

	return result, nil
}

func (t *ttsAdapter) StreamSynthesize(ctx context.Context, req repository.SynthesizeRequest) (<-chan repository.AudioChunk, error) {
	chunkChan := make(chan repository.AudioChunk, 1)

	// 对于大多数TTS提供者，流式合成不是实时的，所以我们直接返回完整结果
	go func() {
		defer close(chunkChan)

		result, err := t.SynthesizeText(ctx, req)
		if err != nil {
			chunk := repository.AudioChunk{
				AudioData: []byte(fmt.Sprintf("Error: %v", err)),
				IsFinal:   true,
				Done:      true,
			}
			select {
			case chunkChan <- chunk:
			default:
			}
			return
		}

		chunk := repository.AudioChunk{
			AudioData: result.AudioData,
			IsFinal:   true,
			Done:      true,
		}

		select {
		case chunkChan <- chunk:
		default:
		}
	}()

	return chunkChan, nil
}

func (t *ttsAdapter) ValidateProvider(ctx context.Context, config inter.TTSConfig) error {
	// 创建一个临时测试来验证连接
	tempAdapter, err := NewTTSAdapter(config.Provider, config, t.logger)
	if err != nil {
		return err
	}

	// 清理临时提供者
	if closer, ok := tempAdapter.(interface{ Close() error }); ok {
		closer.Close()
	}

	return nil
}

func (t *ttsAdapter) GetProviderInfo(provider string) (*repository.ProviderInfo, error) {
	switch provider {
	case "doubao":
		return &repository.ProviderInfo{
			Name:             "Doubao TTS",
			SupportedVoices:  []string{"BV001_streaming", "BV002_streaming"},
			SupportedFormats: []string{"wav", "mp3", "opus"},
			MaxTextLength:    1000,
			Features:         []string{"realtime", "streaming", "multiple-voices"},
		}, nil
	case "edge":
		return &repository.ProviderInfo{
			Name:             "Microsoft Edge TTS",
			SupportedVoices:  []string{"zh-CN-XiaoxiaoNeural", "zh-CN-YunxiNeural"},
			SupportedFormats: []string{"mp3", "wav"},
			MaxTextLength:    2000,
			Features:         []string{"high-quality", "multiple-languages"},
		}, nil
	case "cosyvoice":
		return &repository.ProviderInfo{
			Name:             "CosyVoice TTS",
			SupportedVoices:  []string{"default"},
			SupportedFormats: []string{"wav", "mp3"},
			MaxTextLength:    500,
			Features:         []string{"high-quality", "emotional"},
		}, nil
	default:
		return nil, errors.New(errors.KindDomain, "tts.adapter.info", fmt.Sprintf("unknown provider: %s", provider))
	}
}

func (t *ttsAdapter) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.provider != nil {
		return t.provider.Cleanup()
	}
	return nil
}