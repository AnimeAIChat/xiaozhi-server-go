package infrastructure

import (
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// FunctionRegistry 实现 FunctionRegistryInterface 的具体类
type FunctionRegistry struct {
	functions map[string]openai.Tool
}

// NewFunctionRegistry 创建新的函数注册表
func NewFunctionRegistry() *FunctionRegistry {
	return &FunctionRegistry{
		functions: make(map[string]openai.Tool),
	}
}

// RegisterFunction 注册函数
func (fr *FunctionRegistry) RegisterFunction(name string, function interface{}) error {
	tool, ok := function.(openai.Tool)
	if !ok {
		return fmt.Errorf("function must be of type openai.Tool")
	}
	if _, exists := fr.functions[name]; exists {
		// 记录日志但不返回错误，保持兼容性
		// 这里可以添加日志记录
	}
	fr.functions[name] = tool
	return nil
}

// GetFunction 获取指定名称的函数
func (fr *FunctionRegistry) GetFunction(name string) (interface{}, error) {
	if function, exists := fr.functions[name]; exists {
		return function, nil
	}
	return nil, fmt.Errorf("function not found: %s", name)
}

// GetAllFunctions 获取所有函数
func (fr *FunctionRegistry) GetAllFunctions() []interface{} {
	functions := make([]interface{}, 0, len(fr.functions))
	for _, function := range fr.functions {
		functions = append(functions, function)
	}
	return functions
}

// GetFunctionByFilter 根据过滤器获取函数
func (fr *FunctionRegistry) GetFunctionByFilter(filter []string) []interface{} {
	if len(filter) == 0 {
		return fr.GetAllFunctions()
	}
	functions := make([]interface{}, 0)
	for name, function := range fr.functions {
		// 返回self和local开头的函数
		if strings.HasPrefix(name, "self") || strings.HasPrefix(name, "local") {
			functions = append(functions, function)
			continue
		}
		for _, f := range filter {
			if name == f {
				functions = append(functions, function)
				break
			}
		}
	}
	return functions
}

// UnregisterAllFunctions 注销所有函数
func (fr *FunctionRegistry) UnregisterAllFunctions() error {
	// Unregister all functions
	for name := range fr.functions {
		delete(fr.functions, name)
	}
	return nil
}

// UnregisterFunction 注销指定函数
func (fr *FunctionRegistry) UnregisterFunction(name string) error {
	// Unregister a specific function
	if _, exists := fr.functions[name]; exists {
		delete(fr.functions, name)
	} else {
		return fmt.Errorf("function not found: %s", name)
	}
	return nil
}

// FunctionExists 检查函数是否存在
func (fr *FunctionRegistry) FunctionExists(name string) bool {
	_, exists := fr.functions[name]
	return exists
}