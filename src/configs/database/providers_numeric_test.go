package database

import (
	"encoding/json"
	"testing"

	"xiaozhi-server-go/src/configs"

	"gorm.io/datatypes"
)

func TestNormalizeProviderNumericDataSupportsLegacyLLMValues(t *testing.T) {
	raw := datatypes.JSON([]byte(`{"type":"openai","max_tokens":"1024","temperature":"0.2","top_p":"0.9"}`))
	normalized := normalizeProviderNumericData("LLM", raw)
	var config configs.LLMConfig
	if err := json.Unmarshal([]byte(normalized), &config); err != nil {
		t.Fatalf("normalized configuration cannot be decoded: %v", err)
	}
	if config.MaxTokens != 1024 || config.Temperature != 0.2 || config.TopP != 0.9 {
		t.Fatalf("legacy numeric values were not normalized: %#v", config)
	}
}

func TestNormalizeProviderInputStoresNumericValues(t *testing.T) {
	normalized := normalizeProviderInput("VLLLM", map[string]interface{}{"max_tokens": "512", "temperature": "0.1"}).(map[string]interface{})
	if _, ok := normalized["max_tokens"].(int); !ok {
		t.Fatalf("max_tokens type = %T, want int", normalized["max_tokens"])
	}
	if _, ok := normalized["temperature"].(float64); !ok {
		t.Fatalf("temperature type = %T, want float64", normalized["temperature"])
	}
}
