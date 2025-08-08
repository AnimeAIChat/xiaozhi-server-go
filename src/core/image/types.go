package image

// ImageData 图片数据结构
type ImageData struct {
	URL    string `json:"url,omitempty"`    // 图片URL地址
	Data   string `json:"data,omitempty"`   // base64编码的图片数据
	Format string `json:"format,omitempty"` // 图片格式：jpeg, png, webp, gif
}

// ValidationResult 图片验证结果
type ValidationResult struct {
	IsValid      bool   // 是否有效
	Format       string // 实际格式
	Width        int    // 图片宽度
	Height       int    // 图片高度
	FileSize     int64  // 文件大小
	Error        error  // 错误信息
	SecurityRisk string // 安全风险描述
}

// EnhancedMessage 增强的消息结构，支持图片
type EnhancedMessage struct {
	Type      string    `json:"type"`                 // 消息类型：text, image, mixed
	Text      string    `json:"text,omitempty"`       // 文本内容
	ImageData ImageData `json:"image_data,omitempty"` // 图片数据
}

// ImageMetrics 图片处理统计信息
type ImageMetrics struct {
	TotalProcessed    int64 // 总处理数量
	URLDownloads      int64 // URL下载次数
	Base64Direct      int64 // Base64直接处理次数
	FailedValidations int64 // 验证失败次数
	SecurityIncidents int64 // 安全事件次数
}
