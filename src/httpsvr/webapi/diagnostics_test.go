package webapi

import (
	"net"
	"testing"

	"xiaozhi-server-go/src/configs"
)

func TestRunDiagnosticsDetectsMissingProviderConfig(t *testing.T) {
	config := &configs.Config{}
	config.Web.Port = 8080
	config.Web.Websocket = "ws://127.0.0.1:8000"
	config.SelectedModule = map[string]string{"ASR": "missing", "LLM": "missing", "TTS": "missing"}

	results := runDiagnostics(config)
	if len(results) != 5 {
		t.Fatalf("result count = %d, want 5", len(results))
	}
	if results[2].Status != "error" || results[3].Status != "error" || results[4].Status != "error" {
		t.Fatalf("missing providers should be reported as errors: %#v", results)
	}
}

func TestProbeEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	result := probeEndpoint("测试服务", "http://"+listener.Addr().String())
	if result.Status != "ok" {
		t.Fatalf("probe result = %#v, want ok", result)
	}
}
