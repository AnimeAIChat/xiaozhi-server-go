package webapi

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/image"
	providerbase "xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/asr"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/providers/tts"
	"xiaozhi-server-go/src/core/providers/vlllm"
)

const (
	providerTestTimeout      = 25 * time.Second
	asrTestSampleRate        = 16000
	ttsTestText              = "你好，这是语音合成服务测试。"
	maxTTSTestAudioBytes     = 2 * 1024 * 1024
	llmTestPrompt            = "请用一句简短中文介绍你能提供的帮助。"
	vlllmTestPrompt          = "请判断图片的主要颜色，只回答颜色名称。"
	vlllmTestImageBase64     = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAANSURBVBhXY/jPwPAfAAUAAf+mXJtdAAAAAElFTkSuQmCC"
	maxProviderResponseBytes = 16 * 1024
)

//go:embed testdata/asr-test.wav.b64
var providerTestAssets embed.FS

type liveProviderTestResult struct {
	message string
	data    map[string]interface{}
}

func (s *DefaultUserService) runLiveProviderTest(ctx context.Context, providerType, name, subType string, config map[string]interface{}) (liveProviderTestResult, error) {
	switch providerType {
	case "ASR":
		return s.testASRProvider(ctx, name, subType, config)
	case "TTS":
		return s.testTTSProvider(ctx, name, subType, config)
	case "LLM":
		return s.testLLMProvider(ctx, name, subType, config)
	case "VLLLM":
		return s.testVLLLMProvider(ctx, name, subType, config)
	default:
		return liveProviderTestResult{}, fmt.Errorf("不支持测试的 Provider 类型：%s", providerType)
	}
}

type asrProviderTestListener struct {
	result chan string
	once   sync.Once
}

type asrProviderTest interface {
	asr.Provider
	SetListener(providerbase.AsrEventListener)
	AddAudio([]byte) error
}

func (l *asrProviderTestListener) OnAsrResult(result string, _ bool) bool {
	result = strings.TrimSpace(result)
	if result == "" {
		return false
	}
	l.once.Do(func() { l.result <- result })
	return true
}

func (s *DefaultUserService) testASRProvider(ctx context.Context, name, subType string, config map[string]interface{}) (liveProviderTestResult, error) {
	testCtx, cancel := context.WithTimeout(ctx, providerTestTimeout)
	defer cancel()
	audio, err := loadProviderTestPCM()
	if err != nil {
		return liveProviderTestResult{}, err
	}
	created, err := asr.Create(subType, &asr.Config{Name: name, Type: subType, Data: config}, false, s.logger)
	if err != nil {
		return liveProviderTestResult{}, fmt.Errorf("ASR Provider 初始化失败：%w", err)
	}
	provider, ok := created.(asrProviderTest)
	if !ok {
		return liveProviderTestResult{}, fmt.Errorf("ASR Provider 类型 %s 不支持实时测试", subType)
	}
	if closeable, ok := provider.(interface{ CloseConnection() error }); ok {
		defer closeable.CloseConnection()
	}
	defer provider.Cleanup()

	listener := &asrProviderTestListener{result: make(chan string, 1)}
	provider.SetListener(listener)
	frameSize := asrTestSampleRate * 60 / 1000 * 2
	for len(audio) > 0 {
		n := frameSize
		if n > len(audio) {
			n = len(audio)
		}
		if err := addASRProviderTestAudio(testCtx, provider, audio[:n]); err != nil {
			return liveProviderTestResult{}, fmt.Errorf("ASR 测试请求失败：%w", err)
		}
		audio = audio[n:]
	}
	if finalizer, ok := provider.(interface{ SendLastAudio([]byte) error }); ok {
		if err := finalizer.SendLastAudio(nil); err != nil {
			return liveProviderTestResult{}, fmt.Errorf("ASR 完成请求失败：%w", err)
		}
	}
	select {
	case text := <-listener.result:
		return liveProviderTestResult{message: fmt.Sprintf("语音识别已返回结果：%s", text), data: map[string]interface{}{"level": "live", "text": text}}, nil
	case <-testCtx.Done():
		return liveProviderTestResult{}, errors.New("ASR 测试超时，未收到有效识别结果")
	}
}

func addASRProviderTestAudio(ctx context.Context, provider asrProviderTest, audio []byte) error {
	if withContext, ok := provider.(interface {
		AddAudioWithContext(context.Context, []byte) error
	}); ok {
		return withContext.AddAudioWithContext(ctx, audio)
	}
	return provider.AddAudio(audio)
}

func loadProviderTestPCM() ([]byte, error) {
	encoded, err := providerTestAssets.ReadFile("testdata/asr-test.wav.b64")
	if err != nil {
		return nil, fmt.Errorf("读取内置 ASR 测试音频失败：%w", err)
	}
	wav, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		return nil, fmt.Errorf("解析内置 ASR 测试音频失败：%w", err)
	}
	if len(wav) <= 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, errors.New("内置 ASR 测试音频格式无效")
	}
	var sampleRate uint32
	var pcm []byte
	for offset := 12; offset+8 <= len(wav); {
		chunkID := string(wav[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(wav[offset+4 : offset+8]))
		offset += 8
		if chunkSize < 0 || offset+chunkSize > len(wav) {
			return nil, errors.New("内置 ASR 测试音频数据损坏")
		}
		switch chunkID {
		case "fmt ":
			if chunkSize < 16 || binary.LittleEndian.Uint16(wav[offset:offset+2]) != 1 || binary.LittleEndian.Uint16(wav[offset+2:offset+4]) != 1 || binary.LittleEndian.Uint16(wav[offset+14:offset+16]) != 16 {
				return nil, errors.New("内置 ASR 测试音频必须为单声道 16 位 PCM")
			}
			sampleRate = binary.LittleEndian.Uint32(wav[offset+4 : offset+8])
		case "data":
			pcm = append([]byte(nil), wav[offset:offset+chunkSize]...)
		}
		offset += chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}
	if len(pcm) == 0 || sampleRate == 0 {
		return nil, errors.New("内置 ASR 测试音频缺少 PCM 数据")
	}
	return resamplePCM16Mono(pcm, sampleRate, asrTestSampleRate), nil
}

func resamplePCM16Mono(pcm []byte, sourceRate, targetRate uint32) []byte {
	if sourceRate == targetRate || len(pcm) < 4 {
		return pcm
	}
	sourceSamples := len(pcm) / 2
	targetSamples := int(uint64(sourceSamples) * uint64(targetRate) / uint64(sourceRate))
	output := make([]byte, targetSamples*2)
	for target := 0; target < targetSamples; target++ {
		source := int(uint64(target) * uint64(sourceRate) / uint64(targetRate))
		if source >= sourceSamples {
			source = sourceSamples - 1
		}
		copy(output[target*2:target*2+2], pcm[source*2:source*2+2])
	}
	return output
}

func (s *DefaultUserService) testTTSProvider(ctx context.Context, name, subType string, config map[string]interface{}) (liveProviderTestResult, error) {
	raw, _ := jsonProviderConfig(config)
	var providerConfig configs.TTSConfig
	if err := json.Unmarshal(raw, &providerConfig); err != nil {
		return liveProviderTestResult{}, fmt.Errorf("TTS Provider 配置格式无效：%w", err)
	}
	outputDir := strings.TrimSpace(providerConfig.OutputDir)
	if outputDir == "" {
		outputDir = os.TempDir()
	}
	provider, err := tts.Create(subType, &tts.Config{Name: name, Type: subType, OutputDir: outputDir, Voice: providerConfig.Voice, Format: providerConfig.Format, AppID: providerConfig.AppID, Token: providerConfig.Token, Cluster: providerConfig.Cluster, SupportedVoices: providerConfig.SupportedVoices}, false)
	if err != nil {
		return liveProviderTestResult{}, fmt.Errorf("TTS Provider 初始化失败：%w", err)
	}
	defer provider.Cleanup()
	audio, err := collectTTSProviderTest(ctx, provider)
	if err != nil {
		return liveProviderTestResult{}, fmt.Errorf("TTS 测试失败：%w", err)
	}
	return liveProviderTestResult{message: fmt.Sprintf("语音合成已返回音频（%d 字节）。", len(audio.data)), data: map[string]interface{}{"level": "live", "audio_base64": base64.StdEncoding.EncodeToString(audio.data), "audio_format": audio.format}}, nil
}

func jsonProviderConfig(config map[string]interface{}) ([]byte, error) { return json.Marshal(config) }

type providerTestAudio struct {
	data   []byte
	format string
}
type providerTTSResult struct {
	path string
	err  error
}

func collectTTSProviderTest(ctx context.Context, provider tts.Provider) (providerTestAudio, error) {
	completed := make(chan providerTTSResult, 1)
	go func() {
		path, err := provider.ToTTS(ttsTestText)
		completed <- providerTTSResult{path: path, err: err}
	}()
	var result providerTTSResult
	select {
	case result = <-completed:
	case <-ctx.Done():
		return providerTestAudio{}, errors.New("语音合成超时")
	}
	if result.err != nil {
		return providerTestAudio{}, result.err
	}
	if result.path == "" {
		return providerTestAudio{}, errors.New("语音合成未返回音频数据")
	}
	defer os.Remove(result.path)
	info, err := os.Stat(result.path)
	if err != nil || info.Size() == 0 {
		return providerTestAudio{}, errors.New("语音合成输出文件不可用")
	}
	if info.Size() > maxTTSTestAudioBytes {
		return providerTestAudio{}, fmt.Errorf("测试音频超过 %d MiB", maxTTSTestAudioBytes/(1024*1024))
	}
	data, err := os.ReadFile(result.path)
	if err != nil {
		return providerTestAudio{}, err
	}
	return normalizeProviderTestAudio(data, filepath.Ext(result.path))
}

func normalizeProviderTestAudio(data []byte, format string) (providerTestAudio, error) {
	format = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(format)), ".")
	if len(data) == 0 {
		return providerTestAudio{}, errors.New("语音合成未返回音频数据")
	}
	switch format {
	case "pcm", "raw", "s16le":
		return providerTestAudio{pcmToProviderTestWAV(data), "wav"}, nil
	case "wav", "wave":
		return providerTestAudio{data, "wav"}, nil
	case "mp3", "mpeg":
		return providerTestAudio{data, "mp3"}, nil
	case "ogg", "opus":
		return providerTestAudio{data, "ogg"}, nil
	default:
		return providerTestAudio{}, fmt.Errorf("不支持页面播放的音频格式：%s", format)
	}
}

func pcmToProviderTestWAV(pcm []byte) []byte {
	wav := make([]byte, 44+len(pcm))
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], 1)
	binary.LittleEndian.PutUint32(wav[24:28], 24000)
	binary.LittleEndian.PutUint32(wav[28:32], 48000)
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav
}

func (s *DefaultUserService) testLLMProvider(ctx context.Context, name, subType string, config map[string]interface{}) (liveProviderTestResult, error) {
	provider, err := llm.Create(subType, &llm.Config{Name: name, Type: subType, ModelName: stringConfig(config, "model_name"), BaseURL: stringConfig(config, "url"), APIKey: stringConfig(config, "api_key"), Extra: config})
	if err != nil {
		return liveProviderTestResult{}, fmt.Errorf("LLM Provider 初始化失败：%w", err)
	}
	defer provider.Cleanup()
	testCtx, cancel := context.WithTimeout(ctx, providerTestTimeout)
	defer cancel()
	response, err := provider.Response(testCtx, "provider-test", []providerbase.Message{{Role: "user", Content: llmTestPrompt}})
	if err != nil {
		return liveProviderTestResult{}, fmt.Errorf("LLM 测试请求失败：%w", err)
	}
	text, err := collectLiveResponse(testCtx, response)
	if err != nil {
		return liveProviderTestResult{}, err
	}
	return liveProviderTestResult{message: fmt.Sprintf("模型原始响应：%s", text), data: map[string]interface{}{"level": "live", "prompt": llmTestPrompt, "text": text}}, nil
}

func (s *DefaultUserService) testVLLLMProvider(ctx context.Context, name, subType string, config map[string]interface{}) (liveProviderTestResult, error) {
	cfg, err := decodeVLLLMTestConfig(config)
	if err != nil {
		return liveProviderTestResult{}, err
	}
	security := cfg.Security
	normalizeVLLLMTestSecurity(&security)
	provider, err := vlllm.NewProvider(&vlllm.Config{Type: subType, ModelName: cfg.ModelName, BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, TopP: cfg.TopP, Security: security, Data: cfg.Extra}, s.logger)
	if err == nil {
		err = provider.Initialize()
	}
	if err != nil {
		return liveProviderTestResult{}, fmt.Errorf("视觉模型初始化失败：%w", err)
	}
	defer provider.Cleanup()
	testCtx, cancel := context.WithTimeout(ctx, providerTestTimeout)
	defer cancel()
	response, err := provider.ResponseWithImage(testCtx, "provider-test", []providerbase.Message{}, image.ImageData{Data: vlllmTestImageBase64, Format: "png"}, vlllmTestPrompt)
	if err != nil {
		return liveProviderTestResult{}, fmt.Errorf("视觉模型测试请求失败：%w", err)
	}
	text, err := collectLiveResponse(testCtx, response)
	if err != nil {
		return liveProviderTestResult{}, err
	}
	return liveProviderTestResult{message: fmt.Sprintf("视觉模型已返回结果：%s", text), data: map[string]interface{}{"level": "live", "text": text}}, nil
}

func decodeVLLLMTestConfig(config map[string]interface{}) (configs.VLLMConfig, error) {
	normalized := make(map[string]interface{}, len(config))
	for key, value := range config {
		normalized[key] = value
	}
	for _, key := range []string{"max_tokens"} {
		value, err := normalizeProviderInteger(normalized[key], key)
		if err != nil {
			return configs.VLLMConfig{}, err
		}
		if value != nil {
			normalized[key] = value
		}
	}
	for _, key := range []string{"temperature", "top_p"} {
		value, err := normalizeProviderFloat(normalized[key], key)
		if err != nil {
			return configs.VLLMConfig{}, err
		}
		if value != nil {
			normalized[key] = value
		}
	}
	raw, err := jsonProviderConfig(normalized)
	if err != nil {
		return configs.VLLMConfig{}, fmt.Errorf("视觉模型配置格式无效：%w", err)
	}
	var cfg configs.VLLMConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return configs.VLLMConfig{}, fmt.Errorf("视觉模型配置格式无效：%w", err)
	}
	return cfg, nil
}

func normalizeProviderInteger(value interface{}, field string) (interface{}, error) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return value, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return nil, fmt.Errorf("视觉模型配置格式无效：%s 必须是整数", field)
	}
	return parsed, nil
}

func normalizeProviderFloat(value interface{}, field string) (interface{}, error) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return value, nil
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return nil, fmt.Errorf("视觉模型配置格式无效：%s 必须是数值", field)
	}
	return parsed, nil
}

func normalizeVLLLMTestSecurity(security *configs.SecurityConfig) {
	if security.MaxFileSize <= 0 {
		security.MaxFileSize = 10 * 1024 * 1024
	}
	if security.MaxPixels <= 0 {
		security.MaxPixels = 16 * 1024 * 1024
	}
	if security.MaxWidth <= 0 {
		security.MaxWidth = 4096
	}
	if security.MaxHeight <= 0 {
		security.MaxHeight = 4096
	}
	if len(security.AllowedFormats) == 0 {
		security.AllowedFormats = []string{"jpeg", "jpg", "png", "webp", "gif"}
	}
}

func collectLiveResponse(ctx context.Context, response <-chan string) (string, error) {
	var text strings.Builder
	for {
		select {
		case chunk, ok := <-response:
			if !ok {
				result := strings.TrimSpace(text.String())
				if result == "" {
					return "", errors.New("模型未返回任何内容")
				}
				if strings.HasPrefix(result, "【") {
					return "", errors.New(result)
				}
				return result, nil
			}
			if text.Len()+len(chunk) > maxProviderResponseBytes {
				return "", errors.New("测试响应过长")
			}
			text.WriteString(chunk)
		case <-ctx.Done():
			return "", errors.New("测试超时，未收到有效响应")
		}
	}
}
