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
	"time"

	"xiaozhi-server-go/internal/bootstrap"
)

func main() {
	fmt.Printf("[%s] [INFO] [引导] 开始启动 xiaozhi-server...\n", time.Now().Format("2006-01-02 15:04:05.000"))
	if err := bootstrap.Run(context.Background()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "xiaozhi-server failed: %v\n", err)
		os.Exit(1)
	}
}
