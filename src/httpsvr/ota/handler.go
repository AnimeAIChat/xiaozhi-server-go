package ota

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/httpsvr/webapi"
	"xiaozhi-server-go/src/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OtaFirmwareResponse 定义OTA固件接口返回结构
type OtaFirmwareResponse struct {
	ServerTime struct {
		Timestamp      int64 `json:"timestamp" example:"1688443200000"`
		TimezoneOffset int   `json:"timezone_offset" example:"480"`
	} `json:"server_time"`
	Firmware struct {
		Version string `json:"version" example:"1.0.3"`
		URL     string `json:"url" example:"/ota_bin/1.0.3.bin"`
	} `json:"firmware"`
	Websocket struct {
		URL string `json:"url" example:"wss://example.com/ota"`
	} `json:"websocket"`
}

// ErrorResponse 定义错误返回结构
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"缺少 device-id"`
}

// HandleOTARequest 处理 OTA 请求（POST /ota/）
//
// @Summary 设备 OTA 请求
// @Description 设备通过该接口发起 OTA 升级请求，服务端返回最新固件信息及 MQTT/WebSocket 连接信息。
// @Tags OTA
// @Accept json
// @Produce json
// @Param device-id header string true "设备唯一ID，如 MAC 地址（用于唯一标识设备）"
// @Param client-id header string false "客户端ID，用于标识应用或用户"
// @Param body body ota.OTARequestBody false "应用版本信息"
// @Success 200 {object} ota.OTAResponse
// @Failure 400 {object} object "缺少必要参数或请求体错误"
// @Router /ota/ [post]
func (s *DefaultOTAService) HandleOTARequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodOptions:
			c.Status(http.StatusOK)
			return
		case http.MethodGet:
			fmt.Println("OTA interface accessed, websocket address:", s.UpdateURL)
			c.String(http.StatusOK, "OTA interface is running, websocket address: "+s.UpdateURL)
			return
		case http.MethodPost:
			s.handlePostOTA(c)
			return
		default:
			c.String(http.StatusMethodNotAllowed, "不支持的方法: %s", c.Request.Method)
		}
	}
}

// Trans2OTARequestBody 将原始请求体转换为 OTARequestBody 结构体
// 主要用于兼容处理不同格式的请求体
func (s *DefaultOTAService) Trans2OTARequestBody(raw map[string]interface{}) OTARequestBody {
	var req OTARequestBody
	// 优先尝试直接转换
	if b, err := json.Marshal(raw); err == nil {
		if err := json.Unmarshal(b, &req); err == nil {
			return req
		} else {
			utils.DefaultLogger.Warn("转换 OTARequestBody 失败: %v", err)
		}
	} else {
		utils.DefaultLogger.Error("OTARequestBody 请求体 JSON 编码失败: %v", err)
	}

	utils.DefaultLogger.Warn("直接转换 OTARequestBody 失败，尝试逐字段兼容转换")
	// 失败则逐字段兼容转换
	// Application
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
	// Board
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
	// ChipInfo
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

// @Summary 上传设备信息获取最新固件
// @Description 设备上传信息后，返回最新固件版本和下载地址
// @Tags OTA
// @Accept json
// @Produce json
// @Param device-id header string true "设备ID"
// @Param body body ota.OTARequestBody true "请求体"
// @Success 200 {object} OtaFirmwareResponse
// @Failure 400 {object} ErrorResponse
// @Router /ota/ [post]
func (s *DefaultOTAService) handlePostOTA(c *gin.Context) {
	deviceID := c.GetHeader("device-id")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Success: false, Message: "缺少 device-id"})
		return
	}

	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "解析失败: " + err.Error()})
		utils.DefaultLogger.Error("解析 OTA 请求体失败: %v", err)
		return
	}
	// 兼容转换到 OTARequestBody
	var req OTARequestBody = s.Trans2OTARequestBody(raw)

	clientID := c.GetHeader("client-id")
	client_id := "CGID_test@@@" + strings.Replace(deviceID, ":", "_", -1) + "@@@" + clientID
	version := req.Application.Version
	if version == "" {
		version = "1.0.0"
	}

	otaDir := configs.Cfg.Paths.OTADir
	if otaDir == "" {
		otaDir = "data/ota" // 后备默认值
	}
	_ = os.MkdirAll(otaDir, 0755)
	bins, _ := filepath.Glob(filepath.Join(otaDir, "*.bin"))
	firmwareURL := ""
	if len(bins) > 0 {
		sort.Slice(bins, func(i, j int) bool {
			return versionLess(bins[j], bins[i])
		})
		latest := filepath.Base(bins[0])
		version = strings.TrimSuffix(latest, ".bin")
		firmwareURL = "/ota/download/" + latest
	}
	cfg := configs.Cfg
	updateURL := cfg.Web.Websocket
	deviceName := req.Board.Name
	s.CheckAndUpdateDevice(c, cfg, req, deviceID, client_id, deviceName, version)
	resp := OtaFirmwareResponse{}
	resp.ServerTime.Timestamp = time.Now().UnixNano() / 1e6
	resp.ServerTime.TimezoneOffset = 8 * 60
	resp.Firmware.Version = version
	resp.Firmware.URL = firmwareURL
	resp.Websocket.URL = updateURL
	if resp.Websocket.URL == "" {
		utils.DefaultLogger.Warn("===========================================================")
		utils.DefaultLogger.Warn("=====  WebSocket URL 未配置，OTA 服务可能无法正常工作 =====")
		utils.DefaultLogger.Warn("=====  请尽快修改配置并重启服务                       =====")
		utils.DefaultLogger.Warn("===========================================================")
	}

	c.JSON(http.StatusOK, resp)
}
func (s *DefaultOTAService) CheckAndUpdateDevice(
	c *gin.Context,
	cfg *configs.Config,
	req OTARequestBody,
	deviceID, clientID, deviceName, version string,
) *models.Device {
	var resultDevice *models.Device
	webapi.WithTx(c, func(tx *gorm.DB) error {
		// 查询 设备是否已注册
		device, err := database.FindDeviceByID(tx, deviceID) // 确保设备存在
		if err != nil {
			if device == nil {
				// 检查是否是被软删除的设备
				deleteDevice, err := database.FindDeletedDeviceByID(tx, deviceID)
				if err != nil && err != gorm.ErrRecordNotFound {
					utils.DefaultLogger.Error("查询已删除设备失败: %v", err)
				}
				if deleteDevice != nil {
					// 硬删除
					if err := database.HardDeleteDevice(tx, deleteDevice.DeviceID); err != nil {
						utils.DefaultLogger.Error("硬删除设备失败: %s, %v", deviceID, err)
						c.JSON(
							http.StatusInternalServerError,
							gin.H{"success": false, "message": "设备状态异常，请联系管理员" + err.Error()},
						)
						return err
					}
				}
			}

			device = &models.Device{
				DeviceID:         deviceID,   // 设置设备ID
				ClientID:         clientID,   // 设置客户端ID
				Name:             deviceName, // 设置设备名称
				Version:          version,    // 设置设备版本
				RegisterTimeV2:   time.Now(),
				LastActiveTimeV2: time.Now(),
				BoardType:        req.Board.Type,    // 设置主板类型
				ChipModelName:    req.ChipModelName, // 设置芯片型号
				Channel:          req.Board.Channel, // 设置WiFi频道
				SSID:             req.Board.SSID,    // 设置WiFi SSID
				Language:         req.Language,      // 设置语言
				OTA:              true,              // 设置支持OTA升级
				AgentID:          nil,               // 初始AgentID为nil
			}
			appBytes, _ := json.Marshal(req.Application)
			device.Application = string(appBytes)
			if err := database.AddDevice(tx, device); err != nil { // 保存设备信息
				utils.DefaultLogger.Error("保存设备信息失败: %v", err)
				c.JSON(
					http.StatusInternalServerError,
					gin.H{"success": false, "message": "保存设备信息失败: " + err.Error()},
				)
				return err
			} else {
				utils.DefaultLogger.Info("新设备注册成功: %s", deviceID)
			}
		}

		appBytes, _ := json.Marshal(req.Application)
		if device.Application != string(appBytes) {
			device.Application = string(appBytes) // 更新应用信息
			if err := database.UpdateDevice(tx, device); err != nil {
				utils.DefaultLogger.Error("更新设备应用信息失败: %v", err)
			}
		}
		resultDevice = device
		return nil
	})
	return resultDevice
}

// HandleFirmwareDownload 处理 /ota_bin/:filename 下载
func (s *DefaultOTAService) HandleFirmwareDownload() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 支持之前 /ota_bin/:filename 路径，也兼容通配 /ota_bin/*filepath
		// 尝试先读取通配参数，否则读取老参数名
		reqPath := c.Param("filepath")

		if reqPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid file path"})
			return
		}

		clean := path.Clean(reqPath)
		clean = strings.TrimPrefix(clean, "/")
		if strings.Contains(clean, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid file path"})
			return
		}

		otaDir := configs.Cfg.Paths.OTADir
		if otaDir == "" {
			otaDir = "data/ota" // 后备默认值
		}
		p := filepath.Join(otaDir, filepath.FromSlash(clean))
		//fmt.Println("Firmware download requested:", clean, "full path:", p)

		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "file not found"})
			return
		}

		c.Header("Content-Type", "application/octet-stream")
		// 使用 gin 的 File 来处理范围请求与高效传输
		c.File(p)
	}
}

// 按语义比较两个版本号 a < b
func versionLess(a, b string) bool {
	av := strings.Split(strings.TrimSuffix(filepath.Base(a), ".bin"), ".")
	bv := strings.Split(strings.TrimSuffix(filepath.Base(b), ".bin"), ".")

	maxLen := len(av)
	if len(bv) > maxLen {
		maxLen = len(bv)
	}

	for i := 0; i < maxLen; i++ {
		var ai, bi int
		if i < len(av) {
			ai, _ = strconv.Atoi(av[i])
		}
		if i < len(bv) {
			bi, _ = strconv.Atoi(bv[i])
		}
		if ai != bi {
			return ai < bi
		}
	}
	return false
}
