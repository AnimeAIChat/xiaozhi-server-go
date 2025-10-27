package webapi

import (
	"encoding/json"
	"fmt"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SystemConfigService 系统配置服务
type SystemConfigService struct {
	logger *utils.Logger
	db     *gorm.DB
}

// NewSystemConfigService 构造函数
func NewSystemConfigService(logger *utils.Logger, db *gorm.DB) *SystemConfigService {
	return &SystemConfigService{
		logger: logger,
		db:     db,
	}
}

// RegisterRoutes 注册路由
func (s *SystemConfigService) RegisterRoutes(apiGroup *gin.RouterGroup) {
	// 需要管理员权限的配置管理路由
	adminGroup := apiGroup.Group("/admin/config")
	adminGroup.Use(AuthMiddleware(), AdminMiddleware())
	{
		// 应用配置
		adminGroup.GET("/application", s.handleGetApplicationConfig)
		adminGroup.PUT("/application", s.handleUpdateApplicationConfig)

		// 认证配置
		adminGroup.GET("/auth", s.handleGetAuthConfig)
		adminGroup.PUT("/auth", s.handleUpdateAuthConfig)

		// 传输配置
		adminGroup.GET("/transport", s.handleGetTransportConfig)
		adminGroup.PUT("/transport", s.handleUpdateTransportConfig)

		// Web配置
		adminGroup.GET("/web", s.handleGetWebConfig)
		adminGroup.PUT("/web", s.handleUpdateWebConfig)

		// 日志配置
		adminGroup.GET("/log", s.handleGetLogConfig)
		adminGroup.PUT("/log", s.handleUpdateLogConfig)

		// 角色配置
		adminGroup.GET("/roles", s.handleGetRoleConfigs)
		adminGroup.POST("/roles", s.handleCreateRoleConfig)
		adminGroup.PUT("/roles/:id", s.handleUpdateRoleConfig)
		adminGroup.DELETE("/roles/:id", s.handleDeleteRoleConfig)

		// 快捷回复词
		adminGroup.GET("/quick-reply", s.handleGetQuickReplyWords)
		adminGroup.POST("/quick-reply", s.handleCreateQuickReplyWord)
		adminGroup.PUT("/quick-reply/:id", s.handleUpdateQuickReplyWord)
		adminGroup.DELETE("/quick-reply/:id", s.handleDeleteQuickReplyWord)

		// 本地MCP功能
		adminGroup.GET("/mcp-functions", s.handleGetMCPFunctions)
		adminGroup.POST("/mcp-functions", s.handleCreateMCPFunction)
		adminGroup.PUT("/mcp-functions/:id", s.handleUpdateMCPFunction)
		adminGroup.DELETE("/mcp-functions/:id", s.handleDeleteMCPFunction)

		// 退出指令
		adminGroup.GET("/exit-commands", s.handleGetExitCommands)
		adminGroup.POST("/exit-commands", s.handleCreateExitCommand)
		adminGroup.PUT("/exit-commands/:id", s.handleUpdateExitCommand)
		adminGroup.DELETE("/exit-commands/:id", s.handleDeleteExitCommand)
	}
}

type ApplicationConfig struct {
	EnableMCPFilter bool `json:"enableMCPFilter"`
	QuickReply      bool `json:"quickReply"`
	SaveTtsAudio    bool `json:"saveTtsAudio"`
	SaveUserAudio   bool `json:"saveUserAudio"`
}

// 应用配置相关处理器
func (s *SystemConfigService) handleGetApplicationConfig(c *gin.Context) {
	var config ApplicationConfig
	config.SaveTtsAudio = configs.Cfg.SaveTTSAudio
	config.SaveUserAudio = configs.Cfg.SaveUserAudio
	config.QuickReply = configs.Cfg.QuickReply

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   config,
	})
}

func (s *SystemConfigService) handleUpdateApplicationConfig(c *gin.Context) {
	var config ApplicationConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	configs.Cfg.SaveTTSAudio = config.SaveTtsAudio
	configs.Cfg.SaveUserAudio = config.SaveUserAudio
	configs.Cfg.QuickReply = config.QuickReply
	fmt.Println("设置应用配置：", config)
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "应用配置更新成功",
		"data":    config,
	})
}

type AuthConfig struct {
	Token  string `json:"token"`
	Expiry int    `json:"expiry"`
}

// 认证配置相关处理器
func (s *SystemConfigService) handleGetAuthConfig(c *gin.Context) {
	var config AuthConfig
	config.Token = configs.Cfg.Server.Token
	config.Expiry = configs.Cfg.Server.Auth.Store.Expiry

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   config,
	})
}

func (s *SystemConfigService) handleUpdateAuthConfig(c *gin.Context) {
	var config AuthConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}
	configs.Cfg.Server.Token = config.Token
	configs.Cfg.Server.Auth.Store.Expiry = config.Expiry
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "认证配置更新成功",
		"data":    config,
	})
}

type TransportConfig struct {
	Type    string          `json:"type"` // websocket/mqtt_udp
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config"` // arbitrary JSON payload
}

// 传输配置相关处理器
func (s *SystemConfigService) handleGetTransportConfig(c *gin.Context) {
	var configsTransport []TransportConfig
	configsTransport = make([]TransportConfig, 0)
	str, _ := json.Marshal(configs.Cfg.Transport.WebSocket)
	configsTransport = append(configsTransport, TransportConfig{
		Type:    "websocket",
		Enabled: configs.Cfg.Transport.WebSocket.Enabled,
		Config:  str,
	})
	fmt.Println("获取 WebSocket 配置：", configs.Cfg.Transport.WebSocket.Enabled)

	strMqttUdp, _ := json.Marshal(configs.Cfg.Transport.MQTTUDP)
	configsTransport = append(configsTransport, TransportConfig{
		Type:    "mqtt_udp",
		Enabled: configs.Cfg.Transport.MQTTUDP.Enabled,
		Config:  strMqttUdp,
	})
	c.JSON(200, gin.H{
		"status": "ok",
		"data":   configsTransport,
	})
}

func (s *SystemConfigService) handleUpdateTransportConfig(c *gin.Context) {
	var configsTransport []TransportConfig
	if err := c.ShouldBindJSON(&configsTransport); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		fmt.Println("绑定传输配置错误：", err)
		return
	}
	fmt.Println("更新传输配置：", configsTransport)

	for _, cfg := range configsTransport {
		// 更新传输配置
		switch cfg.Type {
		case "websocket":
			configs.Cfg.Transport.WebSocket.Enabled = cfg.Enabled
			fmt.Println("更新WebSocket配置：", (cfg.Enabled))
			json.Unmarshal(cfg.Config, &configs.Cfg.Transport.WebSocket)
			fmt.Println("更新WebSocket配置内容：", configs.Cfg.Transport.WebSocket)
		case "mqtt_udp":
			configs.Cfg.Transport.MQTTUDP.Enabled = cfg.Enabled
			json.Unmarshal(cfg.Config, &configs.Cfg.Transport.MQTTUDP)
		}
	}
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "传输配置更新成功",
		"data":    configsTransport,
	})
}

// WebConfig Web界面配置
type WebConfig struct {
	Enabled      bool   `json:"enabled"`
	Port         int    `json:"port"`
	StaticDir    string `json:"staticDir"`
	Websocket    string `json:"websocket"`
	VisionURL    string `json:"visionUrl"`
	ActivateText string `json:"activateText"` // 发送激活码时携带的文本
}

// LogConfig 日志配置
type LogConfig struct {
	LogLevel string `json:"logLevel"`
	LogDir   string `json:"logDir"`
	LogFile  string `json:"logFile"`
}

// RoleConfig 角色配置
type RoleConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// QuickReplyWord 快捷回复词
type QuickReplyWord struct {
	Word    string `json:"word"`
	Enabled bool   `json:"enabled"`
	Order   int    `json:"order"`
}

// LocalMCPFunction 本地MCP功能配置
type LocalMCPFunction struct {
	FunctionName string `json:"functionName"`
	Description  string `json:"description"`
	Enabled      bool   `json:"enabled"`
}

// ExitCommand 退出指令
type ExitCommand struct {
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
}

// Web配置相关处理器（使用 configs.Cfg 读取/保存）
func (s *SystemConfigService) handleGetWebConfig(c *gin.Context) {
	// 构造返回 DTO，从全局配置读取
	config := WebConfig{
		Port:         configs.Cfg.Web.Port,
		StaticDir:    configs.Cfg.Web.StaticDir,
		Websocket:    configs.Cfg.Web.Websocket,
		VisionURL:    configs.Cfg.Web.VisionURL,
		ActivateText: configs.Cfg.Web.ActivateText,
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   config,
	})
}

func (s *SystemConfigService) handleUpdateWebConfig(c *gin.Context) {
	var payload WebConfig
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	// 将 DTO 应用到全局配置并保存到数据库
	configs.Cfg.Web.Port = payload.Port
	configs.Cfg.Web.StaticDir = payload.StaticDir
	configs.Cfg.Web.Websocket = payload.Websocket
	configs.Cfg.Web.VisionURL = payload.VisionURL
	configs.Cfg.Web.ActivateText = payload.ActivateText

	if err := configs.Cfg.SaveToDB(database.GetServerConfigDB()); err != nil {
		s.logger.Error("保存 Web 配置到数据库失败: %v", err)
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "保存 Web 配置失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "Web 配置更新成功",
		"data":    payload,
	})
}

// 日志配置相关处理器
func (s *SystemConfigService) handleGetLogConfig(c *gin.Context) {
	// 从全局配置读取并返回 DTO
	cfg := LogConfig{
		LogLevel: configs.Cfg.Log.LogLevel,
		LogDir:   configs.Cfg.Log.LogDir,
		LogFile:  configs.Cfg.Log.LogFile,
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   cfg,
	})
}

func (s *SystemConfigService) handleUpdateLogConfig(c *gin.Context) {
	var payload LogConfig
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	// 应用到全局配置并保存
	configs.Cfg.Log.LogLevel = payload.LogLevel
	configs.Cfg.Log.LogDir = payload.LogDir
	configs.Cfg.Log.LogFile = payload.LogFile

	if err := configs.Cfg.SaveToDB(database.GetServerConfigDB()); err != nil {
		s.logger.Error("保存日志配置到数据库失败: %v", err)
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "保存日志配置失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "日志配置更新成功",
		"data":    payload,
	})
}

// 角色配置相关处理器
func (s *SystemConfigService) handleGetRoleConfigs(c *gin.Context) {
	var roles []RoleConfig
	for _, role := range configs.Cfg.Roles {
		roles = append(roles, RoleConfig{
			Name:        role.Name,
			Description: role.Description,
			Enabled:     role.Enabled,
		})
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   roles,
	})
}

func (s *SystemConfigService) handleCreateRoleConfig(c *gin.Context) {
	var config RoleConfig

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}
	configs.Cfg.Roles = append(configs.Cfg.Roles, configs.Role{
		Name:        config.Name,
		Description: config.Description,
		Enabled:     config.Enabled,
	})
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "角色配置创建成功",
		"data":    config,
	})
}

func (s *SystemConfigService) handleUpdateRoleConfig(c *gin.Context) {

	var config RoleConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	configs.Cfg.Roles = []configs.Role{}
	configs.Cfg.Roles = append(configs.Cfg.Roles, configs.Role{
		Name:        config.Name,
		Description: config.Description,
		Enabled:     config.Enabled,
	})
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "角色配置更新成功",
	})
}

func (s *SystemConfigService) handleDeleteRoleConfig(c *gin.Context) {
	name := c.Param("id")
	newRoles := []configs.Role{}
	for _, role := range configs.Cfg.Roles {
		if role.Name != name {
			newRoles = append(newRoles, role)
		}
	}
	configs.Cfg.Roles = newRoles
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "角色配置删除成功",
	})
}

// 快捷回复词相关处理器
func (s *SystemConfigService) handleGetQuickReplyWords(c *gin.Context) {
	var words []QuickReplyWord
	words = make([]QuickReplyWord, 0)
	for _, dbWord := range configs.Cfg.QuickReplyWords {
		words = append(words, QuickReplyWord{
			Word:    dbWord,
			Enabled: true,
		})
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   words,
	})
}

func (s *SystemConfigService) handleCreateQuickReplyWord(c *gin.Context) {
	var word QuickReplyWord
	if err := c.ShouldBindJSON(&word); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	configs.Cfg.QuickReplyWords = append(configs.Cfg.QuickReplyWords, word.Word)
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "快捷回复词创建成功",
		"data":    word,
	})
}

func (s *SystemConfigService) handleUpdateQuickReplyWord(c *gin.Context) {
	var word QuickReplyWord
	if err := c.ShouldBindJSON(&word); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	for i, dbWord := range configs.Cfg.QuickReplyWords {
		if dbWord == word.Word {
			configs.Cfg.QuickReplyWords[i] = word.Word
			break
		}
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "快捷回复词更新成功",
	})
}

func (s *SystemConfigService) handleDeleteQuickReplyWord(c *gin.Context) {
	word := c.Param("id")
	newWords := []string{}
	for _, dbWord := range configs.Cfg.QuickReplyWords {
		if dbWord != word {
			newWords = append(newWords, dbWord)
		}
	}
	configs.Cfg.QuickReplyWords = newWords
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "快捷回复词删除成功",
	})
}

// 本地MCP功能相关处理器
func (s *SystemConfigService) handleGetMCPFunctions(c *gin.Context) {
	var functions []LocalMCPFunction
	functions = make([]LocalMCPFunction, 0)
	for _, dbFunc := range configs.Cfg.LocalMCPFun {
		functions = append(functions, LocalMCPFunction{
			FunctionName: dbFunc.Name,
			Description:  dbFunc.Description,
			Enabled:      dbFunc.Enabled,
		})
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   functions,
	})
}

func (s *SystemConfigService) handleCreateMCPFunction(c *gin.Context) {
	var function LocalMCPFunction
	if err := c.ShouldBindJSON(&function); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}
	fmt.Println("创建MCP功能：", function)
	configs.Cfg.LocalMCPFun = append(configs.Cfg.LocalMCPFun, configs.LocalMCPFun{
		Name:        function.FunctionName,
		Description: function.Description,
		Enabled:     function.Enabled,
	})
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "MCP功能创建成功",
		"data":    function,
	})
}

func (s *SystemConfigService) handleUpdateMCPFunction(c *gin.Context) {
	id := c.Param("id")
	var function LocalMCPFunction
	if err := c.ShouldBindJSON(&function); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	for i, dbFunc := range configs.Cfg.LocalMCPFun {
		if dbFunc.Name == id {
			configs.Cfg.LocalMCPFun[i] = configs.LocalMCPFun{
				Name:        function.FunctionName,
				Description: function.Description,
				Enabled:     function.Enabled,
			}
		}
	}
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "MCP功能更新成功",
	})
}

func (s *SystemConfigService) handleDeleteMCPFunction(c *gin.Context) {
	id := c.Param("id")
	newFuncs := []configs.LocalMCPFun{}
	for _, dbFunc := range configs.Cfg.LocalMCPFun {
		if dbFunc.Name != id {
			newFuncs = append(newFuncs, dbFunc)
		}
	}
	configs.Cfg.LocalMCPFun = newFuncs
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "MCP功能删除成功",
	})
}

// 退出指令相关处理器
func (s *SystemConfigService) handleGetExitCommands(c *gin.Context) {
	var commands []ExitCommand
	commands = make([]ExitCommand, 0)
	for _, dbCmd := range configs.Cfg.CMDExit {
		commands = append(commands, ExitCommand{
			Command: dbCmd,
			Enabled: true,
		})
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   commands,
	})
}

func (s *SystemConfigService) handleCreateExitCommand(c *gin.Context) {
	var command ExitCommand
	if err := c.ShouldBindJSON(&command); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	configs.Cfg.CMDExit = append(configs.Cfg.CMDExit, command.Command)
	configs.Cfg.SaveToDB(database.GetServerConfigDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "退出指令创建成功",
		"data":    command,
	})
}

func (s *SystemConfigService) handleUpdateExitCommand(c *gin.Context) {
	id := c.Param("id")
	var command ExitCommand
	if err := c.ShouldBindJSON(&command); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}
	for i, dbCmd := range configs.Cfg.CMDExit {
		if dbCmd == id {
			configs.Cfg.CMDExit[i] = command.Command
			break
		}
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "退出指令更新成功",
	})
}

func (s *SystemConfigService) handleDeleteExitCommand(c *gin.Context) {
	id := c.Param("id")
	newCmds := []string{}
	for _, dbCmd := range configs.Cfg.CMDExit {
		if dbCmd != id {
			newCmds = append(newCmds, dbCmd)
		}
	}
	configs.Cfg.CMDExit = newCmds
	configs.Cfg.SaveToDB(database.GetServerConfigDB())
	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "退出指令删除成功",
	})
}
