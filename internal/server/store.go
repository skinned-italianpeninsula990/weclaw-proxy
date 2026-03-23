package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/amigoer/weclaw-proxy/internal/adapter"
	"github.com/amigoer/weclaw-proxy/internal/router"
	"gopkg.in/yaml.v3"
)

// RuntimeConfig 运行时配置（支持热更新）
type RuntimeConfig struct {
	Adapters       []adapter.AdapterConfig  `json:"adapters" yaml:"adapters"`
	Routing        router.RouterConfig      `json:"routing" yaml:"routing"`
	SmartRouting   router.SmartRoutingConfig `json:"smart_routing" yaml:"smart_routing"`
	HistoryLimit   int                      `json:"history_limit" yaml:"-"`
	TimeoutMinutes int                      `json:"timeout_minutes" yaml:"-"`
}

// Store 运行时配置存储
type Store struct {
	config         RuntimeConfig
	filePath       string // runtime.json 路径
	configFilePath string // config.yaml 路径（用于回写）
	mu             sync.RWMutex
	logger         *slog.Logger

	// 配置变更回调
	onUpdate func(cfg *RuntimeConfig)
}

// NewStore 创建配置存储
func NewStore(filePath string, logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	return &Store{
		filePath: filePath,
		logger:   logger,
		config: RuntimeConfig{
			Adapters:       make([]adapter.AdapterConfig, 0),
			HistoryLimit:   20,
			TimeoutMinutes: 30,
		},
	}
}

// SetOnUpdate 设置配置变更回调
func (s *Store) SetOnUpdate(fn func(cfg *RuntimeConfig)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onUpdate = fn
}

// Load 从文件加载配置
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在则使用默认配置
		}
		return err
	}

	return json.Unmarshal(data, &s.config)
}

// Save 保存配置到 runtime.json
func (s *Store) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.config, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0600)
}

// SetConfigFilePath 设置 YAML 配置文件路径（用于回写）
func (s *Store) SetConfigFilePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configFilePath = path
}

// yamlConfig 用于 YAML 序列化的完整配置结构
type yamlConfig struct {
	Server       serverYAML                `yaml:"server"`
	Weixin       weixinYAML                `yaml:"weixin"`
	Adapters     []adapter.AdapterConfig   `yaml:"adapters"`
	Routing      router.RouterConfig       `yaml:"routing"`
	SmartRouting router.SmartRoutingConfig  `yaml:"smart_routing"`
	Session      sessionYAML               `yaml:"session"`
}
type serverYAML struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host,omitempty"`
}
type weixinYAML struct {
	BaseURL           string `yaml:"base_url"`
	CDNBaseURL        string `yaml:"cdn_base_url"`
	LongPollTimeoutMs int    `yaml:"long_poll_timeout_ms"`
	DataDir           string `yaml:"data_dir"`
}
type sessionYAML struct {
	HistoryLimit   int `yaml:"history_limit"`
	TimeoutMinutes int `yaml:"timeout_minutes"`
}

// SaveToYAML 将配置回写到 YAML 文件
func (s *Store) SaveToYAML() error {
	s.mu.RLock()
	cfgPath := s.configFilePath
	currentCfg := s.config
	s.mu.RUnlock()

	if cfgPath == "" {
		return fmt.Errorf("未设置 YAML 配置文件路径")
	}

	// 读取现有 YAML 保留 server/weixin/session 的配置
	var existing yamlConfig
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	// 只更新可编辑的部分
	existing.Adapters = currentCfg.Adapters
	existing.Routing = currentCfg.Routing
	existing.SmartRouting = currentCfg.SmartRouting

	data, err := yaml.Marshal(&existing)
	if err != nil {
		return fmt.Errorf("序列化 YAML 失败: %w", err)
	}

	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		return fmt.Errorf("写入 YAML 文件失败: %w", err)
	}

	s.logger.Info("配置已同步到 YAML 文件", "path", cfgPath)
	return nil
}

// GetConfig 获取当前配置副本
func (s *Store) GetConfig() RuntimeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 深拷贝 adapters
	adapters := make([]adapter.AdapterConfig, len(s.config.Adapters))
	copy(adapters, s.config.Adapters)

	rules := make([]router.RouteRule, len(s.config.Routing.Rules))
	copy(rules, s.config.Routing.Rules)

	return RuntimeConfig{
		Adapters: adapters,
		Routing: router.RouterConfig{
			DefaultAdapter: s.config.Routing.DefaultAdapter,
			Rules:          rules,
		},
		HistoryLimit:   s.config.HistoryLimit,
		TimeoutMinutes: s.config.TimeoutMinutes,
	}
}

// SetConfig 设置完整配置
func (s *Store) SetConfig(cfg RuntimeConfig) {
	s.mu.Lock()
	s.config = cfg
	fn := s.onUpdate
	s.mu.Unlock()

	if fn != nil {
		fn(&cfg)
	}
}

// --- Adapter CRUD ---

// ListAdapters 列出所有适配器
func (s *Store) ListAdapters() []adapter.AdapterConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]adapter.AdapterConfig, len(s.config.Adapters))
	copy(result, s.config.Adapters)
	return result
}

// GetAdapter 获取指定适配器
func (s *Store) GetAdapter(name string) *adapter.AdapterConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.config.Adapters {
		if s.config.Adapters[i].Name == name {
			cfg := s.config.Adapters[i]
			return &cfg
		}
	}
	return nil
}

// AddAdapter 添加适配器
func (s *Store) AddAdapter(cfg adapter.AdapterConfig) {
	s.mu.Lock()
	s.config.Adapters = append(s.config.Adapters, cfg)
	fn := s.onUpdate
	configCopy := s.config
	s.mu.Unlock()

	if fn != nil {
		fn(&configCopy)
	}
}

// UpdateAdapter 更新适配器
func (s *Store) UpdateAdapter(name string, cfg adapter.AdapterConfig) bool {
	s.mu.Lock()
	found := false
	for i := range s.config.Adapters {
		if s.config.Adapters[i].Name == name {
			s.config.Adapters[i] = cfg
			found = true
			break
		}
	}
	fn := s.onUpdate
	configCopy := s.config
	s.mu.Unlock()

	if found && fn != nil {
		fn(&configCopy)
	}
	return found
}

// DeleteAdapter 删除适配器
func (s *Store) DeleteAdapter(name string) bool {
	s.mu.Lock()
	found := false
	for i := range s.config.Adapters {
		if s.config.Adapters[i].Name == name {
			s.config.Adapters = append(s.config.Adapters[:i], s.config.Adapters[i+1:]...)
			found = true
			break
		}
	}
	fn := s.onUpdate
	configCopy := s.config
	s.mu.Unlock()

	if found && fn != nil {
		fn(&configCopy)
	}
	return found
}

// --- Routing ---

// GetRouting 获取路由配置
func (s *Store) GetRouting() router.RouterConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := make([]router.RouteRule, len(s.config.Routing.Rules))
	copy(rules, s.config.Routing.Rules)
	return router.RouterConfig{
		DefaultAdapter: s.config.Routing.DefaultAdapter,
		Rules:          rules,
	}
}

// SetRouting 更新路由配置
func (s *Store) SetRouting(routing router.RouterConfig) {
	s.mu.Lock()
	s.config.Routing = routing
	fn := s.onUpdate
	configCopy := s.config
	s.mu.Unlock()

	if fn != nil {
		fn(&configCopy)
	}
}

// --- SmartRouting ---

// GetSmartRouting 获取智能路由配置
func (s *Store) GetSmartRouting() router.SmartRoutingConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.SmartRouting
}

// SetSmartRouting 更新智能路由配置
func (s *Store) SetSmartRouting(cfg router.SmartRoutingConfig) {
	s.mu.Lock()
	s.config.SmartRouting = cfg
	fn := s.onUpdate
	configCopy := s.config
	s.mu.Unlock()

	if fn != nil {
		fn(&configCopy)
	}
}
