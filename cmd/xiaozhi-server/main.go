// @title 小智服务端 API 文档
// @version 1.0
// @description 小智服务端，包含OTA与Vision等接口
// @host localhost:8080
// @BasePath /api
package main

import (
	"context"
	"fmt"
	"os"

	"xiaozhi-server-go/internal/bootstrap"
)

func main() {
	if err := bootstrap.Run(context.Background()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "xiaozhi-server failed: %v\n", err)
		os.Exit(1)
	}
}
