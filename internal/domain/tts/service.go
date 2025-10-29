package tts

import (
	"context"
	"io"
	"xiaozhi-server-go/internal/domain/tts/aggregate"
)

type Service interface {
	Synthesize(ctx context.Context, req SynthesizeRequest) (*SynthesizeResponse, error)
	GetVoices(ctx context.Context, provider string) ([]Voice, error)
	ValidateConfig(config aggregate.Config) error
}

type SynthesizeRequest struct {
	Text     string
	Voice    string
	Provider string
	Config   aggregate.Config
}

type SynthesizeResponse struct {
	AudioData io.Reader
	Format    string
	Duration  float64
	Size      int64
}

type Voice struct {
	Name        string
	DisplayName string
	Language    string
	Gender      string
	Description string
}