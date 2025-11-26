package chat

// MemoryInterface 定义对话记忆管理接口
type MemoryInterface interface {
	// QueryMemory 查询相关记忆
	QueryMemory(query string) (string, error)

	// SaveMemory 保存对话记忆
	SaveMemory(dialogue []Message) error

	// ClearMemory 清空记忆
	ClearMemory() error
}
