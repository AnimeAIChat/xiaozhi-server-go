# MCP使用方式

## ESP32 MCP
esp32的MCP，添加自定义MCP工具需要在前端修改，默认支持4个（self.get_device_status，self.audio_speaker.set_volume，self.screen.set_brightness，self.screen.set_theme）

服务无需修改配置，直接支持。

使用esp32客户端连接服务后，服务日志会打印以上4个工具的注册信息，直接对话，让小智调整音量，即可测试效果。

## 服务端外部MCP
服务端通过在源码根目录/二进制程序所在目录配置.mcp_server_settings.json文件，支持外部MCP调用，格式为
```
{
  "mcpServers": {
    "amap-maps": {
      "command": "npx",
      "args": [
          "-y",
          "@amap/amap-maps-mcp-server"
      ],
      "env": {
          "AMAP_MAPS_API_KEY": "你的高德api key"
      }
    },
    "filesystem": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "配置权限路径"
      ]
    },
     "playwright": {
      "command": "npx",
      "args": ["-y", "@executeautomation/playwright-mcp-server"],
      "des" : "run 'npx playwright install' first"
    },
    "windows-cli": {
      "command": "npx",
      "args": ["-y", "@simonb97/server-win-cli"]
    }
  }
}
```
服务端需要安装node才支持npx格式的MCP，其他格式的MCP请自行尝试

目前仅支持Stdio格式的MCP，如需使用SSE模式，可以考虑使用mcp-proxy方式，配置方式如下

```
{
  "mcpServers": {
    "zapier": {
      "command": "mcp-proxy",
      "args": [
        "https://actions.zapier.com/mcp/****/sse"
      ]
    }
  }
}
```

服务启动时会自动加载MCP配置，预生成MCP资源池，观察日志可以确认MCP是否加载成功
