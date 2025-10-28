package face

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"xiaozhi-server-go/src/configs"
	facecore "xiaozhi-server-go/src/core/face"
	"xiaozhi-server-go/src/core/utils"

	"github.com/alibabacloud-go/tea/tea"
	"github.com/gin-gonic/gin"
)

// DefaultFaceService 人脸识别服务默认实现
type DefaultFaceService struct {
	logger       *utils.Logger
	config       *configs.Config
	faceDatabase *facecore.FaceDatabase
}

// NewDefaultFaceService 构造函数
func NewDefaultFaceService(config *configs.Config, logger *utils.Logger) (*DefaultFaceService, error) {
	service := &DefaultFaceService{
		logger:       logger,
		config:       config,
		faceDatabase: facecore.NewFaceDatabase(),
	}

	return service, nil
}

// generateNextUserID 生成下一个用户ID（从1开始递增）
func (s *DefaultFaceService) generateNextUserID() string {
	// 获取所有人脸数据
	faces := s.faceDatabase.GetAllFaces()

	// 找到最大的数字ID
	maxID := 0
	for _, face := range faces {
		// 解析user_id，期望格式为 "user_1", "user_2" 等
		if len(face.UserID) > 5 && face.UserID[:5] == "user_" {
			if id, err := strconv.Atoi(face.UserID[5:]); err == nil && id > maxID {
				maxID = id
			}
		}
	}

	// 返回下一个ID
	nextID := maxID + 1
	return fmt.Sprintf("user_%d", nextID)
}

// Start 实现 FaceService 接口，注册所有人脸识别相关路由
func (s *DefaultFaceService) Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) error {
	// 人脸注册接口
	apiGroup.POST("/face/register", s.handleFaceRegister)
	apiGroup.OPTIONS("/face/register", s.handleOptions)

	// 人脸删除接口
	apiGroup.POST("/face/delete", s.handleFaceDelete)
	apiGroup.OPTIONS("/face/delete", s.handleOptions)

	// 人脸数据库同步接口（ESP32从服务端同步数据）
	apiGroup.POST("/face/sync", s.handleFaceSync)
	apiGroup.OPTIONS("/face/sync", s.handleOptions)

	// 人脸数据库查询接口（ESP32获取所有人脸数据）
	apiGroup.GET("/face/list", s.handleFaceList)
	apiGroup.OPTIONS("/face/list", s.handleOptions)

	// 测试接口
	apiGroup.POST("/test", s.handleTest)
	apiGroup.OPTIONS("/test", s.handleOptions)

	s.logger.Info("[Face] [服务] HTTP路由注册完成")
	return nil
}

// handleOptions 处理OPTIONS请求（CORS）
func (s *DefaultFaceService) handleOptions(c *gin.Context) {
	s.logger.Info("收到Face CORS预检请求 options")
	s.addCORSHeaders(c)
	c.Status(http.StatusOK)
}

// handleTest 处理测试请求
func (s *DefaultFaceService) handleTest(c *gin.Context) {
	s.logger.Info("收到Face测试请求")

	var request map[string]interface{}
	if err := c.ShouldBindJSON(&request); err != nil {
		s.logger.Error("解析测试请求失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid JSON format",
		})
		return
	}

	response := gin.H{
		"status":    "ok",
		"message":   "Face service is running",
		"type":      request["type"],
		"timestamp": time.Now().Format(time.RFC3339),
	}

	s.logger.Info("Face测试请求处理完成")
	c.JSON(http.StatusOK, response)
}

// handleFaceRegister 处理人脸注册请求
func (s *DefaultFaceService) handleFaceRegister(c *gin.Context) {
	s.logger.Info("收到人脸注册HTTP请求")

	var request FaceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		s.logger.Error("解析人脸注册请求失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "face_register_ack",
			"success": false,
			"message": "Invalid JSON format",
		})
		return
	}

	// 验证必需字段
	if request.UserName == "" {
		s.logger.Error("人脸注册请求缺少user_name字段")
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "face_register_ack",
			"success": false,
			"message": "Missing user_name field",
		})
		return
	}

	if request.Feature == "" {
		s.logger.Error("人脸注册请求缺少feature字段")
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "face_register_ack",
			"success": false,
			"message": "Missing feature field",
		})
		return
	}

	s.logger.Info(fmt.Sprintf("[人脸注册] 用户: %s, 特征长度: %d, 图像长度: %d", request.UserName, len(request.Feature), len(request.Base64Image)))

	// 直接使用客户端发送的用户名作为用户ID
	userID := request.UserName

	// 将人脸特征保存到数据库
	err := s.faceDatabase.AddFaceWithImage(userID, request.UserName, request.Feature, request.Base64Image)
	if err != nil {
		s.logger.Error(fmt.Sprintf("保存人脸特征失败: %v", err))
		response := FaceResponse{
			Type:     "face_register_ack",
			Success:  false,
			UserID:   userID,
			UserName: request.UserName,
			Message:  fmt.Sprintf("注册失败: %v", err),
		}
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	// 返回注册成功响应
	response := FaceResponse{
		Type:      "face_register_ack",
		Success:   true,
		UserID:    userID,
		UserName:  request.UserName,
		Message:   "注册成功",
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	s.logger.Info(fmt.Sprintf("[人脸注册] 用户 %s 注册成功，用户ID: %s", request.UserName, userID))
	c.JSON(http.StatusOK, response)
}

// handleFaceList 处理人脸列表查询请求
func (s *DefaultFaceService) handleFaceList(c *gin.Context) {
	s.logger.Info("收到人脸列表查询HTTP请求")

	// 获取所有人脸数据
	faces := s.faceDatabase.GetAllFaces()

	response := gin.H{
		"type":        "face_list",
		"success":     true,
		"faces":       faces,
		"total_count": len(faces),
		"timestamp":   time.Now().Format(time.RFC3339),
	}

	s.logger.Info(fmt.Sprintf("[人脸列表] 返回 %d 个人脸数据", len(faces)))
	c.JSON(http.StatusOK, response)
}

// handleFaceDelete 处理人脸删除请求
func (s *DefaultFaceService) handleFaceDelete(c *gin.Context) {
	s.logger.Info("收到人脸删除HTTP请求")

	var request struct {
		Type     string `json:"type" binding:"required"`
		UserID   string `json:"user_id,omitempty"`
		UserName string `json:"user_name,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		s.logger.Error("解析人脸删除请求失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "face_delete_ack",
			"success": false,
			"message": "Invalid JSON format",
		})
		return
	}

	if request.UserID == "" && request.UserName == "" {
		s.logger.Error("人脸删除请求缺少user_id或user_name字段")
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "face_delete_ack",
			"success": false,
			"message": "Missing user_id or user_name field",
		})
		return
	}

	// 执行人脸删除
	err := s.faceDatabase.DeleteFace(request.UserID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("人脸删除失败: %v", err))
		response := gin.H{
			"type":    "face_delete_ack",
			"success": false,
			"message": fmt.Sprintf("删除失败: %v", err),
		}
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	response := gin.H{
		"type":    "face_delete_ack",
		"success": true,
		"message": "删除成功",
	}

	s.logger.Info(fmt.Sprintf("[人脸删除] 删除成功: %s", request.UserID))
	c.JSON(http.StatusOK, response)
}

// handleFaceSync 处理人脸数据库同步请求
func (s *DefaultFaceService) handleFaceSync(c *gin.Context) {
	s.logger.Info("收到人脸数据库同步HTTP请求")

	var request struct {
		Type     string `json:"type" binding:"required"`
		DeviceID string `json:"device_id,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		s.logger.Error("解析人脸数据库同步请求失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid JSON format",
		})
		return
	}

	s.logger.Info(fmt.Sprintf("[人脸数据库] 设备 %s 请求同步人脸数据库", request.DeviceID))

	// 从人脸数据库获取活跃的人脸数据
	responseBytes, err := s.faceDatabase.ToActiveJSON()
	if err != nil {
		s.logger.Error(fmt.Sprintf("获取人脸数据库数据失败: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get face database data",
		})
		return
	}

	// 解析JSON响应
	var responseData interface{}
	if err := json.Unmarshal(responseBytes, &responseData); err != nil {
		s.logger.Error(fmt.Sprintf("解析人脸数据库JSON失败: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to parse face database JSON",
		})
		return
	}

	s.logger.Info("[人脸数据库] 同步请求处理完成", tea.Prettify(responseData))
	c.JSON(http.StatusOK, responseData)
}

// addCORSHeaders 添加CORS头
func (s *DefaultFaceService) addCORSHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
	c.Header("Access-Control-Max-Age", "86400")
}
