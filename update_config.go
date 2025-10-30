package main

import (
	"fmt"
	"log"

	"xiaozhi-server-go/internal/domain/config/manager"
	"xiaozhi-server-go/internal/platform/storage"
)

func main() {
	// 初始化数据库连接
	if err := storage.InitDatabase(); err != nil {
		log.Fatalf("Failed to init DB: %v", err)
	}

	// 创建数据库配置仓库
	repo := manager.NewDatabaseRepository(nil)

	// 加载配置
	cfg, err := repo.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 更新 ASR 配置
	if cfg.ASR == nil {
		cfg.ASR = make(map[string]interface{})
	}
	if cfg.ASR["DoubaoASR"] == nil {
		cfg.ASR["DoubaoASR"] = make(map[string]interface{})
	}

	doubaoASR := cfg.ASR["DoubaoASR"].(map[string]interface{})
	doubaoASR["appid"] = "8888673793"
	doubaoASR["access_token"] = "1BloSUUWMIQdOKKpb6EN_fMU_r7ATZHr"

	// 保存更新后的配置
	if err := repo.SaveConfig(cfg); err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}

	fmt.Printf("Configuration updated successfully!\n")

	// 重新加载配置验证
	cfg2, err := repo.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to reload config: %v", err)
	}

	asrData, ok := cfg2.ASR["DoubaoASR"]
	if !ok {
		log.Fatalf("DoubaoASR config not found after update")
	}

	fmt.Printf("Updated ASR.DoubaoASR config: %+v\n", asrData)
}