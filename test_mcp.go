package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

type FunctionCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ChatMessage struct {
	Role         string        `json:"role"`
	Content      string        `json:"content"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
}

func main() {
	// 连接到WebSocket服务器
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial("ws://localhost:8000", http.Header{})
	if err != nil {
		log.Fatal("Failed to connect to WebSocket:", err)
	}
	defer conn.Close()

	fmt.Println("Connected to WebSocket server")

	// 发送hello消息
	helloMsg := Message{
		Type: "hello",
		Data: map[string]interface{}{
			"version": 1,
			"audio_params": map[string]interface{}{
				"format": "opus",
				"sample_rate": 16000,
				"channels": 1,
				"frame_duration": 60,
			},
			"device_id": "test-device",
			"client_id": "test-client",
		},
	}

	if err := conn.WriteJSON(helloMsg); err != nil {
		log.Fatal("Failed to send hello message:", err)
	}

	fmt.Println("Sent hello message")

	// 读取服务器响应
	go func() {
		for {
			var msg Message
			err := conn.ReadJSON(&msg)
			if err != nil {
				log.Println("Read error:", err)
				return
			}
			fmt.Printf("Received: %+v\n", msg)
		}
	}()

	// 等待一段时间让连接建立
	time.Sleep(2 * time.Second)

	// 发送一个模拟LLM函数调用的消息来测试MCP工具
	// 这模拟了LLM决定调用time工具的情况
	functionCallMsg := Message{
		Type: "chat",
		Data: ChatMessage{
			Role:    "assistant",
			Content: "",
			FunctionCall: &FunctionCall{
				Name: "time",
				Arguments: map[string]interface{}{},
			},
		},
	}

	if err := conn.WriteJSON(functionCallMsg); err != nil {
		log.Fatal("Failed to send function call message:", err)
	}

	fmt.Println("Sent function call message for 'time' tool")

	// 等待响应
	time.Sleep(10 * time.Second)

	fmt.Println("Test completed")
}