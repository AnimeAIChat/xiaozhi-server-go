# ADR-0002 启动依赖图

## 背景
启动流程依赖平台层，先加载配置、日志和可观测性，再编排应用服务。记录顺序可以避免未来回归。

## 决策
启动流程通过 `InitGraph()` 暴露显式初始化图，步骤如下：

| 步骤 ID | 描述 | 依赖 | 产出 |
| --- | --- | --- | --- |
| `storage:init-config-store` | 初始化 SQLite 配置存储。 | - | 配置数据库就绪，可读取配置。 |
| `config:load-runtime` | 使用 `platformconfig.Loader` 加载运行时配置。 | `storage:init-config-store` | `appState.config`、`configPath` 已填充。 |
| `logging:init-provider` | 创建日志提供者，串联 legacy 与 slog。 | `config:load-runtime` | `appState.logger` 准备完毕，并注册数据库 logger。 |
| `observability:setup-hooks` | 调用 `platformobservability.Setup` 挂载可观测性钩子。 | `logging:init-provider` | 记录关闭钩子备用。 |
| `auth:init-manager` | 依据配置与日志构建认证管理器。 | `observability:setup-hooks` | `appState.authManager` 可供各 Transport 使用。 |

`Run` 通过 `executeInitSteps` 执行该依赖图，然后调用 `startServices`。`logBootstrapGraph` 输出同样顺序，确保运行日志、代码与 ADR 一致。

## 状态
已采纳。

## 影响
- 可观测性关闭钩子始终注册（即便当前仅返回 no-op），在退出时安全执行。
- 冒烟测试断言依赖图顺序，避免代码与文档偏离。
