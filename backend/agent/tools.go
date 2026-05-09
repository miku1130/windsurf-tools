package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ToolRegistry 工具注册表
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[ToolType]*Tool
}

// NewToolRegistry 创建工具注册表
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[ToolType]*Tool),
	}
}

// Register 注册工具
func (r *ToolRegistry) Register(tool *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = tool
}

// Get 获取工具
func (r *ToolRegistry) Get(name ToolType) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// Execute 执行工具
func (r *ToolRegistry) Execute(ctx context.Context, name ToolType, params map[string]interface{}) (*ToolResult, error) {
	start := time.Now()

	tool, ok := r.Get(name)
	if !ok {
		return &ToolResult{
			ToolName: name,
			Success:  false,
			Error:    fmt.Sprintf("工具未找到: %s", name),
			Duration: time.Since(start),
		}, nil
	}

	// 执行工具
	data, err := tool.Handler(ctx, params)
	if err != nil {
		return &ToolResult{
			ToolName: name,
			Success:  false,
			Error:    err.Error(),
			Duration: time.Since(start),
		}, nil
	}

	return &ToolResult{
		ToolName: name,
		Success:  true,
		Data:     data,
		Duration: time.Since(start),
	}, nil
}

// ── 内置工具实现 ──

// RateLimitCheckTool 限速检测工具
type RateLimitCheckTool struct {
	currentState RateLimitState
	mu           sync.RWMutex
}

// NewRateLimitCheckTool 创建限速检测工具
func NewRateLimitCheckTool() *RateLimitCheckTool {
	return &RateLimitCheckTool{}
}

func (t *RateLimitCheckTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.currentState, nil
}

func (t *RateLimitCheckTool) UpdateState(state RateLimitState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.currentState = state
}

// AccountSwitchTool 账号切换工具
type AccountSwitchTool struct {
	switchFn func(accountID string) error
}

// NewAccountSwitchTool 创建账号切换工具
func NewAccountSwitchTool(switchFn func(string) error) *AccountSwitchTool {
	return &AccountSwitchTool{
		switchFn: switchFn,
	}
}

func (t *AccountSwitchTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	accountID, ok := params["account_id"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少 account_id 参数")
	}

	if t.switchFn == nil {
		return nil, fmt.Errorf("切换函数未设置")
	}

	if err := t.switchFn(accountID); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"switched_to": accountID,
		"timestamp":   time.Now(),
	}, nil
}

// ModelSwitchTool 模型切换工具
type ModelSwitchTool struct {
	models       []ModelInfo
	currentModel string
	mu           sync.RWMutex
}

// ModelInfo 模型信息
type ModelInfo struct {
	Name          string  `json:"name"`
	DisplayName   string  `json:"display_name"`
	IsAvailable   bool    `json:"is_available"`
	IsRateLimited bool    `json:"is_rate_limited"`
	CostPerToken  float64 `json:"cost_per_token"`
	MaxTokens     int     `json:"max_tokens"`
}

// NewModelSwitchTool 创建模型切换工具
func NewModelSwitchTool() *ModelSwitchTool {
	return &ModelSwitchTool{
		models: []ModelInfo{
			{Name: "claude-sonnet-4-20250514", DisplayName: "Claude Sonnet 4", IsAvailable: true},
			{Name: "claude-opus-4-20250514", DisplayName: "Claude Opus 4", IsAvailable: true},
			{Name: "gpt-4o", DisplayName: "GPT-4o", IsAvailable: true},
			{Name: "gpt-4o-mini", DisplayName: "GPT-4o Mini", IsAvailable: true},
			{Name: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", IsAvailable: true},
		},
	}
}

func (t *ModelSwitchTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 如果指定了模型，切换到指定模型
	if targetModel, ok := params["model"].(string); ok {
		for _, m := range t.models {
			if m.Name == targetModel && m.IsAvailable && !m.IsRateLimited {
				t.currentModel = targetModel
				return map[string]interface{}{
					"model":      targetModel,
					"switched":   true,
					"timestamp":  time.Now(),
				}, nil
			}
		}
		return nil, fmt.Errorf("模型不可用: %s", targetModel)
	}

	// 否则选择最佳可用模型
	for _, m := range t.models {
		if m.IsAvailable && !m.IsRateLimited {
			t.currentModel = m.Name
			return map[string]interface{}{
				"model":      m.Name,
				"switched":   true,
				"timestamp":  time.Now(),
			}, nil
		}
	}

	return nil, fmt.Errorf("没有可用的模型")
}

func (t *ModelSwitchTool) UpdateModelStatus(modelName string, isRateLimited bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, m := range t.models {
		if m.Name == modelName {
			t.models[i].IsRateLimited = isRateLimited
			break
		}
	}
}

// FingerprintRotateTool 设备指纹轮换工具
type FingerprintRotateTool struct {
	rotateFn func() error
}

// NewFingerprintRotateTool 创建设备指纹轮换工具
func NewFingerprintRotateTool(rotateFn func() error) *FingerprintRotateTool {
	return &FingerprintRotateTool{
		rotateFn: rotateFn,
	}
}

func (t *FingerprintRotateTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if t.rotateFn == nil {
		return nil, fmt.Errorf("轮换函数未设置")
	}

	if err := t.rotateFn(); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"rotated":   true,
		"timestamp": time.Now(),
	}, nil
}

// QuotaQueryTool 额度查询工具
type QuotaQueryTool struct {
	queryFn func(accountID string) (*QuotaInfo, error)
}

// QuotaInfo 额度信息
type QuotaInfo struct {
	AccountID       string  `json:"account_id"`
	DailyRemaining  float64 `json:"daily_remaining"`
	WeeklyRemaining float64 `json:"weekly_remaining"`
	TotalQuota      int     `json:"total_quota"`
	UsedQuota       int     `json:"used_quota"`
	ResetAt         string  `json:"reset_at"`
}

// NewQuotaQueryTool 创建额度查询工具
func NewQuotaQueryTool(queryFn func(string) (*QuotaInfo, error)) *QuotaQueryTool {
	return &QuotaQueryTool{
		queryFn: queryFn,
	}
}

func (t *QuotaQueryTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	accountID, ok := params["account_id"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少 account_id 参数")
	}

	if t.queryFn == nil {
		return nil, fmt.Errorf("查询函数未设置")
	}

	return t.queryFn(accountID)
}

// RequestForwardTool 请求转发工具
type RequestForwardTool struct {
	forwardFn func(req *RequestData) (*ResponseData, error)
}

// RequestData 请求数据
type RequestData struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
	Model   string            `json:"model"`
}

// ResponseData 响应数据
type ResponseData struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	Duration   time.Duration     `json:"duration"`
}

// NewRequestForwardTool 创建请求转发工具
func NewRequestForwardTool(forwardFn func(*RequestData) (*ResponseData, error)) *RequestForwardTool {
	return &RequestForwardTool{
		forwardFn: forwardFn,
	}
}

func (t *RequestForwardTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if t.forwardFn == nil {
		return nil, fmt.Errorf("转发函数未设置")
	}

	// 从参数中构建请求数据
	reqData := &RequestData{
		Method:  getStringParam(params, "method", "POST"),
		Path:    getStringParam(params, "path", ""),
		Headers: getMapParam(params, "headers"),
	}

	return t.forwardFn(reqData)
}

// CacheManagerTool 缓存管理工具
type CacheManagerTool struct {
	cache sync.Map
}

// NewCacheManagerTool 创建缓存管理工具
func NewCacheManagerTool() *CacheManagerTool {
	return &CacheManagerTool{}
}

func (t *CacheManagerTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action := getStringParam(params, "action", "get")
	key := getStringParam(params, "key", "")

	switch action {
	case "get":
		if value, ok := t.cache.Load(key); ok {
			return value, nil
		}
		return nil, nil

	case "set":
		value := params["value"]
		t.cache.Store(key, value)
		return map[string]interface{}{"stored": true}, nil

	case "delete":
		t.cache.Delete(key)
		return map[string]interface{}{"deleted": true}, nil

	default:
		return nil, fmt.Errorf("未知操作: %s", action)
	}
}

// ── 辅助函数 ──

func getStringParam(params map[string]interface{}, key, defaultValue string) string {
	if v, ok := params[key].(string); ok {
		return v
	}
	return defaultValue
}

func getMapParam(params map[string]interface{}, key string) map[string]string {
	if v, ok := params[key].(map[string]string); ok {
		return v
	}
	return make(map[string]string)
}
