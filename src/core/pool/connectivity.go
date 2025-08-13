package pool

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"time"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/image"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/utils"
)

// CheckMode 检查模式
type CheckMode int

const (
	// BasicCheck 基础连通性检查（只验证连接和认证）
	BasicCheck CheckMode = iota
	// FunctionalCheck 功能性检查（执行实际的API调用测试）
	FunctionalCheck
)

// CheckResult 检查结果
type CheckResult struct {
	ProviderType string                 `json:"provider_type"`
	Success      bool                   `json:"success"`
	Error        error                  `json:"error,omitempty"`
	Details      map[string]interface{} `json:"details"`
	Duration     time.Duration          `json:"duration"`
	Timestamp    time.Time              `json:"timestamp"`
	CheckMode    CheckMode              `json:"check_mode"`
}

// ConnectivityConfig 连通性检查配置
type ConnectivityConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Timeout       time.Duration `yaml:"timeout"`
	RetryAttempts int           `yaml:"retry_attempts"`
	RetryDelay    time.Duration `yaml:"retry_delay"`
	TestModes     TestModes     `yaml:"test_modes"`
}

// TestModes 测试模式配置
type TestModes struct {
	ASRTestAudio  string `yaml:"asr_test_audio"`
	LLMTestPrompt string `yaml:"llm_test_prompt"`
	TTSTestText   string `yaml:"tts_test_text"`
}

// ConfigFromYAML 从YAML配置创建连通性检查配置
func ConfigFromYAML(yamlConfig *configs.ConnectivityCheckConfig) (*ConnectivityConfig, error) {
	if yamlConfig == nil {
		return DefaultConnectivityConfig(), nil
	}

	// 解析超时时间
	timeout := 30 * time.Second
	if yamlConfig.Timeout != "" {
		if t, err := time.ParseDuration(yamlConfig.Timeout); err == nil {
			timeout = t
		}
	}

	// 解析重试延迟
	retryDelay := 5 * time.Second
	if yamlConfig.RetryDelay != "" {
		if t, err := time.ParseDuration(yamlConfig.RetryDelay); err == nil {
			retryDelay = t
		}
	}

	// 设置重试次数，默认为3
	retryAttempts := 3
	if yamlConfig.RetryAttempts > 0 {
		retryAttempts = yamlConfig.RetryAttempts
	}

	return &ConnectivityConfig{
		Enabled:       yamlConfig.Enabled,
		Timeout:       timeout,
		RetryAttempts: retryAttempts,
		RetryDelay:    retryDelay,
		TestModes: TestModes{
			ASRTestAudio:  yamlConfig.TestModes.ASRTestAudio,
			LLMTestPrompt: yamlConfig.TestModes.LLMTestPrompt,
			TTSTestText:   yamlConfig.TestModes.TTSTestText,
		},
	}, nil
}

// DefaultConnectivityConfig 默认连通性检查配置
func DefaultConnectivityConfig() *ConnectivityConfig {
	return &ConnectivityConfig{
		Enabled:       true,
		Timeout:       30 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    5 * time.Second,
		TestModes: TestModes{
			ASRTestAudio:  "",
			LLMTestPrompt: "Hello",
			TTSTestText:   "测试",
		},
	}
}

// HealthChecker 统一健康检查管理器
type HealthChecker struct {
	config        *configs.Config
	connConfig    *ConnectivityConfig
	logger        *utils.Logger
	testGenerator *TestDataGenerator
	results       map[string]*CheckResult
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(
	config *configs.Config,
	connConfig *ConnectivityConfig,
	logger *utils.Logger,
) *HealthChecker {
	if connConfig == nil {
		connConfig = DefaultConnectivityConfig()
	}

	return &HealthChecker{
		config:        config,
		connConfig:    connConfig,
		logger:        logger,
		testGenerator: NewTestDataGenerator(connConfig.TestModes),
		results:       make(map[string]*CheckResult),
	}
}

// CheckAllProviders 检查所有配置的提供者
func (hc *HealthChecker) CheckAllProviders(ctx context.Context, mode CheckMode) error {
	if !hc.connConfig.Enabled {
		hc.logger.Info("连通性检查已禁用，跳过检查")
		return nil
	}

	checkTypeName := "基础连通性"
	if mode == FunctionalCheck {
		checkTypeName = "功能性"
	}
	hc.logger.Info("开始执行%s检查...", checkTypeName)

	selectedModule := hc.config.SelectedModule
	var allErrors []error

	// 检查ASR
	if asrType, ok := selectedModule["ASR"]; ok && asrType != "" {
		if err := hc.checkASRProvider(ctx, asrType, mode); err != nil {
			allErrors = append(allErrors, fmt.Errorf("ASR%s检查失败: %v", checkTypeName, err))
		}
	}

	// 检查LLM
	if llmType, ok := selectedModule["LLM"]; ok && llmType != "" {
		if err := hc.checkLLMProvider(ctx, llmType, mode); err != nil {
			allErrors = append(allErrors, fmt.Errorf("LLM%s检查失败: %v", checkTypeName, err))
		}
	}

	// 检查TTS
	if ttsType, ok := selectedModule["TTS"]; ok && ttsType != "" {
		if err := hc.checkTTSProvider(ctx, ttsType, mode); err != nil {
			allErrors = append(allErrors, fmt.Errorf("TTS%s检查失败: %v", checkTypeName, err))
		}
	}

	// 检查VLLLM（可选）
	if vlllmType, ok := selectedModule["VLLLM"]; ok && vlllmType != "" {
		if err := hc.checkVLLLMProvider(ctx, vlllmType, mode); err != nil {
			hc.logger.Warn("VLLLM%s检查失败，将继续使用普通LLM: %v", checkTypeName, err)
			// VLLLM是可选的，失败不会导致整体失败
		}
	}

	if len(allErrors) > 0 {
		hc.logger.Error("%s检查失败，详细信息:", checkTypeName)
		for _, err := range allErrors {
			hc.logger.Error("  - %v", err)
		}
		return fmt.Errorf("%s检查失败: %d个服务不可用", checkTypeName, len(allErrors))
	}

	hc.logger.Info("所有资源%s检查通过", checkTypeName)
	return nil
}

// checkASRProvider 检查ASR提供者
func (hc *HealthChecker) checkASRProvider(
	ctx context.Context,
	asrType string,
	mode CheckMode,
) error {
	hc.logger.Info("检查ASR提供者: %s", asrType)

	start := time.Now()
	result := &CheckResult{
		ProviderType: "ASR",
		Timestamp:    start,
		CheckMode:    mode,
		Details:      make(map[string]interface{}),
	}

	// 创建ASR实例
	asrFactory := NewASRFactory(asrType, hc.config, hc.logger)
	if asrFactory == nil {
		result.Success = false
		result.Error = fmt.Errorf("创建ASR工厂失败: 找不到配置 %s", asrType)
		result.Duration = time.Since(start)
		hc.results["ASR"] = result
		return result.Error
	}

	testInstance, err := hc.createWithRetry(ctx, asrFactory)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("创建ASR测试实例失败: %v", err)
		result.Duration = time.Since(start)
		hc.results["ASR"] = result
		return result.Error
	}

	defer asrFactory.Destroy(testInstance)

	// 如果是功能性检查，执行实际的API调用
	if mode == FunctionalCheck {
		asrProvider, ok := testInstance.(providers.ASRProvider)
		if !ok {
			result.Success = false
			result.Error = fmt.Errorf("实例不是有效的ASRProvider")
			result.Duration = time.Since(start)
			hc.results["ASR"] = result
			return result.Error
		}

		hc.logger.Info("执行ASR功能性测试...")

		// 生成测试音频数据
		testAudioData, err := hc.generateTestAudioData()
		if err != nil {
			hc.logger.Warn("生成测试音频失败，跳过功能性测试: %v", err)
			result.Details["functional_test"] = "skipped - audio generation failed"
		} else {
			// 执行实际的ASR测试
			testCtx, cancel := context.WithTimeout(ctx, hc.connConfig.Timeout)
			defer cancel()

			transcriptionResult, err := asrProvider.Transcribe(testCtx, testAudioData)
			if err != nil {
				result.Success = false
				result.Error = fmt.Errorf("ASR转录测试失败: %v", err)
				result.Duration = time.Since(start)
				hc.results["ASR"] = result
				return result.Error
			}

			hc.logger.Info("ASR转录结果: '%s' (长度: %d)", transcriptionResult, len(transcriptionResult))

			// 对于doubao ASR，由于是异步处理，可能立即返回空字符串
			// 这里我们认为能成功调用API且没有错误就算通过
			if err == nil {
				// ASR调用成功，即使返回空字符串也认为连通性正常
				result.Details["functional_test"] = "passed"
				result.Details["test_response_length"] = len(transcriptionResult)
				result.Details["note"] = "ASR调用成功，异步处理中"
				hc.logger.Info("ASR功能性测试通过，API调用成功 (异步处理)")
			} else {
				result.Success = false
				result.Error = fmt.Errorf("ASR响应验证失败: %v", err)
				result.Duration = time.Since(start)
				hc.results["ASR"] = result
				return result.Error
			}
		}
	}

	result.Success = true
	result.Duration = time.Since(start)
	result.Details["config_type"] = asrType
	hc.results["ASR"] = result

	checkType := "基础连通性"
	if mode == FunctionalCheck {
		checkType = "功能性"
	}
	hc.logger.Info("ASR提供者 %s %s检查通过", asrType, checkType)
	return nil
}

// checkLLMProvider 检查LLM提供者
func (hc *HealthChecker) checkLLMProvider(
	ctx context.Context,
	llmType string,
	mode CheckMode,
) error {
	hc.logger.Info("检查LLM提供者: %s", llmType)

	start := time.Now()
	result := &CheckResult{
		ProviderType: "LLM",
		Timestamp:    start,
		CheckMode:    mode,
		Details:      make(map[string]interface{}),
	}

	// 创建LLM实例
	llmFactory := NewLLMFactory(llmType, hc.config, hc.logger)
	if llmFactory == nil {
		result.Success = false
		result.Error = fmt.Errorf("创建LLM工厂失败: 找不到配置 %s", llmType)
		result.Duration = time.Since(start)
		hc.results["LLM"] = result
		return result.Error
	}

	testInstance, err := hc.createWithRetry(ctx, llmFactory)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("创建LLM测试实例失败: %v", err)
		result.Duration = time.Since(start)
		hc.results["LLM"] = result
		return result.Error
	}

	defer llmFactory.Destroy(testInstance)

	// 如果是功能性检查，执行实际的API调用
	if mode == FunctionalCheck {
		llmProvider, ok := testInstance.(providers.LLMProvider)
		if !ok {
			result.Success = false
			result.Error = fmt.Errorf("实例不是有效的LLMProvider")
			result.Duration = time.Since(start)
			hc.results["LLM"] = result
			return result.Error
		}

		hc.logger.Info("执行LLM功能性测试...")

		// 执行实际的LLM测试
		testPrompt := hc.testGenerator.GetTestPrompt()
		testCtx, cancel := context.WithTimeout(ctx, hc.connConfig.Timeout)
		defer cancel()

		messages := []providers.Message{
			{Role: "user", Content: testPrompt},
		}

		// 使用Response方法进行简单的文本响应测试
		responseChan, err := llmProvider.Response(testCtx, "health_check", messages)
		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("LLM响应测试失败: %v", err)
			result.Duration = time.Since(start)
			hc.results["LLM"] = result
			return result.Error
		}

		// 收集响应内容
		var response strings.Builder
		for content := range responseChan {
			response.WriteString(content)
		}
		responseText := response.String()

		// 验证响应
		if !hc.testGenerator.ValidateLLMResponse(responseText) {
			result.Success = false
			result.Error = fmt.Errorf("LLM响应验证失败: 响应内容不合理")
			result.Duration = time.Since(start)
			hc.results["LLM"] = result
			return result.Error
		}

		result.Details["functional_test"] = "passed"
		result.Details["test_response_length"] = len(responseText)
		hc.logger.Info("LLM功能性测试通过，内容：%s,响应长度: %d", responseText, len(responseText))
	}

	result.Success = true
	result.Duration = time.Since(start)
	result.Details["config_type"] = llmType
	hc.results["LLM"] = result

	checkType := "基础连通性"
	if mode == FunctionalCheck {
		checkType = "功能性"
	}
	hc.logger.Info("LLM提供者 %s %s检查通过", llmType, checkType)
	return nil
}

// checkTTSProvider 检查TTS提供者
func (hc *HealthChecker) checkTTSProvider(
	ctx context.Context,
	ttsType string,
	mode CheckMode,
) error {
	hc.logger.Info("检查TTS提供者: %s", ttsType)

	start := time.Now()
	result := &CheckResult{
		ProviderType: "TTS",
		Timestamp:    start,
		CheckMode:    mode,
		Details:      make(map[string]interface{}),
	}

	// 创建TTS实例
	ttsFactory := NewTTSFactory(ttsType, hc.config, hc.logger)
	if ttsFactory == nil {
		result.Success = false
		result.Error = fmt.Errorf("创建TTS工厂失败: 找不到配置 %s", ttsType)
		result.Duration = time.Since(start)
		hc.results["TTS"] = result
		return result.Error
	}

	testInstance, err := hc.createWithRetry(ctx, ttsFactory)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("创建TTS测试实例失败: %v", err)
		result.Duration = time.Since(start)
		hc.results["TTS"] = result
		return result.Error
	}

	defer ttsFactory.Destroy(testInstance)

	// 如果是功能性检查，执行实际的API调用
	if mode == FunctionalCheck {
		ttsProvider, ok := testInstance.(providers.TTSProvider)
		if !ok {
			result.Success = false
			result.Error = fmt.Errorf("实例不是有效的TTSProvider")
			result.Duration = time.Since(start)
			hc.results["TTS"] = result
			return result.Error
		}

		hc.logger.Info("执行TTS功能性测试...")

		// 执行实际的TTS测试
		testText := hc.testGenerator.GetTestTTSText()
		audioPath, err := ttsProvider.ToTTS(testText)
		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("TTS合成测试失败: %v", err)
			result.Duration = time.Since(start)
			hc.results["TTS"] = result
			return result.Error
		}

		// 验证响应
		if !hc.testGenerator.ValidateTTSResponse(audioPath) {
			result.Success = false
			result.Error = fmt.Errorf("TTS响应验证失败: 音频路径不合理")
			result.Duration = time.Since(start)
			hc.results["TTS"] = result
			return result.Error
		}

		result.Details["functional_test"] = "passed"
		result.Details["audio_path"] = audioPath
		hc.logger.Info("TTS功能性测试通过，音频文件: %s", audioPath)
	}

	result.Success = true
	result.Duration = time.Since(start)
	result.Details["config_type"] = ttsType
	hc.results["TTS"] = result

	checkType := "基础连通性"
	if mode == FunctionalCheck {
		checkType = "功能性"
	}
	hc.logger.Info("TTS提供者 %s %s检查通过", ttsType, checkType)
	return nil
}

// checkVLLLMProvider 检查VLLLM提供者
func (hc *HealthChecker) checkVLLLMProvider(
	ctx context.Context,
	vlllmType string,
	mode CheckMode,
) error {
	hc.logger.Info("检查VLLLM提供者: %s", vlllmType)

	start := time.Now()
	result := &CheckResult{
		ProviderType: "VLLLM",
		Timestamp:    start,
		CheckMode:    mode,
		Details:      make(map[string]interface{}),
	}

	// 创建VLLLM实例
	vlllmFactory := NewVLLLMFactory(vlllmType, hc.config, hc.logger)
	if vlllmFactory == nil {
		result.Success = false
		result.Error = fmt.Errorf("创建VLLLM工厂失败: 找不到配置 %s", vlllmType)
		result.Duration = time.Since(start)
		hc.results["VLLLM"] = result
		return result.Error
	}

	testInstance, err := hc.createWithRetry(ctx, vlllmFactory)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("创建VLLLM测试实例失败: %v", err)
		result.Duration = time.Since(start)
		hc.results["VLLLM"] = result
		return result.Error
	}

	defer vlllmFactory.Destroy(testInstance)

	// 如果是功能性检查，执行实际的API调用
	if mode == FunctionalCheck {
		vlllmProvider, ok := testInstance.(*vlllm.Provider)
		if !ok {
			result.Success = false
			result.Error = fmt.Errorf("实例不是有效的VLLLM Provider")
			result.Duration = time.Since(start)
			hc.results["VLLLM"] = result
			return result.Error
		}

		hc.logger.Info("执行VLLLM功能性测试...")

		// 获取测试图片和文本
		testImageData, err := hc.testGenerator.GetTestImageData()
		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("获取测试图片失败: %v", err)
			result.Duration = time.Since(start)
			hc.results["VLLLM"] = result
			return result.Error
		}

		testPrompt := "请描述这张图片"
		testCtx, cancel := context.WithTimeout(ctx, hc.connConfig.Timeout)
		defer cancel()

		// 创建测试用的ImageData - testImageData是[]byte，需要转换为base64字符串
		base64ImageData := base64.StdEncoding.EncodeToString(testImageData)
		imageData := image.ImageData{
			Data:   base64ImageData,
			Format: "png",
		}

		// 调用VLLLM的ResponseWithImage方法
		responseChan, err := vlllmProvider.ResponseWithImage(
			testCtx,
			"health_check",
			[]providers.Message{},
			imageData,
			testPrompt,
		)
		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("VLLLM图像分析测试失败: %v", err)
			result.Duration = time.Since(start)
			hc.results["VLLLM"] = result
			return result.Error
		}

		// 收集响应内容
		var response strings.Builder
		for content := range responseChan {
			response.WriteString(content)
		}
		responseText := response.String()

		// 验证响应
		if !hc.testGenerator.ValidateVLLLMResponse(responseText) {
			result.Success = false
			result.Error = fmt.Errorf("VLLLM响应验证失败: 响应内容不合理")
			result.Duration = time.Since(start)
			hc.results["VLLLM"] = result
			return result.Error
		}

		result.Details["functional_test"] = "passed"
		result.Details["test_response_length"] = len(responseText)
		hc.logger.Info("VLLLM功能性测试通过，响应长度: %d", len(responseText))
	}

	result.Success = true
	result.Duration = time.Since(start)
	result.Details["config_type"] = vlllmType
	hc.results["VLLLM"] = result

	checkType := "基础连通性"
	if mode == FunctionalCheck {
		checkType = "功能性"
	}
	hc.logger.Info("VLLLM提供者 %s %s检查通过", vlllmType, checkType)
	return nil
}

// generateTestAudioData 生成测试音频数据
func (hc *HealthChecker) generateTestAudioData() ([]byte, error) {
	// 生成约100ms的16kHz 16位单声道PCM音频数据
	sampleRate := 16000
	bitsPerSample := 16
	channels := 1
	durationMs := 100

	samplesPerMs := sampleRate / 1000
	totalSamples := samplesPerMs * durationMs * channels
	bytesPerSample := bitsPerSample / 8
	testAudioData := make([]byte, totalSamples*bytesPerSample)

	// 生成低幅度的正弦波而不是静音
	for i := 0; i < len(testAudioData); i += 2 {
		sampleIndex := i / 2
		frequency := 440.0 // A4音调
		amplitude := 1000  // 低幅度
		sample := int16(
			float64(
				amplitude,
			) * math.Sin(
				2*math.Pi*frequency*float64(sampleIndex)/float64(sampleRate),
			),
		)

		// 小端序写入
		testAudioData[i] = byte(sample & 0xFF)
		if i+1 < len(testAudioData) {
			testAudioData[i+1] = byte((sample >> 8) & 0xFF)
		}
	}

	return testAudioData, nil
}

// createWithRetry 带重试的创建实例
func (hc *HealthChecker) createWithRetry(
	ctx context.Context,
	factory ResourceFactory,
) (interface{}, error) {
	var lastErr error

	for attempt := 0; attempt < hc.connConfig.RetryAttempts; attempt++ {
		if attempt > 0 {
			hc.logger.Info("连接重试 %d/%d", attempt+1, hc.connConfig.RetryAttempts)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(hc.connConfig.RetryDelay):
			}
		}

		// 设置超时上下文
		_, cancel := context.WithTimeout(ctx, hc.connConfig.Timeout)

		// 创建实例
		instance, err := factory.Create()
		cancel()

		if err != nil {
			lastErr = err
			hc.logger.Warn("连接尝试 %d/%d 失败: %v", attempt+1, hc.connConfig.RetryAttempts, err)
			continue
		}

		return instance, nil
	}

	return nil, fmt.Errorf("重试 %d 次后仍然失败: %v", hc.connConfig.RetryAttempts, lastErr)
}

// GetResults 获取所有检查结果
func (hc *HealthChecker) GetResults() map[string]*CheckResult {
	return hc.results
}

// PrintReport 打印检查报告
func (hc *HealthChecker) PrintReport() {
	hc.logger.Info("=== 连通性检查报告 ===")

	for providerType, result := range hc.results {
		status := "✓ 通过"
		if !result.Success {
			status = "✗ 失败"
		}

		checkTypeName := "基础"
		if result.CheckMode == FunctionalCheck {
			checkTypeName = "功能性"
		}

		hc.logger.Info("%s (%s): %s (耗时: %v)", providerType, checkTypeName, status, result.Duration)

		if result.Error != nil {
			hc.logger.Error("  错误: %v", result.Error)
		}

		if len(result.Details) > 0 {
			for key, value := range result.Details {
				hc.logger.Info("  %s: %v", key, value)
			}
		}
	}

	hc.logger.Info("=== 检查报告结束 ===")
}
