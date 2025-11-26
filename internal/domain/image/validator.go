package image

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/utils"
)

// SecurityValidator performs layered security checks against incoming image payloads.
type SecurityValidator struct {
	config *config.SecurityConfig
	logger *utils.Logger
}

// NewSecurityValidator constructs a new validator instance.
func NewSecurityValidator(
	config *config.SecurityConfig,
	logger *utils.Logger,
) *SecurityValidator {
	return &SecurityValidator{
		config: config,
		logger: logger,
	}
}

var imageSignatures = map[string][]byte{
	"jpeg": {0xFF, 0xD8},
	"jpg":  {0xFF, 0xD8},
	"png":  {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
	"gif":  {0x47, 0x49, 0x46, 0x38},
	"webp": {0x52, 0x49, 0x46, 0x46},
	"bmp":  {0x42, 0x4D},
}

// ValidateBase64 validates a base64 encoded payload.
func (v *SecurityValidator) ValidateBase64(ImageData ImageData) ValidationResult {
	result := ValidationResult{IsValid: false}

	if ImageData.Data == "" {
		result.Error = fmt.Errorf("missing image payload")
		return result
	}

	raw, err := base64.StdEncoding.DecodeString(ImageData.Data)
	if err != nil {
		result.Error = fmt.Errorf("decode base64: %w", err)
		result.SecurityRisk = "invalid base64 encoding"
		return result
	}
	return v.deepValidateImage(raw, ImageData.Format)
}

// ValidateBytes validates raw bytes directly.
func (v *SecurityValidator) ValidateBytes(raw []byte, declaredFormat string) ValidationResult {
	return v.deepValidateImage(raw, declaredFormat)
}

func (v *SecurityValidator) deepValidateImage(
	ImageData []byte,
	declaredFormat string,
) ValidationResult {
	result := ValidationResult{IsValid: false}

	if len(ImageData) == 0 {
		result.Error = fmt.Errorf("empty image payload")
		return result
	}

	if int64(len(ImageData)) > v.config.MaxFileSize {
		result.Error = fmt.Errorf(
			"file size exceeds limit: %d bytes (max %d bytes)",
			len(ImageData),
			v.config.MaxFileSize,
		)
		result.SecurityRisk = "file too large"
		v.logger.Warn(
			"detected oversized image: size=%d max_size=%d format=%s",
			len(ImageData),
			v.config.MaxFileSize,
			declaredFormat,
		)
		return result
	}

	if declaredFormat != "" && !v.isFormatAllowed(declaredFormat) {
		result.Error = fmt.Errorf("unsupported format: %s", declaredFormat)
		result.SecurityRisk = "unapproved format"
		return result
	}

	decodeResult := v.validateImageDecoding(ImageData, declaredFormat)
	if !decodeResult.IsValid {
		if declaredFormat != "" && !v.validateFileSignature(ImageData, declaredFormat) {
			actualHeader := fmt.Sprintf("%x", ImageData[:min(len(ImageData), 16)])
			v.logger.Warn(
				"file signature mismatch: declared_format=%s actual_header=%s",
				declaredFormat,
				actualHeader,
			)
		}
		return decodeResult
	}

	result = decodeResult
	result.IsValid = true
	result.FileSize = int64(len(ImageData))
	return result
}

func (v *SecurityValidator) isFormatAllowed(format string) bool {
	if v.config == nil || len(v.config.AllowedFormats) == 0 {
		return true
	}
	if format == "" {
		return true
	}

	format = strings.ToLower(format)
	allowed := v.config.AllowedFormats
	if len(allowed) == 0 {
		allowed = []string{"jpeg", "jpg", "png", "webp", "gif", "bmp"}
	}

	for _, allowedFormat := range allowed {
		if strings.ToLower(allowedFormat) == format {
			return true
		}
	}
	return false
}

func (v *SecurityValidator) validateFileSignature(ImageData []byte, format string) bool {
	signature, ok := imageSignatures[strings.ToLower(format)]
	if !ok || len(signature) == 0 {
		return true
	}
	if len(ImageData) < len(signature) {
		return false
	}
	return bytes.Equal(signature, ImageData[:len(signature)])
}

func (v *SecurityValidator) scanForMaliciousContent(ImageData []byte) bool {
	suspiciousSignatures := [][]byte{
		{0x4D, 0x5A},
		{0x25, 0x50, 0x44, 0x46},
	}

	for _, signature := range suspiciousSignatures {
		if bytes.HasPrefix(ImageData, signature) {
			signatureHex := fmt.Sprintf("%x", signature)
			v.logger.Warn(
				"detected executable signature: signature_hex=%s",
				signatureHex,
			)
			return true
		}
	}

	compressionSignatures := [][]byte{
		{0x50, 0x4B, 0x03, 0x04},
		{0x1F, 0x8B, 0x08},
	}

	for _, signature := range compressionSignatures {
		if bytes.HasPrefix(ImageData, signature) {
			signatureHex := fmt.Sprintf("%x", signature)
			v.logger.Warn(
				"detected compressed archive: signature_hex=%s",
				signatureHex,
			)
			return true
		}
	}

	ImageDataStr := string(ImageData)
	if strings.Contains(strings.ToLower(ImageDataStr), "<svg") {
		return v.checkSVGScripts(ImageDataStr)
	}

	return false
}

func (v *SecurityValidator) checkSVGScripts(ImageDataStr string) bool {
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

	ImageDataStrLower := strings.ToLower(ImageDataStr)
	for _, suspicious := range suspiciousStrings {
		if strings.Contains(ImageDataStrLower, suspicious) {
			v.logger.Warn("detected suspicious SVG content: token=%s", suspicious)
			return true
		}
	}
	return false
}

func (v *SecurityValidator) validateImageDecoding(
	ImageData []byte,
	format string,
) ValidationResult {
	result := ValidationResult{Format: format}
	reader := bytes.NewReader(ImageData)

	config, actualFormat, err := image.DecodeConfig(reader)
	if err != nil {
		result.Error = fmt.Errorf("decode image config: %w", err)
		result.SecurityRisk = "corrupted image ImageData"
		return result
	}

	if actualFormat != "" {
		result.Format = actualFormat
	}

	if config.Width > v.config.MaxWidth || config.Height > v.config.MaxHeight {
		result.Error = fmt.Errorf("dimensions exceed limit: %dx%d (max %dx%d)",
			config.Width, config.Height, v.config.MaxWidth, v.config.MaxHeight)
		result.SecurityRisk = "dimensions too large"
		return result
	}

	totalPixels := int64(config.Width) * int64(config.Height)
	if totalPixels > v.config.MaxPixels {
		result.Error = fmt.Errorf("pixel count exceeds limit: %d (max %d)", totalPixels, v.config.MaxPixels)
		result.SecurityRisk = "pixel count too high"
		return result
	}

	if v.config.EnableDeepScan && v.scanForMaliciousContent(ImageData) {
		result.Error = fmt.Errorf("potential malicious content detected")
		result.SecurityRisk = "suspicious content"
		return result
	}

	result.IsValid = true
	result.Width = config.Width
	result.Height = config.Height
	result.FileSize = int64(len(ImageData))

	v.logger.Debug(
		"image validation success: format=%s width=%d height=%d size=%d",
		result.Format,
		result.Width,
		result.Height,
		result.FileSize,
	)

	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
