package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// Gemini 原生 API 适配器
// 支持 Google Gemini API（AI Studio / Vertex AI），包含 google_search 搜索工具

// geminiPart Gemini 消息块
type geminiPart struct {
	Text string `json:"text,omitempty"`
}

// geminiContent Gemini 消息内容
type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

// geminiRequest Gemini generateContent 请求
type geminiRequest struct {
	Contents          []geminiContent    `json:"contents"`
	SystemInstruction *geminiContent     `json:"systemInstruction,omitempty"`
	Tools             []geminiTool       `json:"tools,omitempty"`
	GenerationConfig  *geminiGenConfig   `json:"generationConfig,omitempty"`
}

// geminiTool 工具定义
type geminiTool struct {
	GoogleSearch *struct{} `json:"google_search,omitempty"`
}

// geminiGenConfig 生成配置
type geminiGenConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

// geminiResponse Gemini 响应
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason     string `json:"finishReason"`
		GroundingMetadata *struct {
			SearchEntryPoint *struct {
				RenderedContent string `json:"renderedContent"`
			} `json:"searchEntryPoint"`
			GroundingChunks []struct {
				Web *struct {
					URI   string `json:"uri"`
					Title string `json:"title"`
				} `json:"web"`
			} `json:"groundingChunks"`
		} `json:"groundingMetadata,omitempty"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// GeminiAdapter Gemini 原生适配器
type GeminiAdapter struct {
	name           string
	baseURL        string
	apiKey         string
	model          string
	systemPrompt   string
	maxTokens      int
	temperature    float64
	enableSearch   bool
	httpClient     *http.Client
	logger         *slog.Logger
}

// NewGeminiAdapter 创建 Gemini 适配器
func NewGeminiAdapter(cfg *AdapterConfig, logger *slog.Logger) *GeminiAdapter {
	if logger == nil {
		logger = slog.Default()
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	model := cfg.Model
	if model == "" {
		model = "gemini-2.5-flash-preview-04-17"
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	temperature := cfg.Temperature
	if temperature == 0 {
		temperature = 0.7
	}

	enableSearch := cfg.Extra["enable_search"] == "true"

	return &GeminiAdapter{
		name:         cfg.Name,
		baseURL:      baseURL,
		apiKey:       cfg.APIKey,
		model:        model,
		systemPrompt: cfg.SystemPrompt,
		maxTokens:    maxTokens,
		temperature:  temperature,
		enableSearch: enableSearch,
		httpClient:   &http.Client{},
		logger:       logger,
	}
}

func (a *GeminiAdapter) Name() string { return a.name }
func (a *GeminiAdapter) Type() string { return "gemini" }

// buildRequest 构建 Gemini 请求
func (a *GeminiAdapter) buildRequest(req *ChatRequest) *geminiRequest {
	gemReq := &geminiRequest{
		GenerationConfig: &geminiGenConfig{
			MaxOutputTokens: a.maxTokens,
			Temperature:     a.temperature,
		},
	}

	// 系统提示词
	if a.systemPrompt != "" {
		gemReq.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: a.systemPrompt}},
		}
	}

	// 历史消息
	for _, h := range req.History {
		role := h.Role
		if role == "assistant" {
			role = "model" // Gemini 使用 "model" 而非 "assistant"
		}
		gemReq.Contents = append(gemReq.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: h.Content}},
		})
	}

	// 当前用户消息
	gemReq.Contents = append(gemReq.Contents, geminiContent{
		Role:  "user",
		Parts: []geminiPart{{Text: req.Message}},
	})

	// 搜索工具
	if a.enableSearch {
		gemReq.Tools = []geminiTool{{GoogleSearch: &struct{}{}}}
	}

	return gemReq
}

// buildURL 构建 API URL
func (a *GeminiAdapter) buildURL(stream bool) string {
	action := "generateContent"
	if stream {
		action = "streamGenerateContent?alt=sse"
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:%s", a.baseURL, a.model, action)
	if a.apiKey != "" {
		sep := "?"
		if stream {
			sep = "&"
		}
		url += sep + "key=" + a.apiKey
	}
	return url
}

// Chat 同步对话
func (a *GeminiAdapter) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	gemReq := a.buildRequest(req)

	body, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: 序列化请求失败: %w", err)
	}

	url := a.buildURL(false)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: 创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: 读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(rawBody, &gemResp); err != nil {
		return nil, fmt.Errorf("gemini: 解析响应失败: %w", err)
	}

	if gemResp.Error != nil {
		return nil, fmt.Errorf("gemini: API 错误 (%d): %s", gemResp.Error.Code, gemResp.Error.Message)
	}

	if len(gemResp.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: 响应中没有 candidates")
	}

	// 拼接所有 parts 的文本
	var text strings.Builder
	for _, part := range gemResp.Candidates[0].Content.Parts {
		text.WriteString(part.Text)
	}

	// 附加搜索来源（如果有）
	result := text.String()
	if gm := gemResp.Candidates[0].GroundingMetadata; gm != nil && len(gm.GroundingChunks) > 0 {
		result += "\n\n📎 参考来源：\n"
		for _, chunk := range gm.GroundingChunks {
			if chunk.Web != nil {
				result += fmt.Sprintf("- [%s](%s)\n", chunk.Web.Title, chunk.Web.URI)
			}
		}
	}

	return &ChatResponse{
		Text: result,
	}, nil
}

// ChatStream 流式对话
func (a *GeminiAdapter) ChatStream(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error) {
	gemReq := a.buildRequest(req)

	body, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: 序列化请求失败: %w", err)
	}

	url := a.buildURL(true)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: 创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: 请求失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gemini: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	ch := make(chan *ChatChunk, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "" {
				continue
			}

			var gemResp geminiResponse
			if err := json.Unmarshal([]byte(data), &gemResp); err != nil {
				a.logger.Warn("gemini: 解析 SSE 数据失败",
					"data", data,
					"error", err,
				)
				continue
			}

			if len(gemResp.Candidates) > 0 {
				for _, part := range gemResp.Candidates[0].Content.Parts {
					if part.Text != "" {
						ch <- &ChatChunk{Text: part.Text}
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- &ChatChunk{Error: err}
		}

		ch <- &ChatChunk{Done: true}
	}()

	return ch, nil
}
