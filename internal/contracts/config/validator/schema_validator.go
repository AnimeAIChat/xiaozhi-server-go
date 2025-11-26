package validator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"xiaozhi-server-go/internal/contracts/config"
)

// SchemaValidator 基于Schema的配置验证器
type SchemaValidator struct {
	schema    map[string]interface{}
	rules     map[string]config.ValidationRule
	regexCache map[string]*regexp.Regexp
}

// ValidationRuleType 验证规则类型
type ValidationRuleType string

const (
	// RuleTypeRequired 必填验证
	RuleTypeRequired ValidationRuleType = "required"
	// RuleTypeMinLength 最小长度验证
	RuleTypeMinLength ValidationRuleType = "min_length"
	// RuleTypeMaxLength 最大长度验证
	RuleTypeMaxLength ValidationRuleType = "max_length"
	// RuleTypeMinValue 最小值验证
	RuleTypeMinValue ValidationRuleType = "min_value"
	// RuleTypeMaxValue 最大值验证
	RuleTypeMaxValue ValidationRuleType = "max_value"
	// RuleTypePattern 正则表达式验证
	RuleTypePattern ValidationRuleType = "pattern"
	// RuleTypeEnum 枚举值验证
	RuleTypeEnum ValidationRuleType = "enum"
	// RuleTypeEmail 邮箱验证
	RuleTypeEmail ValidationRuleType = "email"
	// RuleTypeURL URL验证
	RuleTypeURL ValidationRuleType = "url"
	// RuleTypeDuration 时间间隔验证
	RuleTypeDuration ValidationRuleType = "duration"
	// RuleTypeIPAddress IP地址验证
	RuleTypeIPAddress ValidationRuleType = "ip_address"
	// RuleTypePort 端口号验证
	RuleTypePort ValidationRuleType = "port"
)

// SchemaDefinition Schema定义
type SchemaDefinition struct {
	Type        string                 `json:"type"`        // string, number, boolean, object, array
	Required    bool                   `json:"required"`    // 是否必填
	Default     interface{}            `json:"default"`     // 默认值
	MinLength   int                    `json:"min_length"`  // 字符串最小长度
	MaxLength   int                    `json:"max_length"`  // 字符串最大长度
	MinValue    interface{}            `json:"min_value"`   // 数值最小值
	MaxValue    interface{}            `json:"max_value"`   // 数值最大值
	Pattern     string                 `json:"pattern"`     // 正则表达式
	Enum        []interface{}          `json:"enum"`        // 枚举值
	Description string                 `json:"description"` // 描述
	Properties  map[string]interface{} `json:"properties"`  // 对象属性
	Items       map[string]interface{} `json:"items"`       // 数组项定义
}

// NewSchemaValidator 创建Schema验证器
func NewSchemaValidator() *SchemaValidator {
	validator := &SchemaValidator{
		schema:    make(map[string]interface{}),
		rules:     make(map[string]config.ValidationRule),
		regexCache: make(map[string]*regexp.Regexp),
	}

	// 设置默认Schema
	validator.setDefaultSchema()

	return validator
}

// ValidateSchema 验证配置结构是否符合Schema
func (sv *SchemaValidator) ValidateSchema(config map[string]interface{}) error {
	for key, schemaDef := range sv.schema {
		schemaMap, ok := schemaDef.(map[string]interface{})
		if !ok {
			continue
		}

		// 解析Schema定义
		def := sv.parseSchemaDefinition(key, schemaMap)
		if def == nil {
			continue
		}

		// 验证配置值
		if err := sv.validateValueBySchema(key, config[key], def); err != nil {
			return err
		}
	}

	// 应用自定义规则
	for key, value := range config {
		if rule, exists := sv.rules[key]; exists {
			if err := rule.Validator(value); err != nil {
				return fmt.Errorf("custom validation failed for key '%s': %w", key, err)
			}
		}
	}

	return nil
}

// ValidateValue 验证具体配置值
func (sv *SchemaValidator) ValidateValue(key string, value interface{}) error {
	// 首先检查Schema定义
	if schemaDef, exists := sv.schema[key]; exists {
		schemaMap, ok := schemaDef.(map[string]interface{})
		if ok {
			def := sv.parseSchemaDefinition(key, schemaMap)
			if def != nil {
				if err := sv.validateValueBySchema(key, value, def); err != nil {
					return err
				}
			}
		}
	}

	// 然后检查自定义规则
	if rule, exists := sv.rules[key]; exists {
		if rule.Validator(value) != nil {
			return fmt.Errorf("custom validation failed for key '%s'", key)
		}
	}

	return nil
}

// GetSchema 获取配置Schema
func (sv *SchemaValidator) GetSchema() map[string]interface{} {
	// 返回Schema的深拷贝
	result := make(map[string]interface{})
	for k, v := range sv.schema {
		result[k] = v
	}
	return result
}

// AddCustomRule 添加自定义验证规则
func (sv *SchemaValidator) AddCustomRule(key string, rule config.ValidationRule) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	if rule.Validator == nil {
		return fmt.Errorf("validator function cannot be nil")
	}

	sv.rules[key] = rule
	return nil
}

// RemoveCustomRule 移除自定义验证规则
func (sv *SchemaValidator) RemoveCustomRule(key string) error {
	delete(sv.rules, key)
	return nil
}

// AddSchemaField 添加Schema字段定义
func (sv *SchemaValidator) AddSchemaField(key string, definition SchemaDefinition) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	// 转换为map格式
	schemaMap := sv.schemaDefinitionToMap(definition)
	sv.schema[key] = schemaMap

	return nil
}

// 私有方法

// setDefaultSchema 设置默认Schema
func (sv *SchemaValidator) setDefaultSchema() {
	// 服务器配置
	sv.schema["server.ip"] = map[string]interface{}{
		"type":        "string",
		"pattern":     `^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$|^localhost$|^[\w\.-]+$`,
		"description": "服务器IP地址或域名",
	}

	sv.schema["server.port"] = map[string]interface{}{
		"type":     "number",
		"min_value": 1,
		"max_value": 65535,
		"default":   8080,
	}

	sv.schema["server.token"] = map[string]interface{}{
		"type":      "string",
		"min_length": 8,
		"max_length": 256,
		"required":   true,
	}

	// 日志配置
	sv.schema["log.level"] = map[string]interface{}{
		"type":    "string",
		"enum":    []interface{}{"debug", "info", "warn", "error", "fatal"},
		"default": "info",
	}

	sv.schema["log.dir"] = map[string]interface{}{
		"type":    "string",
		"default": "./logs",
	}

	// 数据库配置
	sv.schema["database.driver"] = map[string]interface{}{
		"type":    "string",
		"enum":    []interface{}{"sqlite", "mysql", "postgres"},
		"default": "sqlite",
	}

	sv.schema["database.dsn"] = map[string]interface{}{
		"type":     "string",
		"min_length": 1,
		"required":  true,
	}

	// LLM配置
	sv.schema["llm.openai.api_key"] = map[string]interface{}{
		"type":     "string",
		"min_length": 10,
		"required":  true,
	}

	sv.schema["llm.openai.model"] = map[string]interface{}{
		"type":    "string",
		"enum":    []interface{}{"gpt-3.5-turbo", "gpt-4", "gpt-4-turbo"},
		"default": "gpt-3.5-turbo",
	}

	sv.schema["llm.openai.temperature"] = map[string]interface{}{
		"type":     "number",
		"min_value": 0.0,
		"max_value": 2.0,
		"default":  0.7,
	}

	sv.schema["llm.openai.max_tokens"] = map[string]interface{}{
		"type":     "number",
		"min_value": 1,
		"max_value": 8192,
		"default":  500,
	}

	// ASR配置
	sv.schema["asr.doubao.app_id"] = map[string]interface{}{
		"type":     "string",
		"min_length": 1,
		"required":  true,
	}

	sv.schema["asr.doubao.access_token"] = map[string]interface{}{
		"type":     "string",
		"min_length": 1,
		"required":  true,
	}

	// TTS配置
	sv.schema["tts.edge.voice"] = map[string]interface{}{
		"type":    "string",
		"enum":    []interface{}{"zh-CN-XiaoxiaoNeural", "zh-CN-YunyangNeural", "zh-CN-XiaoyiNeural"},
		"default": "zh-CN-XiaoxiaoNeural",
	}

	sv.schema["tts.edge.speed"] = map[string]interface{}{
		"type":     "number",
		"min_value": 0.25,
		"max_value": 3.0,
		"default":  1.0,
	}
}

// parseSchemaDefinition 解析Schema定义
func (sv *SchemaValidator) parseSchemaDefinition(key string, schemaMap map[string]interface{}) *SchemaDefinition {
	def := &SchemaDefinition{}

	if typ, ok := schemaMap["type"].(string); ok {
		def.Type = typ
	}

	if required, ok := schemaMap["required"].(bool); ok {
		def.Required = required
	}

	if defaultValue, ok := schemaMap["default"]; ok {
		def.Default = defaultValue
	}

	if minLength, ok := schemaMap["min_length"].(float64); ok {
		def.MinLength = int(minLength)
	}

	if maxLength, ok := schemaMap["max_length"].(float64); ok {
		def.MaxLength = int(maxLength)
	}

	if minValue, ok := schemaMap["min_value"]; ok {
		def.MinValue = minValue
	}

	if maxValue, ok := schemaMap["max_value"]; ok {
		def.MaxValue = maxValue
	}

	if pattern, ok := schemaMap["pattern"].(string); ok {
		def.Pattern = pattern
	}

	if enum, ok := schemaMap["enum"].([]interface{}); ok {
		def.Enum = enum
	}

	if description, ok := schemaMap["description"].(string); ok {
		def.Description = description
	}

	if properties, ok := schemaMap["properties"].(map[string]interface{}); ok {
		def.Properties = properties
	}

	if items, ok := schemaMap["items"].(map[string]interface{}); ok {
		def.Items = items
	}

	return def
}

// validateValueBySchema 根据Schema验证值
func (sv *SchemaValidator) validateValueBySchema(key string, value interface{}, def *SchemaDefinition) error {
	// 检查必填项
	if def.Required && value == nil {
		if def.Default != nil {
			return nil // 使用默认值
		}
		return fmt.Errorf("required field '%s' is missing", key)
	}

	// 如果值为空且不是必填，使用默认值
	if value == nil {
		if def.Default != nil {
			return nil
		}
		return nil // 可选字段为空时跳过验证
	}

	// 类型验证
	if err := sv.validateType(key, value, def.Type); err != nil {
		return err
	}

	// 长度验证（字符串类型）
	if def.Type == "string" {
		if str, ok := value.(string); ok {
			if def.MinLength > 0 && len(str) < def.MinLength {
				return fmt.Errorf("field '%s' must be at least %d characters", key, def.MinLength)
			}
			if def.MaxLength > 0 && len(str) > def.MaxLength {
				return fmt.Errorf("field '%s' must be at most %d characters", key, def.MaxLength)
			}
		}
	}

	// 数值范围验证
	if def.Type == "number" {
		if err := sv.validateNumericRange(key, value, def.MinValue, def.MaxValue); err != nil {
			return err
		}
	}

	// 枚举值验证
	if len(def.Enum) > 0 {
		if err := sv.validateEnum(key, value, def.Enum); err != nil {
			return err
		}
	}

	// 正则表达式验证
	if def.Pattern != "" {
		if err := sv.validatePattern(key, value, def.Pattern); err != nil {
			return err
		}
	}

	// 特殊类型验证
	if strings.Contains(def.Description, "email") {
		if err := sv.validateEmail(key, value); err != nil {
			return err
		}
	}

	if strings.Contains(def.Description, "url") {
		if err := sv.validateURL(key, value); err != nil {
			return err
		}
	}

	if strings.Contains(def.Description, "port") {
		if err := sv.validatePort(key, value); err != nil {
			return err
		}
	}

	if strings.Contains(def.Description, "duration") {
		if err := sv.validateDuration(key, value); err != nil {
			return err
		}
	}

	return nil
}

// validateType 验证类型
func (sv *SchemaValidator) validateType(key string, value interface{}, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field '%s' must be a string, got %T", key, value)
		}
	case "number":
		if _, ok := value.(float64); ok {
			return nil
		}
		if _, ok := value.(int); ok {
			return nil
		}
		if _, ok := value.(int64); ok {
			return nil
		}
		return fmt.Errorf("field '%s' must be a number, got %T", key, value)
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field '%s' must be a boolean, got %T", key, value)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("field '%s' must be an object, got %T", key, value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("field '%s' must be an array, got %T", key, value)
		}
	default:
		return fmt.Errorf("unknown type '%s' for field '%s'", expectedType, key)
	}
	return nil
}

// validateNumericRange 验证数值范围
func (sv *SchemaValidator) validateNumericRange(key string, value interface{}, minVal, maxVal interface{}) error {
	var num float64
	var ok bool

	switch v := value.(type) {
	case float64:
		num, ok = v, true
	case int:
		num, ok = float64(v), true
	case int64:
		num, ok = float64(v), true
	case float32:
		num, ok = float64(v), true
	default:
		return fmt.Errorf("field '%s' must be numeric", key)
	}

	if !ok {
		return fmt.Errorf("field '%s' must be numeric", key)
	}

	if minVal != nil {
		if min, ok := minVal.(float64); ok {
			if num < min {
				return fmt.Errorf("field '%s' must be at least %v", key, minVal)
			}
		}
	}

	if maxVal != nil {
		if max, ok := maxVal.(float64); ok {
			if num > max {
				return fmt.Errorf("field '%s' must be at most %v", key, maxVal)
			}
		}
	}

	return nil
}

// validateEnum 验证枚举值
func (sv *SchemaValidator) validateEnum(key string, value interface{}, enumValues []interface{}) error {
	for _, enumValue := range enumValues {
		if value == enumValue {
			return nil
		}
	}
	return fmt.Errorf("field '%s' must be one of %v", key, enumValues)
}

// validatePattern 验证正则表达式
func (sv *SchemaValidator) validatePattern(key string, value interface{}, pattern string) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("field '%s' must be a string for pattern validation", key)
	}

	regex, err := sv.getCompiledRegex(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern for field '%s': %w", key, err)
	}

	if !regex.MatchString(str) {
		return fmt.Errorf("field '%s' does not match pattern '%s'", key, pattern)
	}

	return nil
}

// validateEmail 验证邮箱
func (sv *SchemaValidator) validateEmail(key string, value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("field '%s' must be a string", key)
	}

	emailRegex := sv.getEmailRegex()
	if !emailRegex.MatchString(str) {
		return fmt.Errorf("field '%s' must be a valid email address", key)
	}

	return nil
}

// validateURL 验证URL
func (sv *SchemaValidator) validateURL(key string, value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("field '%s' must be a string", key)
	}

	if !strings.HasPrefix(str, "http://") && !strings.HasPrefix(str, "https://") {
		return fmt.Errorf("field '%s' must be a valid URL starting with http:// or https://", key)
	}

	return nil
}

// validatePort 验证端口号
func (sv *SchemaValidator) validatePort(key string, value interface{}) error {
	var port int
	var ok bool

	switch v := value.(type) {
	case int:
		port, ok = v, true
	case float64:
		port, ok = int(v), true
	case string:
		if p, err := strconv.Atoi(v); err == nil {
			port, ok = p, true
		}
	default:
		return fmt.Errorf("field '%s' must be a valid port number", key)
	}

	if !ok {
		return fmt.Errorf("field '%s' must be a valid port number", key)
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("field '%s' must be between 1 and 65535", key)
	}

	return nil
}

// validateDuration 验证时间间隔
func (sv *SchemaValidator) validateDuration(key string, value interface{}) error {
	switch v := value.(type) {
	case string:
		if _, err := time.ParseDuration(v); err != nil {
			return fmt.Errorf("field '%s' must be a valid duration string", key)
		}
	case time.Duration:
		// 直接接受time.Duration类型
	case int:
		if v <= 0 {
			return fmt.Errorf("field '%s' must be a positive duration", key)
		}
	case int64:
		if v <= 0 {
			return fmt.Errorf("field '%s' must be a positive duration", key)
		}
	case float64:
		if v <= 0 {
			return fmt.Errorf("field '%s' must be a positive duration", key)
		}
	default:
		return fmt.Errorf("field '%s' must be a valid duration", key)
	}

	return nil
}

// getCompiledRegex 获取编译的正则表达式（带缓存）
func (sv *SchemaValidator) getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if regex, exists := sv.regexCache[pattern]; exists {
		return regex, nil
	}

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	sv.regexCache[pattern] = regex
	return regex, nil
}

// getEmailRegex 获取邮箱正则表达式
func (sv *SchemaValidator) getEmailRegex() *regexp.Regexp {
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	regex, _ := sv.getCompiledRegex(pattern)
	return regex
}

// schemaDefinitionToMap 将SchemaDefinition转换为map
func (sv *SchemaValidator) schemaDefinitionToMap(def SchemaDefinition) map[string]interface{} {
	result := make(map[string]interface{})

	if def.Type != "" {
		result["type"] = def.Type
	}

	if def.Required {
		result["required"] = def.Required
	}

	if def.Default != nil {
		result["default"] = def.Default
	}

	if def.MinLength > 0 {
		result["min_length"] = def.MinLength
	}

	if def.MaxLength > 0 {
		result["max_length"] = def.MaxLength
	}

	if def.MinValue != nil {
		result["min_value"] = def.MinValue
	}

	if def.MaxValue != nil {
		result["max_value"] = def.MaxValue
	}

	if def.Pattern != "" {
		result["pattern"] = def.Pattern
	}

	if len(def.Enum) > 0 {
		result["enum"] = def.Enum
	}

	if def.Description != "" {
		result["description"] = def.Description
	}

	if len(def.Properties) > 0 {
		result["properties"] = def.Properties
	}

	if len(def.Items) > 0 {
		result["items"] = def.Items
	}

	return result
}