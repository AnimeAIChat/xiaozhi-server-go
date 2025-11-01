package vision

import (
	"xiaozhi-server-go/internal/domain/image"
)

// VisionRequest Vision分析请求结构
type VisionRequest struct {
	Question  string               // 问题文本
	ImageData image.ImageData      // 图片数据（使用新架构的ImageData）
	DeviceID  string               // 设备ID
	ClientID  string               // 客户端ID
	ImagePath string               // 图片保存路径
}

// VisionAnalysisData 表示视觉分析结果在 data 字段中的结构
type VisionAnalysisData struct {
	Result string `json:"result,omitempty"` // 分析结果（成功时）
	Error  string `json:"error,omitempty"`  // 错误信息（失败时）
}

// VisionStatusResponse Vision状态响应结构
type VisionStatusResponse struct {
	Message string // 状态信息（纯文本）
}

// AuthVerifyResult 认证验证结果
type AuthVerifyResult struct {
	IsValid  bool
	DeviceID string
}