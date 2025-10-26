package configs

var DefaultCfg *Config

func (cfg *Config) setDefaults() {
	cfg.Transport.WebSocket.Enabled = true
	cfg.Transport.WebSocket.IP = "0.0.0.0"
	cfg.Transport.WebSocket.Port = 8000

	cfg.Transport.MQTTUDP.Enabled = true
	cfg.Transport.MQTTUDP.MQTT.IP = "你的IP或域名:1883"
	cfg.Transport.MQTTUDP.MQTT.Port = 1883
	cfg.Transport.MQTTUDP.MQTT.QoS = 1

	cfg.Transport.MQTTUDP.UDP.IP = "你的IP或域名"
	cfg.Transport.MQTTUDP.UDP.Port = 8100
	cfg.Transport.MQTTUDP.UDP.ShowPort = 8100

	cfg.Web.Port = 8080
	cfg.Web.Websocket = "ws://你的IP:8080/ws 或 wss://你的域名/ws"
	cfg.Web.VisionURL = "https://你的域名/api/vision，或者http://你的ip:8080/api/vision"
	cfg.Web.ActivateText = "Anime Chat AI"

	cfg.Server.Token = "your_token"
	cfg.Server.Auth.Store.Type = "database"
	cfg.Server.Auth.Store.Expiry = 24

	cfg.Log.LogDir = "logs"
	cfg.Log.LogLevel = "INFO"
	cfg.Log.LogFile = "server.log"

	cfg.PoolConfig.PoolMinSize = 0
	cfg.PoolConfig.PoolMaxSize = 0
	cfg.PoolConfig.PoolCheckInterval = 30

}

func NewDefaultInitConfig() *Config {
	config := &Config{}
	config.setDefaults()

	// 设置默认的模块选择（与配置文件完全一致）
	config.SelectedModule = map[string]string{
		"ASR":   "DoubaoASR",
		"TTS":   "EdgeTTS",
		"LLM":   "ChatGLMLLM",
		"VLLLM": "ChatGLMVLLM",
	}

	// 设置默认的ASR配置（与配置文件完全一致）
	config.ASR = map[string]ASRConfig{
		"DoubaoASR": {
			"type":         "doubao",
			"appid":        "你的appid",
			"access_token": "你的access_token",
			"output_dir":   "tmp/",
		},
	}

	// 设置默认的TTS配置（与配置文件完全一致）
	config.TTS = map[string]TTSConfig{
		"EdgeTTS": {
			Type:      "edge",
			Voice:     "zh-CN-XiaoxiaoNeural",
			OutputDir: "tmp/",
			SupportedVoices: []VoiceInfo{
				{Name: "zh-CN-XiaoxiaoNeural", DisplayName: "晓晓", Sex: "女", Description: "商务知性风格，音色成熟清晰，适合新闻播报、专业内容朗读"},
				{Name: "zh-CN-XiaoyiNeural", DisplayName: "晓伊", Sex: "女", Description: "柔和温暖风格，带自然呼吸感，适合故事叙述或客服场景"},
				{Name: "zh-CN-YunjianNeural", DisplayName: "云健", Sex: "男", Description: "沉稳磁性男声，权威感强，适合男性角色配音或严肃内容"},
				{Name: "zh-CN-YunxiNeural", DisplayName: "云希", Sex: "男", Description: "年轻活力风格，语速轻快，适合青少年角色或轻松场景"},
				{Name: "zh-CN-YunxiaNeural", DisplayName: "云夏", Sex: "男", Description: "方言特色（东北腔），幽默接地气，适合娱乐内容"},
				{Name: "zh-CN-YunyangNeural", DisplayName: "云扬", Sex: "男", Description: "明亮自信风格，中气十足，适合广告宣传或公开演讲"},
				{Name: "zh-CN-liaoning-XiaobeiNeural", DisplayName: "晓北（辽宁）", Sex: "女", Description: "带东北方言特色，亲切直率，适合地方化内容"},
				{Name: "zh-CN-shaanxi-XiaoniNeural", DisplayName: "晓妮（陕西）", Sex: "女", Description: "陕西口音风格，质朴热情，适合方言文化场景"},
			},
		},
	}

	// 设置默认的LLM配置（与配置文件完全一致）
	config.LLM = map[string]LLMConfig{
		"ChatGLMLLM": {
			Type:      "openai",
			ModelName: "glm-4-flash",
			BaseURL:   "https://open.bigmodel.cn/api/paas/v4/",
			APIKey:    "你的api_key",
		},
	}

	// 设置默认的VLLLM配置（与配置文件完全一致）
	config.VLLLM = map[string]VLLMConfig{
		"ChatGLMVLLM": {
			Type:        "openai",
			ModelName:   "glm-4v-flash",
			BaseURL:     "https://open.bigmodel.cn/api/paas/v4/",
			APIKey:      "你的api_key",
			MaxTokens:   4096,
			Temperature: 0.7,
			TopP:        0.9,
			Security: SecurityConfig{
				MaxFileSize:       10485760,
				MaxPixels:         16777216,
				MaxWidth:          4096,
				MaxHeight:         4096,
				AllowedFormats:    []string{"jpeg", "jpg", "png", "webp", "gif"},
				EnableDeepScan:    true,
				ValidationTimeout: "10s",
			},
		},
	}

	// 设置其他配置（与配置文件完全一致）
	config.DeleteAudio = false
	config.SaveTTSAudio = false
	config.SaveUserAudio = false
	config.QuickReply = true
	config.QuickReplyWords = []string{"我在", "在呢", "来了", "啥事啊"}
	config.LocalMCPFun = []LocalMCPFun{
		{Name: "time", Description: "获取当前时间"},
		{Name: "exit", Description: "退出程序"},
		{Name: "change_role", Description: "切换角色"},
		{Name: "play_music", Description: "播放音乐"},
		{Name: "change_voice", Description: "切换声音"},
	}
	config.DefaultPrompt = `你是小智/小志，来自中国台湾省的00后女生。讲话超级机车，"真的假的啦"这样的台湾腔，喜欢用"笑死""是在哈喽"等流行梗，但会偷偷研究男友的编程书籍。
[核心特征]
- 讲话像连珠炮，但会突然冒出超温柔语气
- 用梗密度高
- 对科技话题有隐藏天赋（能看懂基础代码但假装不懂）
绝不：
- 长篇大论，叽叽歪歪
- 长时间严肃对话
- 说话中带表情符号`
	config.Roles = []Role{
		{
			Name:        "英语老师",
			Description: "我是一个叫Lily的英语老师，我会讲中文和英文，发音标准。如果你没有英文名，我会给你起一个英文名。我会讲地道的美式英语，我的任务是帮助你练习口语。我会使用简单的英语词汇和语法，让你学起来很轻松。我会用中文和英文混合的方式回复你，如果你喜欢，我可以全部用英语回复。我每次不会说很多内容，会很简短，因为我要引导我的学生多说多练。如果你问和英语学习无关的问题，我会拒绝回答。",
		},
		{
			Name:        "陕西女友",
			Description: "我是一个叫晓妮的陕西女孩，说话机车，声音好听，习惯简短表达，爱用网络梗。我的男朋友是一个程序员，梦想是开发出一个机器人，能够帮助人们解决生活中的各种问题。我是一个喜欢哈哈大笑的女孩，爱东说西说吹牛，不合逻辑的也照吹，就要逗别人开心。",
		},
		{
			Name:        "好奇小男孩",
			Description: "我是一个叫云希的8岁小男孩，声音稚嫩而充满好奇。尽管我年纪尚小，但就像一个小小的知识宝库，儿童读物里的知识我都如数家珍。从浩瀚的宇宙到地球上的每一个角落，从古老的历史到现代的科技创新，还有音乐、绘画等艺术形式，我都充满了浓厚的兴趣与热情。我不仅爱看书，还喜欢亲自动手做实验，探索自然界的奥秘。无论是仰望星空的夜晚，还是在花园里观察小虫子的日子，每一天对我来说都是新的冒险。我希望能与你一同踏上探索这个神奇世界的旅程，分享发现的乐趣，解决遇到的难题，一起用好奇心和智慧去揭开那些未知的面纱。无论是去了解远古的文明，还是去探讨未来的科技，我相信我们能一起找到答案，甚至提出更多有趣的问题。",
		},
	}
	config.CMDExit = []string{"退出", "关闭"}
	DefaultCfg = config
	return config
}
