package ota

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"xiaozhi-server-go/internal/domain/device/service"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/platform/errors"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/models"

	"github.com/gin-gonic/gin"
)

// Service OTA服务的HTTP传输层实现
type Service struct {
	updateURL     string
	config        *config.Config
	deviceService *service.DeviceService
	logger        *utils.Logger
}

// NewService 创建新的OTA服务实例
func NewService(
	updateURL string,
	config *config.Config,
	deviceService *service.DeviceService,
	logger *utils.Logger,
) (*Service, error) {
	if config == nil {
		return nil, errors.Wrap(errors.KindConfig, "ota.new", "config is required", nil)
	}
	if deviceService == nil {
		return nil, errors.Wrap(errors.KindConfig, "ota.new", "device service is required", nil)
	}
	if logger == nil {
		return nil, errors.Wrap(errors.KindConfig, "ota.new", "logger is required", nil)
	}

	service := &Service{
		updateURL:     updateURL,
		config:        config,
		deviceService: deviceService,
		logger:        logger,
	}

	return service, nil
}

// Register 注册OTA相关的HTTP路由
func (s *Service) Register(ctx context.Context, router *gin.RouterGroup) error {
	// OTA 主接口（支持GET和POST）
	router.Any("/ota/", s.handleOTARequest)

	// 固件下载接口
	router.GET("/ota_bin/*filepath", s.handleFirmwareDownload)

	s.logger.InfoTag("HTTP", "OTA服务路由注册完成")
	return nil
}

// handleOTARequest 处理OTA请求
func (s *Service) handleOTARequest(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodOptions:
		s.addCORSHeaders(c)
		c.Status(http.StatusOK)
		return
	case http.MethodGet:
		s.logger.Info("OTA interface accessed, websocket address: %s", s.updateURL)
		s.addCORSHeaders(c)
		c.String(http.StatusOK, "OTA interface is running, websocket address: "+s.updateURL)
		return
	case http.MethodPost:
		s.handlePostOTA(c)
		return
	default:
		s.addCORSHeaders(c)
		c.String(http.StatusMethodNotAllowed, "不支持的方法: %s", c.Request.Method)
	}
}

// handlePostOTA 处理POST OTA请求
// @Summary OTA设备注册和固件更新
// @Description 处理设备OTA请求，包括设备注册、激活码生成和固件信息返回
// @Tags OTA
// @Accept json
// @Produce json
// @Param device-id header string true "设备ID"
// @Param client-id header string true "客户端ID"
// @Param body body map[string]interface{} true "设备信息"
// @Success 200 {object} OTAResponse
// @Failure 400 {object} object
// @Failure 500 {object} object
// @Router /ota/ [post]
func (s *Service) handlePostOTA(c *gin.Context) {
	s.addCORSHeaders(c)

	deviceID := c.GetHeader("device-id")
	if deviceID == "" {
		s.respondError(c, http.StatusBadRequest, "缺少 device-id")
		return
	}

	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		s.respondError(c, http.StatusBadRequest, "解析失败: "+err.Error())
		s.logger.Error("解析 OTA 请求体失败: %v", err)
		return
	}

	// 兼容转换到 OTARequestBody
	var req OTARequestBody = s.trans2OTARequestBody(raw)

	clientID := c.GetHeader("client-id")
	clientIDFormatted := "CGID_test@@@" + strings.Replace(deviceID, ":", "_", -1) + "@@@" + clientID
	version := req.Application.Version
	if version == "" {
		version = "1.0.0"
	}

	// 获取最新固件信息
	firmwareInfo := s.getLatestFirmwareInfo(version)

	// 检查并更新设备信息
	device := s.checkAndUpdateDevice(c, req, deviceID, clientIDFormatted, req.Board.Name, version)

	// 构建响应
	resp := OTAResponse{
		ServerTime: ServerTimeInfo{
			Timestamp:      time.Now().UnixNano() / 1e6,
			TimezoneOffset: 8 * 60, // 东八区
		},
		Firmware: FirmwareInfo{
			Version: firmwareInfo.Version,
			URL:     firmwareInfo.URL,
		},
		WebSocket: WebSocketInfo{
			URL: s.updateURL,
		},
	}

	// 检查WebSocket URL配置
	if resp.WebSocket.URL == "" {
		s.logger.Warn("===========================================================")
		s.logger.Warn("=====  WebSocket URL 未配置，OTA 服务可能无法正常工作 =====")
		s.logger.Warn("=====  请尽快修改配置并重启服务                       =====")
		s.logger.Warn("===========================================================")
	}

	// 如果设备未激活，添加激活信息
	if device != nil && !s.isDeviceActivated(device) {
		resp.Activation = &Activation{
			Code:    s.generateActivationCode(deviceID),
			Message: fmt.Sprintf("Anime AI Chat %s", s.generateActivationCode(deviceID)),
		}
	}

	c.JSON(http.StatusOK, resp)
}

// trans2OTARequestBody 将原始请求体转换为 OTARequestBody 结构体
func (s *Service) trans2OTARequestBody(raw map[string]interface{}) OTARequestBody {
	var req OTARequestBody

	// 尝试直接转换
	if b, err := json.Marshal(raw); err == nil {
		if err := json.Unmarshal(b, &req); err == nil {
			return req
		} else {
			s.logger.Warn("转换 OTARequestBody 失败: %v", err)
		}
	} else {
		s.logger.Error("OTARequestBody 请求体 JSON 编码失败: %v", err)
	}

	s.logger.Warn("直接转换 OTARequestBody 失败，尝试逐字段兼容转换")

	// 逐字段兼容转换
	if app, ok := raw["application"].(map[string]interface{}); ok {
		if v, ok := app["name"].(string); ok {
			req.Application.Name = v
		}
		if v, ok := app["version"].(string); ok {
			req.Application.Version = v
		}
		if v, ok := app["compile_time"].(string); ok {
			req.Application.CompileTime = v
		}
		if v, ok := app["elf_sha256"].(string); ok {
			req.Application.ElfSHA256 = v
		}
		if v, ok := app["idf_version"].(string); ok {
			req.Application.IDFVersion = v
		}
	}

	if board, ok := raw["board"].(map[string]interface{}); ok {
		if v, ok := board["channel"].(float64); ok {
			req.Board.Channel = int(v)
		}
		if v, ok := board["ip"].(string); ok {
			req.Board.IP = v
		}
		if v, ok := board["mac"].(string); ok {
			req.Board.MAC = v
		}
		if v, ok := board["name"].(string); ok {
			req.Board.Name = v
		}
		if v, ok := board["rssi"].(float64); ok {
			req.Board.RSSI = int(v)
		}
		if v, ok := board["ssid"].(string); ok {
			req.Board.SSID = v
		}
		if v, ok := board["type"].(string); ok {
			req.Board.Type = v
		}
	}

	if chip, ok := raw["chip_info"].(map[string]interface{}); ok {
		if v, ok := chip["cores"].(float64); ok {
			req.ChipInfo.Cores = int(v)
		}
		if v, ok := chip["features"].(float64); ok {
			req.ChipInfo.Features = int(v)
		}
		if v, ok := chip["model"].(float64); ok {
			req.ChipInfo.Model = int(v)
		}
		if v, ok := chip["revision"].(float64); ok {
			req.ChipInfo.Revision = int(v)
		}
	}

	if v, ok := raw["chip_model_name"].(string); ok {
		req.ChipModelName = v
	}
	if v, ok := raw["flash_size"].(float64); ok {
		req.FlashSize = v
	}
	if v, ok := raw["language"].(string); ok {
		req.Language = v
	}
	if v, ok := raw["mac_address"].(string); ok {
		req.MacAddress = v
	}
	if v, ok := raw["minimum_free_heap_size"]; ok {
		switch vv := v.(type) {
		case string:
			req.MinimumFreeHeapSize = StringOrNumber(vv)
		case float64:
			req.MinimumFreeHeapSize = StringOrNumber(fmt.Sprintf("%v", vv))
		}
	}
	if ota, ok := raw["ota"].(map[string]interface{}); ok {
		if v, ok := ota["label"].(string); ok {
			req.OTA.Label = v
		}
	}
	if pt, ok := raw["partition_table"].([]interface{}); ok {
		for _, item := range pt {
			if m, ok := item.(map[string]interface{}); ok {
				var p struct {
					Address float64 `json:"address"`
					Label   string  `json:"label"`
					Size    float64 `json:"size"`
					Subtype int     `json:"subtype"`
					Type    int     `json:"type"`
				}
				if v, ok := m["address"].(float64); ok {
					p.Address = v
				}
				if v, ok := m["label"].(string); ok {
					p.Label = v
				}
				if v, ok := m["size"].(float64); ok {
					p.Size = v
				}
				if v, ok := m["subtype"].(float64); ok {
					p.Subtype = int(v)
				}
				if v, ok := m["type"].(float64); ok {
					p.Type = int(v)
				}
				req.PartitionTable = append(req.PartitionTable, p)
			}
		}
	}
	if v, ok := raw["uuid"].(string); ok {
		req.UUID = v
	}
	if v, ok := raw["version"].(float64); ok {
		req.Version = int(v)
	}

	return req
}

// getLatestFirmwareInfo 获取最新固件信息
func (s *Service) getLatestFirmwareInfo(currentVersion string) FirmwareInfo {
	otaDir := filepath.Join(".", "data", "ota_bin")
	_ = os.MkdirAll(otaDir, 0755)

	bins, _ := filepath.Glob(filepath.Join(otaDir, "*.bin"))
	if len(bins) == 0 {
		return FirmwareInfo{
			Version: currentVersion,
			URL:     "",
		}
	}

	// 按版本号排序
	sort.Slice(bins, func(i, j int) bool {
		return s.versionLess(bins[j], bins[i])
	})

	latest := filepath.Base(bins[0])
	version := strings.TrimSuffix(latest, ".bin")

	return FirmwareInfo{
		Version: version,
		URL:     "/ota_bin/" + latest,
	}
}

// checkAndUpdateDevice 检查并更新设备信息
func (s *Service) checkAndUpdateDevice(
	c *gin.Context,
	req OTARequestBody,
	deviceID, clientID, deviceName, version string,
) *models.Device {
	// 获取客户端IP地址
	ip := req.Board.IP
	if ip == "" {
		ip = c.ClientIP()
	}

	// 构建应用信息
	appInfo := ""
	if req.Application.Name != "" {
		appInfo = req.Application.Name
		if req.Application.Version != "" {
			appInfo += " " + req.Application.Version
		}
	}

	// 调用设备服务进行注册
	device, isNew, err := s.deviceService.RegisterDevice(
		c.Request.Context(),
		deviceID,
		clientID,
		deviceName,
		version,
		ip,
		appInfo,
	)

	if err != nil {
		// 注册失败时记录错误，但不中断OTA流程，返回mock设备
		s.logger.Error("设备注册失败: %v", err)
		return &models.Device{
			DeviceID:         deviceID,
			ClientID:         clientID,
			Name:             deviceName,
			Version:          version,
			RegisterTimeV2:   time.Now(),
			LastActiveTimeV2: time.Now(),
			BoardType:        req.Board.Type,
			ChipModelName:    req.ChipModelName,
			Channel:          req.Board.Channel,
			SSID:             req.Board.SSID,
			Language:         req.Language,
			OTA:              true,
		}
	}

	// 只有新注册的设备才记录成功日志
	if isNew {
		s.logger.Info("设备注册成功: deviceID=%s, clientID=%s, name=%s, status=%s",
			device.DeviceID, device.ClientID, device.Name, device.AuthStatus)
	}

	// 注册成功，返回包含注册信息的设备对象
	return &models.Device{
		DeviceID:         device.DeviceID,
		ClientID:         device.ClientID,
		Name:             device.Name,
		Version:          device.Version,
		RegisterTimeV2:   device.RegisterTime,
		LastActiveTimeV2: device.LastActiveTime,
		BoardType:        req.Board.Type,
		ChipModelName:    req.ChipModelName,
		Channel:          req.Board.Channel,
		SSID:             req.Board.SSID,
		Language:         req.Language,
		AuthStatus:       string(device.AuthStatus), // 设置认证状态
		OTA:              true,
	}
}

// handleFirmwareDownload 处理固件下载请求
func (s *Service) handleFirmwareDownload(c *gin.Context) {
	s.addCORSHeaders(c)

	// 支持通配路径
	reqPath := c.Param("filepath")
	if reqPath == "" {
		s.respondError(c, http.StatusBadRequest, "invalid file path")
		return
	}

	clean := path.Clean(reqPath)
	clean = strings.TrimPrefix(clean, "/")
	if strings.Contains(clean, "..") {
		s.respondError(c, http.StatusBadRequest, "invalid file path")
		return
	}

	p := filepath.Join("data", "ota_bin", filepath.FromSlash(clean))

	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		s.respondError(c, http.StatusNotFound, "file not found")
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	c.File(p)
}

// versionLess 按语义比较两个版本号 a < b
func (s *Service) versionLess(a, b string) bool {
	aV := strings.Split(strings.TrimSuffix(filepath.Base(a), ".bin"), ".")
	bV := strings.Split(strings.TrimSuffix(filepath.Base(b), ".bin"), ".")
	for i := 0; i < len(aV) && i < len(bV); i++ {
		if aV[i] != bV[i] {
			return aV[i] < bV[i]
		}
	}
	return len(aV) < len(bV)
}

// isDeviceActivated 检查设备是否已激活
func (s *Service) isDeviceActivated(device *models.Device) bool {
	// 根据 domain 层定义，approved 状态表示已激活
	return device.AuthStatus == "approved"
}

// generateActivationCode 生成激活码
func (s *Service) generateActivationCode(deviceID string) string {
	// 简化激活码生成逻辑，实际应该使用更安全的方法
	return fmt.Sprintf("%06d", len(deviceID)%1000000)
}

// addCORSHeaders 添加CORS头
func (s *Service) addCORSHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type, device-id, client-id")
}

// respondError 返回错误响应
func (s *Service) respondError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"success": false,
		"message": message,
	})
}