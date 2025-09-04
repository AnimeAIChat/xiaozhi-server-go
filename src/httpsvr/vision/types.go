package vision

// VisionRequest Vision分析请求结构（从multipart表单解析）
// @Description Vision分析请求体
// @Param question formData string true "问题文本"
// @Param image formData file true "图片文件"
// @Param Device-Id header string true "设备ID"
// @Param Client-Id header string false "客户端ID"
// @Param image_path formData string false "图片路径"
type VisionRequest struct {
	Question  string // 问题文本（从表单字段获取）
	Image     []byte // 图片数据（从文件字段获取）
	DeviceID  string // 设备ID（从请求头获取）
	ClientID  string // 客户端ID（从请求头获取）
	ImagePath string // 图片路径
}

// VisionResponse Vision标准响应结构（兼容Python版本）
// @Description Vision分析响应体
// @Success 200 {object} VisionResponse
// @Failure 400 {object} VisionResponse
// @Failure 401 {object} VisionResponse
// @Failure 500 {object} VisionResponse
type VisionResponse struct {
	Success bool   `json:"success"`           // 是否成功
	Result  string `json:"result,omitempty"`  // 分析结果（成功时）
	Message string `json:"message,omitempty"` // 错误信息（失败时）
}

// VisionStatusResponse Vision状态响应结构
// @Description Vision状态响应体
type VisionStatusResponse struct {
	Message string // 状态信息（纯文本）
}

// AuthVerifyResult 认证验证结果
type AuthVerifyResult struct {
	IsValid  bool
	DeviceID string
}
