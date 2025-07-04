package ota

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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

// @Summary OTA 预检请求
// @Description 处理 OTA 接口的 OPTIONS 预检请求，返回 200
// @Tags OTA
// @Accept */*
// @Produce plain
// @Success 200 {string} string "OK"
// @Router /ota/ [options]
func handleOtaOptions(c *gin.Context) {
	c.Status(http.StatusOK)
}

// @Summary 获取 OTA 状态
// @Description 获取 OTA 服务状态和 WebSocket 地址，供设备查询
// @Tags OTA
// @Produce plain
// @Success 200 {string} string "OTA interface is running, websocket address: ws://..."
// @Router /ota/ [get]
func handleOtaGet(c *gin.Context, updateURL string) {
	c.String(http.StatusOK, "OTA interface is running, websocket address: "+updateURL)
}

// 请求体结构体定义
type OtaRequest struct {
	Application struct {
		Version string `json:"version" example:"1.0.0"`
	} `json:"application"`
}

// @Summary 上传设备信息获取最新固件
// @Description 设备上传信息后，返回最新固件版本和下载地址
// @Tags OTA
// @Accept json
// @Produce json
// @Param device-id header string true "设备ID"
// @Param body body OtaRequest true "请求体"
// @Success 200 {object} OtaFirmwareResponse
// @Failure 400 {object} ErrorResponse
// @Router /ota/ [post]
func handleOtaPost(c *gin.Context, updateURL string) {
	deviceID := c.GetHeader("device-id")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Success: false, Message: "缺少 device-id"})
		return
	}
	var body OtaRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Success: false, Message: "解析失败: " + err.Error()})
		return
	}

	version := body.Application.Version
	if version == "" {
		version = "1.0.0"
	}

	otaDir := filepath.Join(".", "ota_bin")
	_ = os.MkdirAll(otaDir, 0755)
	bins, _ := filepath.Glob(filepath.Join(otaDir, "*.bin"))
	firmwareURL := ""
	if len(bins) > 0 {
		sort.Slice(bins, func(i, j int) bool {
			return versionLess(bins[j], bins[i])
		})
		latest := filepath.Base(bins[0])
		version = strings.TrimSuffix(latest, ".bin")
		firmwareURL = "/ota_bin/" + latest
	}

	resp := OtaFirmwareResponse{}
	resp.ServerTime.Timestamp = time.Now().UnixNano() / 1e6
	resp.ServerTime.TimezoneOffset = 8 * 60
	resp.Firmware.Version = version
	resp.Firmware.URL = firmwareURL
	resp.Websocket.URL = updateURL

	c.JSON(http.StatusOK, resp)
}

// @Summary 下载 OTA 固件文件
// @Description 根据文件名下载 OTA 固件
// @Tags OTA
// @Produce application/octet-stream
// @Param filename path string true "固件文件名"
// @Success 200 "文件流"
// @Failure 404 {object} ErrorResponse
// @Router /ota_bin/{filename} [get]
func handleOtaBinDownload(c *gin.Context) {
	fname := c.Param("filename")
	p := filepath.Join("ota_bin", fname)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{Success: false, Message: "file not found"})
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename="+fname)
	c.File(p)
}

// versionLess 比较版本号语义 a < b
func versionLess(a, b string) bool {
	aV := strings.Split(strings.TrimSuffix(filepath.Base(a), ".bin"), ".")
	bV := strings.Split(strings.TrimSuffix(filepath.Base(b), ".bin"), ".")
	for i := 0; i < len(aV) && i < len(bV); i++ {
		if aV[i] != bV[i] {
			return aV[i] < bV[i]
		}
	}
	return len(aV) < len(bV)
}
