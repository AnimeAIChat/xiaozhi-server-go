package vision

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	domainauth "xiaozhi-server-go/internal/domain/auth"
	domainimage "xiaozhi-server-go/internal/domain/image"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/platform/errors"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
)

const (
	// MaxFileSize 最大文件大小为5MB
	MaxFileSize = 5 * 1024 * 1024
)

// Service Vision服务的HTTP传输层实现
type Service struct {
	logger       *utils.Logger
	config       *config.Config
	imagePipeline *domainimage.Pipeline
	vlllmMap     map[string]*vlllm.Provider
	authToken    *domainauth.AuthToken
}

// NewService 创建新的Vision服务实例
func NewService(
	config *config.Config,
	logger *utils.Logger,
	imagePipeline *domainimage.Pipeline,
) (*Service, error) {
	if config == nil {
		return nil, errors.Wrap(errors.KindConfig, "vision.new", "config is required", nil)
	}
	if logger == nil {
		return nil, errors.Wrap(errors.KindConfig, "vision.new", "logger is required", nil)
	}
	if imagePipeline == nil {
		return nil, errors.Wrap(errors.KindConfig, "vision.new", "image pipeline is required", nil)
	}

	service := &Service{
		logger:         logger,
		config:         config,
		imagePipeline:  imagePipeline,
		vlllmMap:       make(map[string]*vlllm.Provider),
	}

	// 初始化认证工具
	service.authToken = domainauth.NewAuthToken(config.Server.Token)

	// 初始化VLLLM providers
	if err := service.initVLLMProviders(); err != nil {
		return nil, errors.Wrap(errors.KindConfig, "vision.new", "failed to init VLLLM providers", err)
	}

	return service, nil
}

// Register 注册Vision相关的HTTP路由
func (s *Service) Register(ctx context.Context, router *gin.RouterGroup) error {
	// Vision 主接口（GET用于状态检查，POST用于图片分析）
	router.GET("/vision", s.handleGet)
	router.POST("/vision", s.handlePost)
	router.OPTIONS("/vision", s.handleOptions)

	s.logger.InfoTag("HTTP", "Vision服务路由注册完成")
	return nil
}

// initVLLMProviders 初始化VLLLM providers
func (s *Service) initVLLMProviders() error {
	selectedVLLLM := s.config.Selected.VLLLM
	if selectedVLLLM == "" {
		s.logger.WarnTag("VLLLM", "请先设置 VLLLM provider 配置")
		return errors.Wrap(errors.KindConfig, "init_vlllm", "VLLLM provider not configured", nil)
	}

	vlllmConfig := s.config.VLLLM[selectedVLLLM]

	// 创建VLLLM provider配置
	providerConfig := &vlllm.Config{
		Type:        vlllmConfig.Type,
		ModelName:   vlllmConfig.ModelName,
		BaseURL:     vlllmConfig.BaseURL,
		APIKey:      vlllmConfig.APIKey,
		Temperature: vlllmConfig.Temperature,
		MaxTokens:   vlllmConfig.MaxTokens,
		TopP:        vlllmConfig.TopP,
		Security:    vlllmConfig.Security,
	}

	// 创建provider实例
	provider, err := vlllm.NewProvider(providerConfig, s.logger)
	if err != nil {
		s.logger.WarnTag("VLLLM", "创建 provider 失败: %v", err)
		return errors.Wrap(errors.KindDomain, "init_vlllm", "failed to create VLLLM provider", err)
	}

	// 初始化provider
	if err := provider.Initialize(); err != nil {
		s.logger.WarnTag("VLLLM", "初始化 provider 失败: %v", err)
		return errors.Wrap(errors.KindDomain, "init_vlllm", "failed to initialize VLLLM provider", err)
	}

	s.vlllmMap[selectedVLLLM] = provider
	// s.logger.InfoTag("VLLLM", "初始化完成: %s", selectedVLLLM)

	if len(s.vlllmMap) == 0 {
		s.logger.ErrorTag("VLLLM", "没有可用的 provider，请检查配置")
		return errors.Wrap(errors.KindDomain, "init_vlllm", "no available VLLLM providers", nil)
	}

	return nil
}

// handleOptions 处理OPTIONS请求（CORS）
func (s *Service) handleOptions(c *gin.Context) {
	s.logger.InfoTag("Vision", "收到 CORS 预检请求 (OPTIONS)")
	s.addCORSHeaders(c)
	c.Status(http.StatusOK)
}

// handleGet 处理GET请求（状态检查）
// @Summary 检查Vision服务状态
// @Description 获取Vision服务的运行状态和可用模型信息
// @Tags Vision
// @Produce plain
// @Success 200 {string} string "服务状态信息"
// @Router /vision [get]
func (s *Service) handleGet(c *gin.Context) {
	s.logger.Info("收到Vision状态检查请求 get")
	s.addCORSHeaders(c)

	// 检查Vision服务状态
	var message string
	if len(s.vlllmMap) > 0 {
		message = fmt.Sprintf("MCP Vision 接口运行正常，共有 %d 个可用的视觉分析模型", len(s.vlllmMap))
	} else {
		message = "MCP Vision 接口运行不正常，没有可用的VLLLM provider"
	}

	c.String(http.StatusOK, message)
}

// handlePost 处理POST请求（图片分析）
// @Summary 图片视觉分析
// @Description 上传图片并进行视觉分析，支持问题询问
// @Tags Vision
// @Accept multipart/form-data
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param Device-Id header string true "设备ID"
// @Param Client-Id header string true "客户端ID"
// @Param file formData file true "图片文件"
// @Param question formData string true "分析问题"
// @Success 200 {object} VisionAnalysisData
// @Failure 400 {object} object
// @Failure 401 {object} object
// @Failure 500 {object} object
// @Router /vision [post]
func (s *Service) handlePost(c *gin.Context) {
	s.addCORSHeaders(c)

	deviceID := c.GetHeader("Device-Id")

	// 验证认证
	authResult, err := s.verifyAuth(c)
	if err != nil {
		s.respondError(c, http.StatusUnauthorized, err.Error())
		s.logger.Warn("vision 认证失败: %v", err)
		return
	}

	if !authResult.IsValid {
		s.respondError(c, http.StatusUnauthorized, "无效的认证token或设备ID不匹配")
		s.logger.Warn("Vision认证失败: %s", authResult.DeviceID)
		return
	}

	// 解析multipart表单
	req, err := s.parseMultipartRequest(c, deviceID)
	if err != nil {
		s.respondError(c, http.StatusBadRequest, err.Error())
		s.logger.Warn("Vision请求解析失败: %v", err)
		return
	}

	s.logger.Debug("收到Vision分析请求: %+v", map[string]interface{}{
		"device_id":  req.DeviceID,
		"client_id":  req.ClientID,
		"question":   req.Question,
		"image_size": len(req.ImageData.Data),
		"image_path": req.ImagePath,
	})

	// 处理图片分析
	result, err := s.processVisionRequest(req)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, err.Error())
		s.logger.Warn("Vision请求处理失败: %v", err)
		return
	}

	payload := VisionAnalysisData{
		Result: result,
	}
	s.logger.Info("Vision分析结果: %s", result)
	s.respondSuccess(c, http.StatusOK, payload, "Vision 分析成功")
}

// verifyAuth 验证认证token
func (s *Service) verifyAuth(c *gin.Context) (*AuthVerifyResult, error) {
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, errors.Wrap(errors.KindTransport, "verify_auth", "invalid auth header format", nil)
	}

	token := authHeader[7:] // 移除"Bearer "前缀

	s.logger.Debug("收到认证token: %s", token)

	// 验证token
	isValid, deviceID, err := s.authToken.VerifyToken(token)
	if err != nil || !isValid {
		s.logger.Warn("认证token验证失败: %v", err)
		return nil, errors.Wrap(errors.KindTransport, "verify_auth", "token verification failed", err)
	}

	// 检查设备ID匹配
	requestDeviceID := c.GetHeader("Device-Id")
	if requestDeviceID != deviceID {
		s.logger.Warn(
			"设备ID与token不匹配: 请求设备ID=%s, token设备ID=%s",
			requestDeviceID,
			deviceID,
		)
		return nil, errors.Wrap(errors.KindTransport, "verify_auth", "device ID mismatch", nil)
	}

	return &AuthVerifyResult{
		IsValid:  true,
		DeviceID: deviceID,
	}, nil
}

// parseMultipartRequest 解析multipart表单请求
func (s *Service) parseMultipartRequest(
	c *gin.Context,
	deviceID string,
) (*VisionRequest, error) {
	// 解析multipart表单
	err := c.Request.ParseMultipartForm(MaxFileSize)
	if err != nil {
		return nil, errors.Wrap(errors.KindTransport, "parse_request", "failed to parse multipart form", err)
	}

	// 获取question字段
	question := c.Request.FormValue("question")
	if question == "" {
		return nil, errors.Wrap(errors.KindTransport, "parse_request", "question field is required", nil)
	}

	// 获取图片文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return nil, errors.Wrap(errors.KindTransport, "parse_request", "file field is required", err)
	}
	defer file.Close()

	// 检查文件大小
	if header.Size > MaxFileSize {
		return nil, errors.Wrap(errors.KindTransport, "parse_request", "file size exceeds limit", fmt.Errorf("max size: %dMB", MaxFileSize/1024/1024))
	}

	// 使用新的image pipeline处理图片
	input := domainimage.Input{
		Reader:         file,
		DeclaredFormat: s.detectImageFormatFromFilename(header.Filename),
		Source:         "upload",
	}

	output, err := s.imagePipeline.Process(c.Request.Context(), input)
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "parse_request", "image processing failed", err)
	}

	// 将图片保存在本地
	saveImageToFile, err := s.saveImageToFile(output.Bytes, deviceID, output.Format)
	if err != nil {
		return nil, errors.Wrap(errors.KindStorage, "parse_request", "failed to save image file", err)
	}

	return &VisionRequest{
		Question:  question,
		ImageData: domainimage.ImageData{
			Data:   output.Base64,
			Format: output.Format,
		},
		DeviceID:  deviceID,
		ClientID:  c.GetHeader("Client-Id"),
		ImagePath: saveImageToFile,
	}, nil
}

// saveImageToFile 将图片保存到本地文件
func (s *Service) saveImageToFile(imageData []byte, deviceID, format string) (string, error) {
	// 生成唯一的文件名
	deviceIDFormat := strings.ReplaceAll(deviceID, ":", "_")
	filename := fmt.Sprintf(
		"%s_%d.%s",
		deviceIDFormat,
		time.Now().Unix(),
		format,
	)
	filepath := fmt.Sprintf("data/uploads/%s", filename)

	// 确保uploads目录存在
	if err := os.MkdirAll("data/uploads", os.ModePerm); err != nil {
		return "", errors.Wrap(errors.KindStorage, "save_image", "failed to create uploads directory", err)
	}

	// 保存图片文件
	if err := os.WriteFile(filepath, imageData, 0o644); err != nil {
		return "", errors.Wrap(errors.KindStorage, "save_image", "failed to write image file", err)
	}

	s.logger.Info("图片已保存到: %s", filepath)
	return filepath, nil
}

// processVisionRequest 处理视觉分析请求
func (s *Service) processVisionRequest(req *VisionRequest) (string, error) {
	// 选择VLLLM provider
	provider := s.selectProvider("")
	if provider == nil {
		return "", errors.Wrap(errors.KindDomain, "process_request", "no available vision model", nil)
	}

	// 调用VLLLM provider
	messages := []providers.Message{} // 空的历史消息
	responseChan, err := provider.ResponseWithImage(
		context.Background(),
		"",
		messages,
		req.ImageData,
		req.Question,
	)
	if err != nil {
		return "", errors.Wrap(errors.KindDomain, "process_request", "VLLLM call failed", err)
	}

	// 收集所有响应内容
	var result strings.Builder
	for content := range responseChan {
		result.WriteString(content)
	}
	s.logger.InfoTag("VLLLM", "分析结果: %s", result.String())

	return result.String(), nil
}

// selectProvider 选择VLLLM provider
func (s *Service) selectProvider(modelName string) *vlllm.Provider {
	// 如果指定了模型名，尝试找到对应的provider
	if modelName != "" {
		if provider, exists := s.vlllmMap[modelName]; exists {
			return provider
		}
	}

	// 否则返回第一个可用的provider
	for _, provider := range s.vlllmMap {
		return provider
	}

	return nil
}

// detectImageFormatFromFilename 从文件名检测图片格式
func (s *Service) detectImageFormatFromFilename(filename string) string {
	if strings.HasSuffix(strings.ToLower(filename), ".jpg") || strings.HasSuffix(strings.ToLower(filename), ".jpeg") {
		return "jpeg"
	}
	if strings.HasSuffix(strings.ToLower(filename), ".png") {
		return "png"
	}
	if strings.HasSuffix(strings.ToLower(filename), ".gif") {
		return "gif"
	}
	if strings.HasSuffix(strings.ToLower(filename), ".bmp") {
		return "bmp"
	}
	if strings.HasSuffix(strings.ToLower(filename), ".webp") {
		return "webp"
	}
	return "jpeg" // 默认格式
}

// addCORSHeaders 添加CORS头
func (s *Service) addCORSHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Headers", "client-id, content-type, device-id, authorization")
	c.Header("Access-Control-Allow-Credentials", "true")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

// respondSuccess 返回成功响应
func (s *Service) respondSuccess(c *gin.Context, statusCode int, data interface{}, message string) {
	c.JSON(statusCode, gin.H{
		"success": true,
		"data":    data,
		"message": message,
		"code":    statusCode,
	})
}

// respondError 返回错误响应
func (s *Service) respondError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"success": false,
		"data":    gin.H{"error": message},
		"message": message,
		"code":    statusCode,
	})
}

// Cleanup 清理资源
func (s *Service) Cleanup() error {
	for name, provider := range s.vlllmMap {
		if err := provider.Cleanup(); err != nil {
			s.logger.WarnTag("VLLLM", "清理 provider %s 失败: %v", name, err)
		}
	}
	s.logger.InfoTag("Vision", "服务清理完成")
	return nil
}