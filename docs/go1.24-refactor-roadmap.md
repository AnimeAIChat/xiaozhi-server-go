# Go 1.24 升级与架构重塑路线图

## 背景与目标
- **充分利用 Go 1.24**：采用 `slog`、`context.WithTimeoutCause`、`maps`/`slices` 泛型工具等新特性，提升代码简洁度与并发安全。
- **重塑可维护性**：拆分巨型入口、管控依赖注入，建立模块化、可测试的内部结构。
- **增强可观测性与运维**：引入统一日志、指标、链路追踪与标准化启动、部署流程。
- **降低技术债务**：逐步迁移 Legacy 模块，补齐测试与文档，确保回归稳定。

## 目标架构概览
- `cmd/xiaozhi-server`：应用入口，仅负责构建上下文与调用引导逻辑。
- `internal/bootstrap`：生命周期管理、依赖装配、服务启动/关闭。
- `internal/platform/...`：配置、日志、存储、错误、观测等横切能力。
- `internal/domain/{auth,chat,vision,image,mcp}`：领域服务及业务流程。
- `internal/transport/{http,ws,mcp}`：面向协议的传输层，包含路由、中间件、会话管理。
- `tests/{unit,e2e}`：单元与端到端测试，结合 `testcontainers-go` 等工具。

## 里程碑计划

### Phase 0 · 骨架奠定（约 1 周）
- 创建新入口与 `bootstrap`，迁移旧 `main.go` 功能。
- 实施 `internal/platform/logging`，封装 `slog`，适配旧日志接口。
- `internal/config`：实现 Loader/Schema/Defaults/Validate，保留数据库写回桥接。
- 引入 `internal/platform/runtime.App`，统一 `errgroup.WithContext`、`signal.NotifyContext`、`errors.Join` 处理。
- 确认旧业务在新骨架下可启动，记录暂存的 TODO。

### Phase 1 · 横切资源整合（约 1-2 周）
- 拆分存储层：`internal/platform/storage` 支持配置、会话、日志的抽象。
- `internal/platform/observability` 集成 OpenTelemetry、Prometheus，编写 Gin/WebSocket 中间件。
- 建立统一错误模型（`internal/platform/errors`），替换配置、数据库初始化的原始打印。
- 调整 `bootstrap` 启动顺序，生成依赖拓扑图。
- 新增冒烟测试（`go test ./internal/bootstrap -run Smoke`），验证启动/关闭流程。

### Phase 2 · 传输与资源池重塑（约 2 周）
- `internal/transport/ws`：提炼 Session/Hub/Router，使用 `atomic.Bool`、`context.WithTimeoutCause` 管理生命周期。
- `internal/domain/providers`：定义 Provider 接口、注册表、统一初始化；`pool` 改用 `sync.Pool` + `maps.Clone`。
- HTTP 层重构：路由注册迁移到 `internal/transport/http/router.go`，整合 Auth、CORS、Logging、Recovery 中间件。
- 完成旧 `core/transport` 到新结构的迁移，保留临时兼容层。

### Phase 3 · 业务域迁移（约 3 周）
- `internal/domain/auth`：重写 AuthManager，使用 `time.Duration`、pluggable Store（memory/sqlite/redis）。
- `internal/domain/mcp`：拆分客户端、工具注册、调用执行模块，利用 `maps`/`slices` 提升安全与可读性。
- `internal/domain/vision`、`image`：引入 Validator/Pipeline，使用 `io.Reader`/`io.Pipe` 控制内存；统一 Provider 调用。
- 对话管理模块抽象为 `internal/domain/chat/dialogue`，补齐 streaming/历史管理测试。
- 每个领域迁移后补齐单元 + 端到端测试，确保回归稳定。

### Phase 4 · 性能与运维（持续迭代）
- 性能调优：加入 pprof、trace，编写基准测试与 FlameGraph 脚本。
- 运维体系：更新 Dockerfile（多阶段构建）、Makefile/Taskfile（lint/test/run/migrate），引入 ADR 文档与部署指南。
- 安全增强：密钥轮换、速率限制、token 签名升级 (`crypto/ed25519`)。
- 协作机制：制定迁移 Checklist、周度同步，维护路线图进展。

## 关键交付物
- `docs/ADR-0001-architecture.md`：目标架构与依赖流描述。
- `cmd/internal` 新目录结构与最小可运行应用。
- 统一日志、配置、观测模块与对应测试。
- Provider & Transport 重构后的接口文档与示例。
- 自动化测试与 CI/CD 流程（lint、unit、e2e、安全扫描）。
- 更新后的部署脚本、Dockerfile、README/运维手册。

## 风险与缓解
- **迁移跨度大**：采用分阶段、双轨运行与兼容层，确保每阶段可回滚。
- **测试缺失**：优先搭建测试框架，迁移即补齐回归用例。
- **新架构学习成本**：通过 ADR、代码自述、周会同步降低认知门槛。
- **性能回退**：在每阶段加入基准测试与 profiling，及时调整。

## 立即行动项
1. 创建 Phase 0 分支和基础目录，提交最小骨架 PR。
2. 撰写 ADR 草稿并在团队评审，统一目标结构。
3. 建立模块映射表与负责人分工，评估工作量。
4. 搭建初始 `Makefile` 任务（`run`, `test`, `lint`），为后续迭代铺路。

