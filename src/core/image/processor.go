package image

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/utils"

	"github.com/google/uuid"
)

// ImageProcessor 图片处理器
type ImageProcessor struct {
	config     *configs.VLLMConfig
	validator  *ImageSecurityValidator
	logger     *utils.Logger
	tempDir    string
	metrics    *ImageMetrics
	httpClient *http.Client
}

// NewImageProcessor 创建新的图片处理器
func NewImageProcessor(config *configs.VLLMConfig, logger *utils.Logger) (*ImageProcessor, error) {
	// 创建临时目录
	tempDir := filepath.Join("tmp", "images")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %v", err)
	}

	// 创建安全验证器
	validator := NewImageSecurityValidator(&config.Security, logger)

	// 配置HTTP客户端
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 限制重定向次数为3次
			if len(via) >= 3 {
				return fmt.Errorf("停止重定向：超过最大重定向次数")
			}
			return nil
		},
	}

	return &ImageProcessor{
		config:     config,
		validator:  validator,
		logger:     logger,
		tempDir:    tempDir,
		metrics:    &ImageMetrics{},
		httpClient: httpClient,
	}, nil
}

// ProcessImage 处理图片数据，返回base64编码的图片
func (p *ImageProcessor) ProcessImage(ctx context.Context, imageData ImageData) (string, error) {
	atomic.AddInt64(&p.metrics.TotalProcessed, 1)

	var finalImageData ImageData

	// 根据输入类型处理图片
	if imageData.URL != "" {
		// 处理URL类型图片
		atomic.AddInt64(&p.metrics.URLDownloads, 1)

		base64Data, err := p.processURLImage(ctx, imageData.URL, imageData.Format)
		if err != nil {
			atomic.AddInt64(&p.metrics.FailedValidations, 1)
			return "", fmt.Errorf("URL图片处理失败: %v", err)
		}

		finalImageData = ImageData{
			Data:   base64Data,
			Format: imageData.Format,
		}

		p.logger.Info("URL图片处理成功", map[string]interface{}{
			"url":    imageData.URL,
			"format": imageData.Format,
		})

	} else if imageData.Data != "" {
		// 直接处理base64数据
		atomic.AddInt64(&p.metrics.Base64Direct, 1)
		finalImageData = imageData

		p.logger.Debug("Base64图片处理开始 %v", map[string]interface{}{
			"format":      imageData.Format,
			"data_length": len(imageData.Data),
		})
	} else {
		return "", fmt.Errorf("图片数据为空：既没有URL也没有base64数据")
	}

	// 安全验证
	validationResult := p.validator.ValidateImageData(finalImageData)
	if !validationResult.IsValid {
		atomic.AddInt64(&p.metrics.FailedValidations, 1)
		if validationResult.SecurityRisk != "" {
			atomic.AddInt64(&p.metrics.SecurityIncidents, 1)
			p.logger.Warn("检测到安全威胁", map[string]interface{}{
				"error":         validationResult.Error.Error(),
				"security_risk": validationResult.SecurityRisk,
				"format":        finalImageData.Format,
			})
		}
		return "", fmt.Errorf("图片验证失败: %v", validationResult.Error)
	}

	p.logger.Debug("图片处理完成 %v", map[string]interface{}{
		"format":    validationResult.Format,
		"width":     validationResult.Width,
		"height":    validationResult.Height,
		"file_size": validationResult.FileSize,
	})

	return finalImageData.Data, nil
}

// processURLImage 处理URL图片
func (p *ImageProcessor) processURLImage(
	ctx context.Context,
	url string,
	format string,
) (string, error) {
	// 创建唯一的临时文件名
	tempFileName := fmt.Sprintf("img_%d_%s", time.Now().UnixNano(), uuid.New().String())
	if format != "" {
		tempFileName += "." + format
	}
	tempPath := filepath.Join(p.tempDir, tempFileName)

	// 确保在函数结束时删除临时文件
	defer func() {
		if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
			p.logger.Warn("删除临时文件失败", map[string]interface{}{
				"path":  tempPath,
				"error": err.Error(),
			})
		}
	}()

	// 下载图片
	if err := p.downloadImage(ctx, url, tempPath); err != nil {
		return "", fmt.Errorf("下载图片失败: %v", err)
	}

	// 读取文件并转换为base64
	imageData, err := os.ReadFile(tempPath)
	if err != nil {
		return "", fmt.Errorf("读取临时文件失败: %v", err)
	}

	// 转换为base64
	base64Data := base64.StdEncoding.EncodeToString(imageData)

	p.logger.Info("URL图片下载和转换完成", map[string]interface{}{
		"url":         url,
		"temp_path":   tempPath,
		"file_size":   len(imageData),
		"base64_size": len(base64Data),
	})

	return base64Data, nil
}

// downloadImage 下载图片到临时文件
func (p *ImageProcessor) downloadImage(ctx context.Context, url string, tempPath string) error {
	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置User-Agent，避免被某些网站拒绝
	req.Header.Set("User-Agent", "XiaoZhi-Image-Bot/1.0")

	// 发送请求
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP响应错误: %d %s", resp.StatusCode, resp.Status)
	}

	// 检查Content-Type
	contentType := resp.Header.Get("Content-Type")
	if !p.isValidImageContentType(contentType) {
		return fmt.Errorf("无效的Content-Type: %s", contentType)
	}

	// 检查Content-Length
	if resp.ContentLength > p.config.Security.MaxFileSize {
		return fmt.Errorf("文件过大: %d bytes，最大允许: %d bytes",
			resp.ContentLength, p.config.Security.MaxFileSize)
	}

	// 创建临时文件
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer tempFile.Close()

	// 使用LimitReader限制下载大小，防止无限下载
	limitedReader := io.LimitReader(resp.Body, p.config.Security.MaxFileSize)

	// 复制数据到临时文件
	written, err := io.Copy(tempFile, limitedReader)
	if err != nil {
		return fmt.Errorf("下载文件失败: %v", err)
	}

	p.logger.Info("图片下载完成", map[string]interface{}{
		"url":          url,
		"content_type": contentType,
		"size":         written,
		"temp_path":    tempPath,
	})

	return nil
}

// isValidImageContentType 检查Content-Type是否为有效的图片类型
func (p *ImageProcessor) isValidImageContentType(contentType string) bool {
	validContentTypes := []string{
		"image/jpeg",
		"image/jpg",
		"image/png",
		"image/gif",
		"image/webp",
		"image/bmp",
	}

	contentTypeLower := strings.ToLower(contentType)
	for _, validType := range validContentTypes {
		if strings.Contains(contentTypeLower, validType) {
			return true
		}
	}

	return false
}

// GetMetrics 获取处理统计信息
func (p *ImageProcessor) GetMetrics() ImageMetrics {
	return ImageMetrics{
		TotalProcessed:    atomic.LoadInt64(&p.metrics.TotalProcessed),
		URLDownloads:      atomic.LoadInt64(&p.metrics.URLDownloads),
		Base64Direct:      atomic.LoadInt64(&p.metrics.Base64Direct),
		FailedValidations: atomic.LoadInt64(&p.metrics.FailedValidations),
		SecurityIncidents: atomic.LoadInt64(&p.metrics.SecurityIncidents),
	}
}

// Cleanup 清理资源
func (p *ImageProcessor) Cleanup() error {
	// 清理临时目录中的旧文件
	entries, err := os.ReadDir(p.tempDir)
	if err != nil {
		return fmt.Errorf("读取临时目录失败: %v", err)
	}

	now := time.Now()
	cleanedCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(p.tempDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 删除超过1小时的临时文件
		if now.Sub(info.ModTime()) > time.Hour {
			if err := os.Remove(filePath); err != nil {
				p.logger.Warn("删除过期临时文件失败", map[string]interface{}{
					"path":  filePath,
					"error": err.Error(),
				})
			} else {
				cleanedCount++
			}
		}
	}

	if cleanedCount > 0 {
		p.logger.Info("清理临时文件完成", map[string]interface{}{
			"cleaned_count": cleanedCount,
		})
	}

	return nil
}
