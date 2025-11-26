package webrtc

import (
	"xiaozhi-server-go/internal/core/providers/vad"
	"xiaozhi-server-go/internal/domain/vad/inter"
	"xiaozhi-server-go/internal/domain/vad/webrtc_vad"
)

// Provider WebRTC VAD 提供者
type Provider struct {
	*vad.BaseProvider
	vadInstance interface{} // 底层 VAD 实例
}

// NewProvider 创建 WebRTC VAD 提供者
func NewProvider(config *vad.Config) (vad.Provider, error) {
	base := vad.NewBaseProvider(config)

	// 创建配置映射
	vadConfig := map[string]interface{}{
		"sample_rate": float64(config.SampleRate),
		"channels":    float64(config.Channels),
	}

	// 添加额外配置
	for k, v := range config.Extra {
		vadConfig[k] = v
	}

	// 获取 VAD 实例
	vadInstance, err := webrtc_vad.AcquireVAD(vadConfig)
	if err != nil {
		return nil, err
	}

	return &Provider{
		BaseProvider: base,
		vadInstance:  vadInstance,
	}, nil
}

// ProcessAudio 处理音频数据
func (p *Provider) ProcessAudio(audioData []byte) (bool, error) {
	if vadProvider, ok := p.vadInstance.(interface{ ProcessAudio([]byte) (bool, error) }); ok {
		return vadProvider.ProcessAudio(audioData)
	}
	return false, nil
}

// Reset 重置VAD状态
func (p *Provider) Reset() {
	if vadProvider, ok := p.vadInstance.(interface{ Reset() }); ok {
		vadProvider.Reset()
	}
}

// Close 关闭VAD资源
func (p *Provider) Close() error {
	if vadProvider, ok := p.vadInstance.(interface{ Close() error }); ok {
		return vadProvider.Close()
	}
	return nil
}

// GetConfig 获取VAD配置
func (p *Provider) GetConfig() inter.VADConfig {
	if vadProvider, ok := p.vadInstance.(interface{ GetConfig() inter.VADConfig }); ok {
		return vadProvider.GetConfig()
	}
	return inter.VADConfig{}
}

func init() {
	vad.Register("webrtc", NewProvider)
}