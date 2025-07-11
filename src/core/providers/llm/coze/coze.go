package coze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/coze-dev/coze-go"
	"github.com/sashabaranov/go-openai"
	"io"
	"sync"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/types"
)

type Provider struct {
	*llm.BaseProvider

	botID                  string
	userID                 string
	accessToken            string
	clientId               string
	publicKey              string
	privateKey             string
	client                 coze.CozeAPI
	sessionConversationMap sync.Map
}

func init() {
	llm.Register("coze", NewProvider)
}

// NewProvider 创建Coze提供者
func NewProvider(config *llm.Config) (llm.Provider, error) {
	base := llm.NewBaseProvider(config)

	provider := &Provider{
		BaseProvider: base,
	}
	botId, ok := config.Extra["bot_id"]
	if ok {
		provider.botID = botId.(string)
	}
	userID, ok := config.Extra["user_id"]
	if ok {
		provider.userID = userID.(string)
	}
	clientId, ok := config.Extra["client_id"]
	if ok {
		provider.clientId = clientId.(string)
	}
	publicKey, ok := config.Extra["public_key"]
	if ok {
		provider.publicKey = publicKey.(string)
	}
	privateKey, ok := config.Extra["private_key"]
	if ok {
		provider.privateKey = privateKey.(string)
	}
	accessToken, ok := config.Extra["personal_access_token"]
	if ok {
		provider.accessToken = accessToken.(string)
	}
	return provider, nil
}

// Initialize 初始化提供者
func (p *Provider) Initialize() error {
	config := p.Config()
	baseURL := config.BaseURL
	if baseURL == "" {
		// 尝试从url字段获取
		if url, ok := config.Extra["url"].(string); ok {
			baseURL = url
		}
	}
	if baseURL == "" {
		return fmt.Errorf("缺少Coze基础URL配置")
	}

	var authCli coze.Auth
	if p.clientId != "" && p.publicKey != "" && p.privateKey != "" {
		// 正式环境
		client, err := coze.NewJWTOAuthClient(coze.NewJWTOAuthClientParam{
			ClientID:      p.clientId,
			PublicKey:     p.publicKey,
			PrivateKeyPEM: p.privateKey,
		}, coze.WithAuthBaseURL(baseURL))
		if err != nil {
			return fmt.Errorf("Coze创建JWT授权令牌失败: %v", err)
		}

		authCli = coze.NewJWTAuth(client, nil)
	} else {
		// 个人测试
		authCli = coze.NewTokenAuth(p.accessToken)
	}
	p.client = coze.NewCozeAPI(authCli, coze.WithBaseURL(baseURL))
	return nil
}

// Response types.LLMProvider接口实现
func (p *Provider) Response(ctx context.Context, sessionID string, messages []types.Message) (<-chan string, error) {
	responseChan := make(chan string, 10)

	go func() {
		defer close(responseChan)

		var lastMsg string
		if len(messages) > 0 {
			lastMsg = messages[len(messages)-1].Content
		}

		conversationId, ok := p.sessionConversationMap.Load(sessionID)
		if !ok {
			conversation, err := p.client.Conversations.Create(ctx, &coze.CreateConversationsReq{
				Messages: []*coze.Message{},
			})
			if err != nil {
				responseChan <- fmt.Sprintf("【Coze服务创建会话失败: %v】", err)
				return
			}
			conversationId = conversation.ID
			p.sessionConversationMap.Store(sessionID, conversationId)
		}

		stream, err := p.client.Chat.Stream(ctx, &coze.CreateChatsReq{
			BotID:  p.botID,
			UserID: p.userID,
			Messages: []*coze.Message{
				coze.BuildUserQuestionObjects([]*coze.MessageObjectString{
					coze.NewTextMessageObject(lastMsg),
				}, nil),
			},
			ConversationID: conversationId.(string),
		})
		if err != nil {
			responseChan <- fmt.Sprintf("【Coze服务响应异常: %v】", err)
			return
		}
		defer stream.Close()

		for {
			event, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					fmt.Println("Coze Stream finished")
				}
				break
			}

			if event.Event == coze.ChatEventConversationMessageDelta {
				responseChan <- event.Message.Content
			}
		}
	}()

	return responseChan, nil
}

// ResponseWithFunctions types.LLMProvider接口实现
func (p *Provider) ResponseWithFunctions(ctx context.Context, sessionID string, messages []types.Message, tools []openai.Tool) (<-chan types.Response, error) {
	responseChan := make(chan types.Response, 10)

	go func() {
		defer close(responseChan)

		// 第一次调用 LLM，取最后一条用户消息，附加 tool 提示词
		if len(messages) == 2 && len(tools) > 0 {
			lastMsg := messages[len(messages)-1].Content

			functionBytes, err := json.Marshal(tools)
			if err != nil {
				responseChan <- types.Response{
					Content: fmt.Sprintf("【序列化工具失败: %v】", err),
					Error:   err.Error(),
				}
				return
			}
			functionStr := string(functionBytes)
			modifyMsg := llm.GetSystemPromptForFunction(functionStr) + lastMsg
			messages[len(messages)-1].Content = modifyMsg
		}
		// 如果最后一个是 role="tool"，则附加到 user 消息中
		if len(messages) > 1 && messages[len(messages)-1].Role == "tool" {
			assistantMsg := "\ntool call result: " + messages[len(messages)-1].Content + "\n\n"

			for len(messages) > 1 {
				if messages[len(messages)-1].Role == "user" {
					messages[len(messages)-1].Content = assistantMsg + messages[len(messages)-1].Content
					break
				}
				messages = messages[:len(messages)-1]
			}
		}

		// 调用普通 Response 接口获取结果流
		respChan, err := p.Response(ctx, sessionID, messages)
		if err != nil {
			responseChan <- types.Response{
				Content: fmt.Sprintf("【调用Response失败: %v】", err),
				Error:   err.Error(),
			}
			return
		}

		// 透传结果
		for token := range respChan {
			responseChan <- types.Response{
				Content: token,
			}
		}
	}()

	return responseChan, nil
}
