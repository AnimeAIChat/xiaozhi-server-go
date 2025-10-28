package main

import (
	"fmt"
	"log"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/utils"
)

// TestResource 简单的测试资源
type TestResource struct {
	id    int
	valid bool
}

func (r *TestResource) Reset() error {
	r.valid = true
	return nil
}

func (r *TestResource) IsValid() bool {
	return r.valid
}

func (r *TestResource) Cleanup() error {
	fmt.Printf("清理资源 %d\n", r.id)
	return nil
}

// TestFactory 测试工厂
type TestFactory struct {
	counter int
}

func (f *TestFactory) Create() (interface{}, error) {
	f.counter++
	resource := &TestResource{
		id:    f.counter,
		valid: true,
	}
	fmt.Printf("创建资源 %d\n", resource.id)
	return resource, nil
}

func (f *TestFactory) Destroy(resource interface{}) error {
	if res, ok := resource.(*TestResource); ok {
		return res.Cleanup()
	}
	return nil
}

func main() {
	// 创建日志器
	logger := utils.NewLogger("test", utils.InfoLevel)

	// 创建资源池配置
	config := pool.PoolConfig{
		MinSize: 2,
		MaxSize: 5,
	}

	// 创建工厂
	factory := &TestFactory{}

	// 创建资源池
	resourcePool, err := pool.NewResourcePool("test_pool", factory, config, logger)
	if err != nil {
		log.Fatalf("创建资源池失败: %v", err)
	}
	defer resourcePool.Close()

	fmt.Println("=== 资源池创建成功 ===")

	// 测试获取资源
	fmt.Println("\n=== 测试获取资源 ===")
	res1, err := resourcePool.Get()
	if err != nil {
		log.Fatalf("获取资源1失败: %v", err)
	}
	fmt.Printf("获取到资源: %d\n", res1.(*TestResource).id)

	res2, err := resourcePool.Get()
	if err != nil {
		log.Fatalf("获取资源2失败: %v", err)
	}
	fmt.Printf("获取到资源: %d\n", res2.(*TestResource).id)

	// 查看统计信息
	fmt.Println("\n=== 查看统计信息 ===")
	available, total := resourcePool.GetStats()
	detailed := resourcePool.GetDetailedStats()
	fmt.Printf("可用资源: %d, 总资源: %d\n", available, total)
	fmt.Printf("详细统计: %+v\n", detailed)

	// 测试归还资源
	fmt.Println("\n=== 测试归还资源 ===")
	err = resourcePool.Put(res1)
	if err != nil {
		log.Printf("归还资源1失败: %v", err)
	} else {
		fmt.Println("成功归还资源1")
	}

	err = resourcePool.Put(res2)
	if err != nil {
		log.Printf("归还资源2失败: %v", err)
	} else {
		fmt.Println("成功归还资源2")
	}

	// 再次查看统计信息
	fmt.Println("\n=== 归还后的统计信息 ===")
	available, total = resourcePool.GetStats()
	detailed = resourcePool.GetDetailedStats()
	fmt.Printf("可用资源: %d, 总资源: %d\n", available, total)
	fmt.Printf("详细统计: %+v\n", detailed)

	// 测试Resize功能
	fmt.Println("\n=== 测试Resize功能 ===")
	err = resourcePool.Resize(3)
	if err != nil {
		log.Printf("Resize失败: %v", err)
	} else {
		fmt.Println("成功调整池大小为3")
	}

	// 获取完整的统计信息
	fmt.Println("\n=== 完整统计信息 ===")
	fullStats := resourcePool.GetPoolStats()
	for k, v := range fullStats {
		fmt.Printf("%s: %v\n", k, v)
	}

	fmt.Println("\n=== 测试完成 ===")
}