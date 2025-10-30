# AI Coding Instructions for xiaozhi-server-go

## Project Overview

**xiaozhi-server-go** is a Go-based backend server for an AI voice interaction robot system. It supports multiple AI models (ASR, TTS, LLM), real-time voice conversations via WebSocket/MQTT, and integrates with various clients (ESP32, Android, Python). The project is currently undergoing a major architectural migration from traditional monolithic structure to Domain-Driven Design (DDD).

## Architecture Overview

### Current Hybrid Architecture

The codebase exists in a **hybrid state** during migration:

- **New Architecture (`internal/`)**: DDD-based with clean separation of concerns
  - `internal/domain/`: Business logic and domain models
  - `internal/platform/`: Infrastructure (config, logging, storage, errors)
  - `internal/transport/`: HTTP/WebSocket interfaces
  - `internal/app/`: Application services

- **Legacy Architecture (`src/`)**: Traditional monolithic structure
  - `src/core/`: Core business logic (being migrated)
  - `src/httpsvr/`: HTTP services (being migrated)
  - `src/models/`: Data models

### Key Architectural Patterns

1. **Provider Pattern**: AI services (ASR, TTS, LLM) implement provider interfaces
   ```go
   type LLMProvider interface {
       GenerateResponse(ctx context.Context, req *LLMRequest) (*LLMResponse, error)
       StreamResponse(ctx context.Context, req *LLMRequest) (<-chan *LLMResponse, error)
   }
   ```

2. **Repository Pattern**: Data access abstraction
   ```go
   type LLMRepository interface {
       Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error)
       Stream(ctx context.Context, req GenerateRequest) (<-chan ResponseChunk, error)
   }
   ```

3. **Error Classification**: Custom error types with Kind classification
   ```go
   type Error struct {
       Kind    Kind    // KindConfig, KindDomain, KindTransport, etc.
       Op      string  // Operation name
       Message string
       Cause   error
   }
   ```

## Critical Developer Workflows

### Building and Running

```bash
# Build binary
make build
# or
go build -o xiaozhi-server ./cmd/xiaozhi-server

# Run development server
make run
# or
go run ./cmd/xiaozhi-server

# Run tests
make test
# or
go test ./...

# Generate API documentation
make swag
# Access docs at http://localhost:8080/docs
```

### Docker Deployment

```bash
# Build and run with Docker Compose
docker-compose up -d

# Build Docker image
docker build -t xiaozhi-server-go .
```

### Configuration

- **Primary config**: Database-backed configuration via web UI (`http://localhost:8080`)
- **Fallback config**: `platformconfig.DefaultConfig()` provides built-in defaults
- **File config**: YAML files loaded via viper (optional)

**Critical config for development**:
```yaml
web:
  websocket: ws://your-ip:8000  # WebSocket address for clients
  port: 8080                    # HTTP server port

server:
  port: 8000                    # WebSocket server port
```

## Project-Specific Conventions

### 1. Error Handling

Always use typed errors with Kind classification:

```go
// Good: Typed error with context
return platformerrors.Wrap(platformerrors.KindDomain, "llm.generate", "API call failed", err)

// Bad: Generic error
return fmt.Errorf("API call failed: %w", err)
```

### 2. Logging

Use structured logging with tag-based categorization:

```go
// Good: Tagged logging
logger.InfoTag("引导", "服务启动完成，监听端口 %d", config.Web.Port)

// Bad: Untagged logging
log.Printf("Server started on port %d", config.Web.Port)
```

### 3. Context Usage

Always pass context for cancellation and tracing:

```go
func (s *Service) ProcessRequest(ctx context.Context, req *Request) error {
    // Use context for API calls, database operations, etc.
    return s.repo.Save(ctx, req)
}
```

### 4. Configuration Management

Use the platform config system:

```go
// Good: Use platform config
config := platformconfig.DefaultConfig()

// Bad: Direct viper usage (unless necessary)
viper.GetString("llm.api_key")
```

### 5. Database Operations

Use GORM with proper error handling:

```go
func (r *repository) GetByID(ctx context.Context, id string) (*Model, error) {
    var model Model
    if err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, platformerrors.Wrap(platformerrors.KindStorage, "get_by_id", "record not found", err)
        }
        return nil, platformerrors.Wrap(platformerrors.KindStorage, "get_by_id", "database error", err)
    }
    return &model, nil
}
```

## Integration Points and Dependencies

### AI Model Providers

**LLM Providers** (in `src/core/providers/llm/`):
- `openai`: OpenAI GPT models
- `doubao`: ByteDance Doubao models
- `ollama`: Local Ollama models
- `coze`: Coze workflow models

**TTS Providers** (in `src/core/providers/tts/`):
- `doubao`: ByteDance TTS
- `edge`: Microsoft Edge TTS
- `cosyvoice`: CosyVoice models

**ASR Providers** (in `src/core/providers/asr/`):
- `doubao`: ByteDance ASR
- `deepgram`: Deepgram ASR
- `gosherpa`: Local Sherpa ASR

### MCP (Model Context Protocol)

Enables tool integration for enhanced AI capabilities:

```go
// MCP Manager handles tool registration and execution
mcpManager := mcp.NewManagerForPool(logger, config)
```

### Transport Protocols

**WebSocket** (`internal/transport/ws/`): Real-time voice communication
**HTTP** (`internal/transport/http/`): REST API and web interface
**MQTT+UDP**: Alternative transport for resource-constrained devices

### External Services

- **OTA Updates**: Firmware distribution to ESP32 devices
- **Vision API**: Image analysis (integrated with LLM)
- **Music Playback**: Audio file streaming
- **RAG System**: Document embedding and retrieval

## Key Files and Directories

### Entry Points
- `cmd/xiaozhi-server/main.go`: Application entry point
- `internal/bootstrap/bootstrap.go`: Service initialization and lifecycle

### Core Components
- `internal/domain/`: Business domain logic (LLM, ASR, TTS, Auth, etc.)
- `internal/platform/`: Infrastructure (config, logging, storage, errors)
- `src/core/`: Legacy core logic (migration in progress)
- `src/core/connection.go`: WebSocket connection handling

### Configuration
- `internal/platform/config/config.go`: Configuration structures
- `constants/constants.go`: Provider type constants

### Web Interface
- `web/`: Static web assets
- `src/httpsvr/`: HTTP handlers (being migrated)

### Data and Storage
- `data/xiaozhi.db`: SQLite database
- `rag/`: RAG system data (embeddings, documents)
- `tmp/`: Temporary audio files

## Development Best Practices

### 1. Architecture Migration Awareness

**During migration period**:
- New features: Implement in `internal/` using DDD patterns
- Bug fixes: Can be in either location, prefer `internal/` if possible
- Avoid adding new dependencies to `src/` components

### 2. Testing Strategy

```go
// Unit tests for domain logic
func TestLLMService_GenerateResponse(t *testing.T) {
    // Use test helpers from internal/platform/testing
    config := testing.SetupTestConfig(t)
    logger := testing.SetupTestLogger(t)
    
    // Test domain service logic
}

// Integration tests
func TestFullConversationFlow(t *testing.T) {
    // Test end-to-end conversation flow
}
```

### 3. Code Organization

**Domain Layer Structure**:
```
internal/domain/llm/
├── aggregate/     # Domain models
├── repository/    # Data access interfaces
├── service.go     # Domain services
└── inter/         # Domain interfaces
```

**Platform Layer Structure**:
```
internal/platform/
├── config/        # Configuration management
├── logging/       # Logging infrastructure
├── storage/       # Data persistence
├── errors/        # Error handling
└── observability/ # Monitoring and tracing
```

### 4. API Design

Follow RESTful conventions with consistent response format:

```go
type APIResponse struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data"`
    Message string      `json:"message"`
    Code    int         `json:"code"`
}
```

### 5. Performance Considerations

- **Connection Pooling**: Use resource pools for AI provider connections
- **Streaming**: Prefer streaming responses for real-time voice
- **Caching**: Implement caching for frequently accessed data
- **Async Processing**: Use goroutines for non-blocking operations

## Common Pitfalls to Avoid

### 1. Architecture Confusion

**Don't**: Mix new and old architecture patterns
```go
// Bad: Mixing architectures
func NewHandler(config *configs.Config, logger *utils.Logger) *Handler {
    // Uses old config and logger types
}
```

**Do**: Use new architecture consistently
```go
// Good: Pure new architecture
func NewHandler(config *platformconfig.Config, logger *platformlogging.Logger) *Handler {
    // Uses new platform types
}
```

### 2. Configuration Issues

**Don't**: Hardcode configuration values
```go
// Bad: Hardcoded values
client := openai.NewClient("sk-hardcoded-key")
```

**Do**: Use configuration system
```go
// Good: Configurable
client := openai.NewClient(config.LLM.APIKey)
```

### 3. Error Handling

**Don't**: Ignore errors or use generic errors
```go
// Bad: Silent failure
resp, _ := client.Call(ctx, req)
```

**Do**: Proper error handling with context
```go
// Good: Typed errors with context
resp, err := client.Call(ctx, req)
if err != nil {
    return platformerrors.Wrap(platformerrors.KindDomain, "call", "API request failed", err)
}
```

### 4. Resource Management

**Don't**: Forget to close resources
```go
// Bad: Resource leak
file, _ := os.Open("audio.wav")
// No defer close
```

**Do**: Proper resource cleanup
```go
// Good: Resource management
file, err := os.Open("audio.wav")
if err != nil {
    return err
}
defer file.Close()
```

## Debugging and Troubleshooting

### Common Issues

1. **WebSocket Connection Issues**
   - Check `web.websocket` config matches server IP
   - Verify firewall allows ports 8000 (WS) and 8080 (HTTP)

2. **AI Provider Errors**
   - Verify API keys in configuration
   - Check network connectivity to provider endpoints
   - Review provider-specific logs

3. **Database Issues**
   - Check SQLite file permissions (`data/xiaozhi.db`)
   - Verify database schema migrations
   - Check concurrent access patterns

### Logging and Monitoring

- **Application Logs**: `logs/server.log`
- **Web Interface**: `http://localhost:8080` for configuration and monitoring
- **API Documentation**: `http://localhost:8080/docs`

### Performance Profiling

```bash
# CPU profiling
go tool pprof http://localhost:8080/debug/pprof/profile

# Memory profiling
go tool pprof http://localhost:8080/debug/pprof/heap
```

## Getting Help

1. **Check existing code**: Look for similar patterns in `internal/domain/`
2. **Review tests**: Check `*_test.go` files for usage examples
3. **Read documentation**: Check `docs/` and `README.md`
4. **Web interface**: Use `/docs` for API reference

Remember: This codebase is in active migration. When in doubt, prefer the new DDD architecture in `internal/` over legacy code in `src/`.</content>
<parameter name="filePath">c:\Users\kalicyh\Documents\GitHub\xiaozhi-server-go\.github\copilot-instructions.md