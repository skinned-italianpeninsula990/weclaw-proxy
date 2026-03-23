package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// SmartRoutingConfig 智能路由配置
type SmartRoutingConfig struct {
	Enabled     bool    `yaml:"enabled" json:"enabled"`
	APIKey      string  `yaml:"api_key" json:"api_key"`
	BaseURL     string  `yaml:"base_url" json:"base_url"`
	Model       string  `yaml:"model" json:"model"`
	Temperature float64 `yaml:"temperature" json:"temperature"`
}

// AdapterInfo 适配器描述信息（用于 LLM 分类）
type AdapterInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SmartRouter LLM 驱动的智能路由器
type SmartRouter struct {
	apiKey      string
	baseURL     string
	model       string
	temperature float64
	httpClient  *http.Client
	logger      *slog.Logger
}

// NewSmartRouter 创建智能路由器
func NewSmartRouter(cfg *SmartRoutingConfig, logger *slog.Logger) *SmartRouter {
	if logger == nil {
		logger = slog.Default()
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	temperature := cfg.Temperature
	if temperature == 0 {
		temperature = 0.1
	}

	return &SmartRouter{
		apiKey:      cfg.APIKey,
		baseURL:     baseURL,
		model:       model,
		temperature: temperature,
		httpClient: &http.Client{
			Timeout: 8 * time.Second, // 智能路由总超时
		},
		logger: logger,
	}
}

// smartMessage OpenAI 消息格式（内部用）
type smartMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// smartRequest OpenAI ChatCompletion 请求（内部用）
type smartRequest struct {
	Model       string         `json:"model"`
	Messages    []smartMessage `json:"messages"`
	MaxTokens   int            `json:"max_tokens"`
	Temperature float64        `json:"temperature"`
}

// smartResponse OpenAI ChatCompletion 响应（内部用）
type smartResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Classify 调用 LLM 分析消息内容，返回应路由到的 adapter 名称
func (s *SmartRouter) Classify(ctx context.Context, message string, adapters []AdapterInfo) (string, error) {
	if len(adapters) == 0 {
		return "", fmt.Errorf("没有可用的适配器")
	}
	if len(adapters) == 1 {
		return adapters[0].Name, nil
	}

	// 构建适配器列表描述
	var adapterList strings.Builder
	for i, a := range adapters {
		adapterList.WriteString(fmt.Sprintf("%d. %s", i+1, a.Name))
		if a.Description != "" {
			adapterList.WriteString(fmt.Sprintf(" - %s", a.Description))
		}
		adapterList.WriteString("\n")
	}

	systemPrompt := fmt.Sprintf(`你是一个消息路由分类器。根据用户消息的内容和意图，判断应该将消息转发给哪个 AI Agent 处理。

可用的 Agent 列表：
%s
请直接返回最适合处理该消息的 Agent 名称（仅返回名称，不要附加任何解释）。
如果无法判断，返回第一个 Agent 的名称。`, adapterList.String())

	reqBody := &smartRequest{
		Model: s.model,
		Messages: []smartMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: message},
		},
		MaxTokens:   32, // 只需要返回一个名称
		Temperature: s.temperature,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("smart-router: 序列化请求失败: %w", err)
	}

	url := s.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("smart-router: 创建请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("smart-router: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("smart-router: 读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("smart-router: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var chatResp smartResponse
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return "", fmt.Errorf("smart-router: 解析响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("smart-router: 响应中没有 choices")
	}

	result := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	s.logger.Info("智能路由分类完成", "message", truncate(message, 50), "result", result)

	// 验证返回的名称是否在适配器列表中
	for _, a := range adapters {
		if strings.EqualFold(a.Name, result) {
			return a.Name, nil
		}
	}

	// 模糊匹配：LLM 可能返回带引号或额外内容
	for _, a := range adapters {
		if strings.Contains(strings.ToLower(result), strings.ToLower(a.Name)) {
			s.logger.Debug("智能路由模糊匹配", "result", result, "matched", a.Name)
			return a.Name, nil
		}
	}

	return "", fmt.Errorf("smart-router: LLM 返回了无效的适配器名称: %s", result)
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
