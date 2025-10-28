package vad

import (
	"errors"
	"xiaozhi-server-go/internal/domain/vad/inter"
	// "xiaozhi-server-go/internal/domain/vad/silero_vad"
	// "xiaozhi-server-go/internal/domain/vad/webrtc_vad"
)

func AcquireVAD(provider string, config map[string]interface{}) (inter.VAD, error) {
	switch provider {
	// case constants.VadTypeSileroVad:
	//	return silero_vad.AcquireVAD(config)
	// case constants.VadTypeWebRTCVad:
	//	return webrtc_vad.AcquireVAD(config)
	default:
		return nil, errors.New("VAD not implemented yet")
	}
}

func ReleaseVAD(vad inter.VAD) error {
	// TODO: Implement ReleaseVAD
	return errors.New("VAD not implemented yet")
}