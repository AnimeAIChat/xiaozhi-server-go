package image

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"strings"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/utils"

	_ "image/gif"  // 注册GIF解码器
	_ "image/jpeg" // 注册JPEG解码器
	_ "image/png"  // 注册PNG解码器

	_ "golang.org/x/image/webp" // 注册WEBP解码器
)

// ImageSecurityValidator 图片安全验证器
type ImageSecurityValidator struct {
	config *configs.SecurityConfig
	logger *utils.Logger
}

// NewImageSecurityValidator 创建新的图片安全验证器
func NewImageSecurityValidator(
	config *configs.SecurityConfig,
	logger *utils.Logger,
) *ImageSecurityValidator {
	return &ImageSecurityValidator{
		config: config,
		logger: logger,
	}
}

// 图片格式魔数签名 - 修复JPEG格式验证
var imageSignatures = map[string][]byte{
	"jpeg": {0xFF, 0xD8}, // JPEG文件只需要前两个字节
	"jpg":  {0xFF, 0xD8}, // 与JPEG相同
	"png":  {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
	"gif":  {0x47, 0x49, 0x46, 0x38},
	"webp": {0x52, 0x49, 0x46, 0x46}, // RIFF，需要进一步检查WEBP标识
	"bmp":  {0x42, 0x4D},
}

// ValidateImageData 验证图片数据
func (v *ImageSecurityValidator) ValidateImageData(imageData ImageData) ValidationResult {
	result := ValidationResult{IsValid: false}

	var imageBytes []byte
	var err error

	// 根据数据类型获取图片字节
	if imageData.Data != "" {
		// Base64数据
		imageBytes, err = base64.StdEncoding.DecodeString(imageData.Data)
		if err != nil {
			result.Error = fmt.Errorf("base64解码失败: %v", err)
			result.SecurityRisk = "无效的base64数据"
			return result
		}
	} else {
		result.Error = fmt.Errorf("缺少图片数据")
		return result
	}

	// 执行深度验证
	return v.deepValidateImage(imageBytes, imageData.Format)
}

// deepValidateImage 深度验证图片 - 优化验证策略
func (v *ImageSecurityValidator) deepValidateImage(
	data []byte,
	declaredFormat string,
) ValidationResult {
	result := ValidationResult{IsValid: false}

	// 1. 基础大小检查
	if int64(len(data)) > v.config.MaxFileSize {
		result.Error = fmt.Errorf(
			"文件大小超限: %d bytes，最大允许: %d bytes",
			len(data),
			v.config.MaxFileSize,
		)
		result.SecurityRisk = "文件过大，可能是DoS攻击"
		v.logger.Warn("检测到超大文件", map[string]interface{}{
			"size":     len(data),
			"max_size": v.config.MaxFileSize,
			"format":   declaredFormat,
		})
		return result
	}

	// 2. 格式支持检查
	if declaredFormat != "" && !v.isFormatAllowed(declaredFormat) {
		result.Error = fmt.Errorf("不支持的格式: %s", declaredFormat)
		result.SecurityRisk = "使用了不被允许的格式"
		return result
	}

	// 3. 恶意内容检测
	if v.config.EnableDeepScan && v.scanForMaliciousContent(data) {
		result.Error = fmt.Errorf("检测到潜在恶意内容")
		result.SecurityRisk = "可能包含恶意载荷"
		v.logger.Warn("检测到可疑内容", map[string]interface{}{
			"format": declaredFormat,
			"size":   len(data),
		})
		return result
	}

	// 4. 尝试解码图片获取详细信息（这是最可靠的验证方式）
	decodeResult := v.validateImageDecoding(data, declaredFormat)
	if !decodeResult.IsValid {
		// 图片解码失败，再检查文件头是否匹配
		if declaredFormat != "" && !v.validateFileSignature(data, declaredFormat) {
			// 记录警告但不直接失败，有些图片可能格式稍有不同但仍是有效的
			v.logger.Warn("文件头验证失败，但继续尝试解码", map[string]interface{}{
				"declared_format": declaredFormat,
				"actual_header":   fmt.Sprintf("%x", data[:min(len(data), 16)]),
			})
		}
		return decodeResult
	}

	// 图片解码成功，验证通过
	return decodeResult
}

// validateFileSignature 验证文件头签名
func (v *ImageSecurityValidator) validateFileSignature(data []byte, format string) bool {
	signature, exists := imageSignatures[strings.ToLower(format)]
	if !exists {
		return false
	}

	if len(data) < len(signature) {
		return false
	}

	// 检查文件头是否匹配
	for i, b := range signature {
		if data[i] != b {
			return false
		}
	}

	// WEBP需要额外验证
	if strings.ToLower(format) == "webp" && len(data) >= 12 {
		webpSignature := data[8:12]
		return bytes.Equal(webpSignature, []byte("WEBP"))
	}

	return true
}

// isFormatAllowed 检查格式是否被允许
func (v *ImageSecurityValidator) isFormatAllowed(format string) bool {
	formatLower := strings.ToLower(format)
	for _, allowedFormat := range v.config.AllowedFormats {
		if strings.ToLower(allowedFormat) == formatLower {
			return true
		}
	}
	return false
}

// scanForMaliciousContent 扫描恶意内容 - 优化后更加智能和宽松
func (v *ImageSecurityValidator) scanForMaliciousContent(data []byte) bool {
	// 首先尝试验证这是否是一个真正的图片文件
	// 如果能够正常解码为图片，那么即使包含一些可疑字节序列，也很可能是安全的
	reader := bytes.NewReader(data)
	if _, _, err := image.DecodeConfig(reader); err == nil {
		v.logger.Debug("文件能够正常解码为图片，跳过大部分恶意内容检测")
		// 对于能正常解码的图片，只进行最基本的检查
		return v.basicSecurityCheck(data)
	}

	v.logger.Info("文件无法解码为标准图片格式，进行完整的安全检测")
	return v.fullSecurityCheck(data)
}

// basicSecurityCheck 对能正常解码的图片进行基本安全检查
func (v *ImageSecurityValidator) basicSecurityCheck(data []byte) bool {
	// 只检查最明显的威胁：文件开头的可执行文件签名
	executableSignatures := [][]byte{
		{0x4D, 0x5A},             // PE文件头 (MZ) - 必须在文件开头
		{0x7F, 0x45, 0x4C, 0x46}, // ELF文件头 - 必须在文件开头
	}

	signatureNames := []string{"PE", "ELF"}

	for i, signature := range executableSignatures {
		if bytes.HasPrefix(data, signature) {
			v.logger.Warn("文件开头检测到可执行文件签名", map[string]interface{}{
				"signature_type": signatureNames[i],
				"signature_hex":  fmt.Sprintf("%x", signature),
			})
			return true
		}
	}

	// 检查SVG中的脚本内容
	dataStr := string(data)
	if strings.Contains(strings.ToLower(dataStr), "<svg") {
		return v.checkSVGScripts(dataStr)
	}

	v.logger.Debug("基本安全检查通过")
	return false
}

// fullSecurityCheck 对无法解码的文件进行完整安全检查
func (v *ImageSecurityValidator) fullSecurityCheck(data []byte) bool {
	// 检查可执行文件签名 - 只在文件开头检查
	executableSignatures := [][]byte{
		{0x4D, 0x5A},             // PE文件头 (MZ)
		{0x7F, 0x45, 0x4C, 0x46}, // ELF文件头
		{0xCA, 0xFE, 0xBA, 0xBE}, // Mach-O文件头
	}

	signatureNames := []string{"PE", "ELF", "Mach-O"}

	for i, signature := range executableSignatures {
		if bytes.HasPrefix(data, signature) {
			v.logger.Warn("文件开头检测到可执行文件签名", map[string]interface{}{
				"signature_type": signatureNames[i],
				"signature_hex":  fmt.Sprintf("%x", signature),
			})
			return true
		}
	}

	// 检查是否是压缩文件（ZIP/GZIP）- 也只在文件开头检查
	compressionSignatures := [][]byte{
		{0x50, 0x4B, 0x03, 0x04}, // ZIP文件头
		{0x1F, 0x8B, 0x08},       // GZIP文件头
	}

	compressionNames := []string{"ZIP", "GZIP"}

	for i, signature := range compressionSignatures {
		if bytes.HasPrefix(data, signature) {
			v.logger.Warn("文件开头检测到压缩文件签名", map[string]interface{}{
				"signature_type": compressionNames[i],
				"signature_hex":  fmt.Sprintf("%x", signature),
			})
			return true
		}
	}

	// 检查SVG中的脚本内容
	dataStr := string(data)
	if strings.Contains(strings.ToLower(dataStr), "<svg") {
		return v.checkSVGScripts(dataStr)
	}

	v.logger.Info("完整安全检查通过")
	return false
}

// checkSVGScripts 检查SVG文件中的脚本内容
func (v *ImageSecurityValidator) checkSVGScripts(dataStr string) bool {
	suspiciousStrings := []string{
		"<script",
		"javascript:",
		"vbscript:",
		"onload=",
		"onerror=",
		"eval(",
		"document.cookie",
		"window.location",
		"<iframe",
		"<object",
		"<embed",
	}

	dataStrLower := strings.ToLower(dataStr)
	for _, suspicious := range suspiciousStrings {
		if strings.Contains(dataStrLower, suspicious) {
			v.logger.Warn("在SVG中检测到可疑脚本内容", map[string]interface{}{
				"suspicious_content": suspicious,
			})
			return true
		}
	}

	return false
}

// validateImageDecoding 验证图片解码
func (v *ImageSecurityValidator) validateImageDecoding(
	data []byte,
	format string,
) ValidationResult {
	result := ValidationResult{Format: format}
	reader := bytes.NewReader(data)

	// 使用标准库解码验证
	config, actualFormat, err := image.DecodeConfig(reader)
	if err != nil {
		result.Error = fmt.Errorf("图片解码失败: %v", err)
		result.SecurityRisk = "可能包含恶意载荷或损坏的图片数据"
		return result
	}

	// 更新实际格式
	if actualFormat != "" {
		result.Format = actualFormat
	}

	// 检查尺寸限制
	if config.Width > v.config.MaxWidth || config.Height > v.config.MaxHeight {
		result.Error = fmt.Errorf("图片尺寸超限: %dx%d，最大允许: %dx%d",
			config.Width, config.Height, v.config.MaxWidth, v.config.MaxHeight)
		result.SecurityRisk = "图片过大，可能消耗过多资源"
		return result
	}

	// 检查像素总数
	totalPixels := int64(config.Width) * int64(config.Height)
	if totalPixels > v.config.MaxPixels {
		result.Error = fmt.Errorf("像素总数超限: %d，最大允许: %d", totalPixels, v.config.MaxPixels)
		result.SecurityRisk = "像素过多，可能导致内存耗尽"
		return result
	}

	// 验证成功
	result.IsValid = true
	result.Width = config.Width
	result.Height = config.Height
	result.FileSize = int64(len(data))

	v.logger.Debug("图片验证成功 %v", map[string]interface{}{
		"format": result.Format,
		"width":  result.Width,
		"height": result.Height,
		"size":   result.FileSize,
	})

	return result
}

// min 辅助函数：返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
