package asr

import (
	"context"
	"io"
	"xiaozhi-server-go/internal/domain/asr/aggregate"
)

type Service interface {
	Transcribe(ctx context.Context, req TranscribeRequest) (*TranscribeResponse, error)
	StreamTranscribe(ctx context.Context, audioStream <-chan []byte) (<-chan TranscribeChunk, error)
	ValidateConfig(config aggregate.Config) error
}

type TranscribeRequest struct {
	AudioData io.Reader
	Format    string
	Language  string
	Provider  string
	Config    aggregate.Config
}

type TranscribeResponse struct {
	Text       string
	Language   string
	Duration   float64
	Confidence float64
}

type TranscribeChunk struct {
	Text      string
	IsFinal   bool
	Timestamp int64
	Confidence float64
}