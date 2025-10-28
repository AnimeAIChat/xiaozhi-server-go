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

### Phase 0 —— 启动基线（约 1 周）
- ✅ 确认新建 `bootstrap`，迁移原 `main.go` 逻辑。
- ✅ 施行 `internal/platform/logging`，封装 `slog` 与 legacy 日志接口。
- ✅ `internal/platform/config` 实现 Loader/Schema/Defaults/Validate，并迁移数据库写入逻辑。
- ✅ 搭建 `internal/platform/runtime.App`，统一 `errgroup.WithContext`、`signal.NotifyContext`、`errors.Join` 调用。
- ✅ 确认旧业务模块的关键清单及阶段性记录 TODO。

### Phase 1 —— 平台能力铺设（约 1-2 周）
- ✅ 启动 `internal/platform/config` Loader，打通 `internal/platform/storage` 配置存储。
- ✅ 落地 `internal/platform/logging`，统一使用 slog 封装替换 `utils.Logger`。
- ✅ 整理 Swagger 生成，Makefile + `.swagignore` 控制扫描范围，压缩冗余输出。
- ✅ 提炼统一错误模型并接入 `internal/platform/errors`，完善 bootstrap 适配。
- ✅ 搭建可观测性脚手架，`internal/platform/observability` 在 Run 阶段预留关闭链路。
- ✅ 添加 bootstrap smoke test，确保 `go test ./internal/bootstrap` 可通过。
- ✅ 对齐 `bootstrap` 启动顺序与依赖图文档。
- ✅ 抽离 observability/错误骨架到下游模块，确保可测试的骨架。
### Phase 2 · 传输与资源池重塑（约 2 周）
 - ✅ internal/transport/ws 已完成 Session/Hub/Router 重构，统一使用 tomic.Bool 与 context.WithTimeoutCause 管理生命周期。
 - ✅ internal/domain/providers 已封装 Provider 注册与统一初始化，pool 基于 sync.Pool + maps.Clone 提供线程安全复用。
 - ✅ HTTP 路由迁移至 internal/transport/http/router.go，集中编排 Auth / CORS / Logging / Recovery 等中间件。
 - ✅ 旧 core/transport 已切换至新实现并保留兼容层，保障迁移期稳定性。

### Phase 3 · 业务域迁移（约 3 周）
- [x] `internal/domain/auth`����д AuthManager��ʹ�� `time.Duration`��pluggable Store��memory/sqlite/redis����
- [x] `internal/domain/mcp`����ֿͻ��ˡ�����ע�ᡢ����ִ��ģ�飬���� `maps`/`slices` ������ȫ��ɶ��ԡ�
- [x] `internal/domain/vision`��`image`������ Validator/Pipeline��ʹ�� `io.Reader`/`io.Pipe` �����ڴ棻ͳһ Provider ���á�
- 对话管理模块抽象为 `internal/domain/chat/dialogue`，补齐 streaming/历史管理测试。
- 每个领域迁移后补齐单元 + 端到端测试，确保回归稳定。

### Phase 4 · 性能与运维（持续迭代）
- 性能调优：加入 pprof、trace，编写基准测试与 FlameGraph 脚本。
- 运维体系：更新 Dockerfile（多阶段构建）、Makefile/Taskfile（lint/test/run/migrate），引入 ADR 文档与部署指南。
- 安全增强：密钥轮换、速率限制、token 签名升级 (`crypto/ed25519`)。
- 协作机制：制定迁移 Checklist、周度同步，维护路线图进展。

## 关键交付物
- `docs/ADR-0001-architecture.md、docs/ADR-0002-bootstrap-deps.md`：目标架构与依赖流描述。
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



