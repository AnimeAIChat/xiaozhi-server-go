package ota

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type DefaultOTAService struct {
	UpdateURL string
}

// NewDefaultOTAService 构造函数
func NewDefaultOTAService(updateURL string) *DefaultOTAService {
	return &DefaultOTAService{UpdateURL: updateURL}
}

// Start 实现 OTAService 接口，注册所有 OTA 相关路由
func (s *DefaultOTAService) Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) error {
	// OTA 主接口（支持 OPTIONS/GET/POST）
	apiGroup.Any("/ota/", func(c *gin.Context) {
		c.Header("Access-Control-Allow-Headers", "client-id, content-type, device-id")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Origin", "*")

		switch c.Request.Method {
		case http.MethodOptions:
			c.Status(http.StatusOK)
		case http.MethodGet:
			c.String(http.StatusOK, "OTA interface is running, websocket address: "+s.UpdateURL)
		case http.MethodPost:
			deviceID := c.GetHeader("device-id")
			if deviceID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "缺少 device-id"})
				return
			}
			var body map[string]interface{}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "解析失败: " + err.Error()})
				return
			}

			// 从请求体取版本号
			version := "1.0.0"
			if app, ok := body["application"].(map[string]interface{}); ok {
				if v, ok := app["version"].(string); ok {
					version = v
				}
			}

			// 扫描 ota_bin 目录，选最新固件
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

			c.JSON(http.StatusOK, gin.H{
				"server_time": gin.H{
					"timestamp":       time.Now().UnixNano() / 1e6,
					"timezone_offset": 8 * 60,
				},
				"firmware": gin.H{
					"version": version,
					"url":     firmwareURL,
				},
				"websocket": gin.H{
					"url": s.UpdateURL,
				},
			})
		default:
			c.String(http.StatusMethodNotAllowed, "不支持的方法: %s", c.Request.Method)
		}
	})

	// OTA 固件下载
	engine.GET("/ota_bin/:filename", func(c *gin.Context) {
		fname := c.Param("filename")
		p := filepath.Join("ota_bin", fname)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "file not found"})
			return
		}
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Content-Disposition", "attachment; filename="+fname)
		c.File(p)
	})

	return nil
}

// 按语义比较两个版本号 a < b
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
