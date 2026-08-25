package webapi

import (
	"context"
	"strings"
	"testing"
)

func TestMissingProviderTestFields(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		subType string
		config  map[string]interface{}
		want    []string
	}{
		{
			name:    "OpenAI requires endpoint key and model",
			kind:    "LLM",
			subType: "openai",
			config:  map[string]interface{}{"url": "https://example.test/v1", "api_key": "key"},
			want:    []string{"model_name"},
		},
		{
			name:    "Ollama does not require a key",
			kind:    "LLM",
			subType: "ollama",
			config:  map[string]interface{}{"url": "http://127.0.0.1:11434", "model_name": "qwen3"},
			want:    []string{},
		},
		{
			name:    "TTS requires its configured voice",
			kind:    "TTS",
			subType: "edge",
			config:  map[string]interface{}{},
			want:    []string{"voice"},
		},
		{
			name:    "unknown subtype is not silently accepted",
			kind:    "ASR",
			subType: "custom",
			config:  map[string]interface{}{},
			want:    []string{"当前类型暂不支持测试"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := missingProviderTestFields(tt.kind, tt.subType, tt.config)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("missingProviderTestFields() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCollectLiveResponse(t *testing.T) {
	t.Run("collects streamed response", func(t *testing.T) {
		response := make(chan string, 2)
		response <- "连接"
		response <- "成功"
		close(response)

		got, err := collectLiveResponse(context.Background(), response)
		if err != nil || got != "连接成功" {
			t.Fatalf("collectLiveResponse() = %q, %v", got, err)
		}
	})

	t.Run("returns provider error", func(t *testing.T) {
		response := make(chan string, 1)
		response <- "【OpenAI服务响应异常: unauthorized】"
		close(response)

		_, err := collectLiveResponse(context.Background(), response)
		if err == nil || !strings.Contains(err.Error(), "unauthorized") {
			t.Fatalf("expected provider error, got %v", err)
		}
	})
}

func TestLoadProviderTestPCM(t *testing.T) {
	pcm, err := loadProviderTestPCM()
	if err != nil {
		t.Fatalf("loadProviderTestPCM() error = %v", err)
	}
	if len(pcm) < 24000 {
		t.Fatalf("test audio is unexpectedly short: %d bytes", len(pcm))
	}
}

func TestDecodeVLLLMTestConfigConvertsNumericStrings(t *testing.T) {
	cfg, err := decodeVLLLMTestConfig(map[string]interface{}{
		"type":        "openai",
		"model_name":  "vision-model",
		"url":         "https://example.test/v1",
		"api_key":     "test-key",
		"max_tokens":  "1024",
		"temperature": "0.2",
		"top_p":       "0.9",
	})
	if err != nil {
		t.Fatalf("decodeVLLLMTestConfig() error = %v", err)
	}
	if cfg.MaxTokens != 1024 || cfg.Temperature != 0.2 || cfg.TopP != 0.9 {
		t.Fatalf("numeric values were not converted: %#v", cfg)
	}
}
