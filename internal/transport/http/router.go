package httptransport

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"

	"xiaozhi-server-go/internal/platform/observability"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/utils"
)

// Options configures the HTTP router builder.
type Options struct {
	Config         *configs.Config
	Logger         *utils.Logger
	AuthMiddleware gin.HandlerFunc
	StaticRoot     string
}

// Router bundles together the gin engine and common route groups.
type Router struct {
	Engine  *gin.Engine
	API     *gin.RouterGroup
	Secured *gin.RouterGroup
}

// Build constructs a gin engine pre-configured with logging, recovery, CORS and observability middlewares.
func Build(opts Options) (*Router, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("http router requires config")
	}
	logger := opts.Logger
	if logger == nil {
		logger = utils.DefaultLogger
	}

	if opts.Config.Log.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(loggingMiddleware(logger))
	engine.Use(observabilityMiddleware())

	engine.SetTrustedProxies([]string{"0.0.0.0"})

	engine.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Authorization",
			"Client-Id",
			"Device-Id",
			"Token",
		},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	staticRoot := opts.StaticRoot
	if staticRoot == "" {
		staticRoot = "./web"
	}
	engine.Use(static.Serve("/", static.LocalFile(staticRoot, true)))
	engine.Use(static.Serve("/ota", static.LocalFile(staticRoot, true)))

	api := engine.Group("/api")
	var secured *gin.RouterGroup
	if opts.AuthMiddleware != nil {
		secured = api.Group("")
		secured.Use(opts.AuthMiddleware)
	}

	return &Router{
		Engine:  engine,
		API:     api,
		Secured: secured,
	}, nil
}

func loggingMiddleware(logger *utils.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		status := c.Writer.Status()

		if logger != nil {
			logger.Info(
				"[HTTP] %s %s -> %d (%s)",
				c.Request.Method,
				c.Request.URL.Path,
				status,
				duration,
			)
		}
	}
}

func observabilityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		reqCtx, spanEnd := observability.StartSpan(c.Request.Context(), "http.server", path)
		var spanErr error
		c.Request = c.Request.WithContext(reqCtx)

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		if len(c.Errors) > 0 {
			spanErr = c.Errors.Last().Err
		} else if status := c.Writer.Status(); status >= http.StatusInternalServerError {
			spanErr = fmt.Errorf("status %d", status)
		}
		spanEnd(spanErr)

		observability.RecordMetric(
			reqCtx,
			"http.requests",
			1,
			map[string]string{
				"component": "http.server",
				"method":    c.Request.Method,
				"path":      path,
				"status":    strconv.Itoa(c.Writer.Status()),
			},
		)
		observability.RecordMetric(
			reqCtx,
			"http.request.duration_ms",
			float64(duration.Milliseconds()),
			map[string]string{
				"component": "http.server",
				"method":    c.Request.Method,
				"path":      path,
			},
		)
	}
}
