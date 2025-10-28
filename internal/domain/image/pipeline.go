package image

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/utils"
)

// Pipeline orchestrates streaming ingestion, validation, and encoding of image payloads.
type Pipeline struct {
	validator *SecurityValidator
	logger    *utils.Logger
	security  *configs.SecurityConfig
}

// Options configures the pipeline behaviour.
type Options struct {
	Security *configs.SecurityConfig
	Logger   *utils.Logger
}

// Input describes a streaming image payload.
type Input struct {
	Reader         io.Reader
	DeclaredFormat string
	Source         string
}

// Output contains the sanitised artefacts produced by the pipeline.
type Output struct {
	Base64       string
	Base64Reader io.ReadSeeker
	Bytes        []byte
	Format       string
	Validation   ValidationResult
}

// NewPipeline constructs a streaming image pipeline.
func NewPipeline(opts Options) (*Pipeline, error) {
	if opts.Security == nil {
		return nil, fmt.Errorf("security config is required")
	}
	if opts.Logger == nil {
		opts.Logger = utils.DefaultLogger
	}

	validator := NewSecurityValidator(opts.Security, opts.Logger)

	return &Pipeline{
		validator: validator,
		logger:    opts.Logger,
		security:  opts.Security,
	}, nil
}

// Process streams the input through validation and base64 encoding.
func (p *Pipeline) Process(ctx context.Context, input Input) (*Output, error) {
	if input.Reader == nil {
		return nil, fmt.Errorf("image reader is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	maxSize := p.security.MaxFileSize
	if maxSize <= 0 {
		maxSize = 5 * 1024 * 1024
	}

	limited := &io.LimitedReader{
		R: input.Reader,
		N: maxSize + 1,
	}

	rawBuf := bytes.NewBuffer(make([]byte, 0, 32*1024))
	base64Buf := bytes.NewBuffer(make([]byte, 0, 64*1024))

	encoder := base64.NewEncoder(base64.StdEncoding, base64Buf)
	writer := io.MultiWriter(rawBuf, encoder)

	if _, err := io.Copy(writer, limited); err != nil {
		return nil, fmt.Errorf("stream image bytes: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("finalise base64 encoding: %w", err)
	}

	if limited.N <= 0 {
		return nil, fmt.Errorf("image exceeds maximum size of %d bytes", maxSize)
	}

	rawBytes := rawBuf.Bytes()
	validation := p.validator.ValidateBytes(rawBytes, input.DeclaredFormat)
	if !validation.IsValid {
		if validation.Error != nil {
			return nil, validation.Error
		}
		return nil, fmt.Errorf("image validation failed")
	}

	sanitised := make([]byte, len(rawBytes))
	copy(sanitised, rawBytes)

	base64Data := base64Buf.String()
	base64Reader := bytes.NewReader([]byte(base64Data))

	return &Output{
		Base64:       base64Data,
		Base64Reader: base64Reader,
		Bytes:        sanitised,
		Format:       validation.Format,
		Validation:   validation,
	}, nil
}
