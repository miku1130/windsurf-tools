package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Agent 基础 Agent 接口
type Agent interface {
	// GetType 获取 Agent 类型
	GetType() AgentType
	// GetStatus 获取 Agent 状态
	GetStatus() AgentStatus
	// Start 启动 Agent
	Start(ctx context.Context) error
	// Stop 停止 Agent
	Stop() error
	// HandleTask 处理任务
	HandleTask(ctx context.Context, task *Task) (*TaskResult, error)
	// HandleMessage 处理消息
	HandleMessage(msg *Message) error
	// GetState 获取 Agent 状态
	GetState() AgentState
}

// BaseAgent Agent 基础实现
type BaseAgent struct {
	mu          sync.RWMutex
	agentType   AgentType
	status      AgentStatus
	currentTask *Task
	taskCount   int
	errorCount  int
	lastActive  time.Time
	messageCh   chan *Message
	stopCh      chan struct{}
	logFn       func(string, ...interface{})
}

// NewBaseAgent 创建基础 Agent
func NewBaseAgent(agentType AgentType) *BaseAgent {
	return &BaseAgent{
		agentType:  agentType,
		status:     AgentStatusIdle,
		messageCh:  make(chan *Message, 100),
		stopCh:     make(chan struct{}),
		lastActive: time.Now(),
		logFn: func(format string, args ...interface{}) {
			log.Printf("[Agent:%s] %s", agentType, fmt.Sprintf(format, args...))
		},
	}
}

func (a *BaseAgent) GetType() AgentType {
	return a.agentType
}

func (a *BaseAgent) GetStatus() AgentStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

func (a *BaseAgent) GetState() AgentState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return AgentState{
		Type:         a.agentType,
		Status:       a.status,
		CurrentTask:  a.currentTask,
		TasksHandled: a.taskCount,
		LastActive:   a.lastActive,
		ErrorCount:   a.errorCount,
	}
}

func (a *BaseAgent) setStatus(status AgentStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = status
	a.lastActive = time.Now()
}

func (a *BaseAgent) log(format string, args ...interface{}) {
	if a.logFn != nil {
		a.logFn(format, args...)
	}
}

func (a *BaseAgent) Start(ctx context.Context) error {
	a.setStatus(AgentStatusRunning)
	a.log("Agent 启动")
	go a.messageLoop(ctx)
	return nil
}

func (a *BaseAgent) Stop() error {
	a.setStatus(AgentStatusStopped)
	close(a.stopCh)
	a.log("Agent 停止")
	return nil
}

func (a *BaseAgent) HandleMessage(msg *Message) error {
	select {
	case a.messageCh <- msg:
		return nil
	default:
		return fmt.Errorf("消息队列已满")
	}
}

func (a *BaseAgent) messageLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case msg := <-a.messageCh:
			if err := a.processMessage(msg); err != nil {
				a.log("处理消息失败: %v", err)
				a.mu.Lock()
				a.errorCount++
				a.mu.Unlock()
			}
		}
	}
}

func (a *BaseAgent) processMessage(msg *Message) error {
	// 基础实现，子类可以覆盖
	a.log("收到消息: type=%s from=%s", msg.Type, msg.From)
	return nil
}

// ── 具体 Agent 实现 ──

// RateLimitMonitorAgent 限速监控 Agent
type RateLimitMonitorAgent struct {
	*BaseAgent
	rateLimitState RateLimitState
	tools          *ToolRegistry
	mu             sync.RWMutex
}

// NewRateLimitMonitorAgent 创建限速监控 Agent
func NewRateLimitMonitorAgent(tools *ToolRegistry) *RateLimitMonitorAgent {
	return &RateLimitMonitorAgent{
		BaseAgent: NewBaseAgent(AgentTypeRateLimiter),
		tools:     tools,
	}
}

func (a *RateLimitMonitorAgent) HandleTask(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	a.mu.Lock()
	a.currentTask = task
	a.taskCount++
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.currentTask = nil
		a.mu.Unlock()
	}()

	switch task.Type {
	case TaskTypeCheckRateLimit:
		return a.checkRateLimit(ctx, task)
	default:
		return &TaskResult{
			TaskID:    task.ID,
			Success:   false,
			Error:     fmt.Sprintf("未知任务类型: %s", task.Type),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}, nil
	}
}

func (a *RateLimitMonitorAgent) checkRateLimit(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	a.log("检查限速状态...")

	// 使用工具检查限速
	result, err := a.tools.Execute(ctx, ToolTypeRateLimitCheck, map[string]interface{}{})
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Duration: time.Since(start),
		}, nil
	}

	// 更新状态
	a.mu.Lock()
	if state, ok := result.Data.(RateLimitState); ok {
		a.rateLimitState = state
	}
	a.mu.Unlock()

	return &TaskResult{
		TaskID:    task.ID,
		Success:   true,
		Data:      result.Data,
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}, nil
}

func (a *RateLimitMonitorAgent) GetRateLimitState() RateLimitState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.rateLimitState
}

// AccountManagerAgent 账号管理 Agent
type AccountManagerAgent struct {
	*BaseAgent
	accounts []AccountState
	tools    *ToolRegistry
	mu       sync.RWMutex
}

// NewAccountManagerAgent 创建账号管理 Agent
func NewAccountManagerAgent(tools *ToolRegistry) *AccountManagerAgent {
	return &AccountManagerAgent{
		BaseAgent: NewBaseAgent(AgentTypeAccountManager),
		tools:     tools,
	}
}

func (a *AccountManagerAgent) HandleTask(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	a.mu.Lock()
	a.currentTask = task
	a.taskCount++
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.currentTask = nil
		a.mu.Unlock()
	}()

	switch task.Type {
	case TaskTypeRotateAccount:
		return a.rotateAccount(ctx, task)
	case TaskTypeRefreshQuota:
		return a.refreshQuota(ctx, task)
	case TaskTypeHealthCheck:
		return a.healthCheck(ctx, task)
	default:
		return &TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    fmt.Sprintf("未知任务类型: %s", task.Type),
			Duration: time.Since(start),
		}, nil
	}
}

func (a *AccountManagerAgent) rotateAccount(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	a.log("轮换账号...")

	result, err := a.tools.Execute(ctx, ToolTypeAccountSwitch, task.Payload.(map[string]interface{}))
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Duration: time.Since(start),
		}, nil
	}

	return &TaskResult{
		TaskID:    task.ID,
		Success:   true,
		Data:      result.Data,
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}, nil
}

func (a *AccountManagerAgent) refreshQuota(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	a.log("刷新额度...")

	result, err := a.tools.Execute(ctx, ToolTypeQuotaQuery, task.Payload.(map[string]interface{}))
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Duration: time.Since(start),
		}, nil
	}

	return &TaskResult{
		TaskID:    task.ID,
		Success:   true,
		Data:      result.Data,
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}, nil
}

func (a *AccountManagerAgent) healthCheck(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	a.log("健康检查...")

	// 检查所有账号状态
	a.mu.RLock()
	accounts := a.accounts
	a.mu.RUnlock()

	healthyCount := 0
	for _, acc := range accounts {
		if acc.IsHealthy && !acc.IsRateLimited {
			healthyCount++
		}
	}

	return &TaskResult{
		TaskID:  task.ID,
		Success: true,
		Data: map[string]interface{}{
			"total":   len(accounts),
			"healthy": healthyCount,
		},
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}, nil
}

// ModelSelectorAgent 模型选择 Agent
type ModelSelectorAgent struct {
	*BaseAgent
	currentModel string
	modelStates  map[string]ModelState
	tools        *ToolRegistry
	mu           sync.RWMutex
}

// ModelState 模型状态
type ModelState struct {
	Name          string    `json:"name"`
	IsAvailable   bool      `json:"is_available"`
	IsRateLimited bool      `json:"is_rate_limited"`
	LastUsed      time.Time `json:"last_used"`
	SuccessRate   float64   `json:"success_rate"`
}

// NewModelSelectorAgent 创建模型选择 Agent
func NewModelSelectorAgent(tools *ToolRegistry) *ModelSelectorAgent {
	return &ModelSelectorAgent{
		BaseAgent:   NewBaseAgent(AgentTypeModelSelector),
		tools:       tools,
		modelStates: make(map[string]ModelState),
	}
}

func (a *ModelSelectorAgent) HandleTask(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	a.mu.Lock()
	a.currentTask = task
	a.taskCount++
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.currentTask = nil
		a.mu.Unlock()
	}()

	switch task.Type {
	case TaskTypeSelectModel:
		return a.selectModel(ctx, task)
	default:
		return &TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    fmt.Sprintf("未知任务类型: %s", task.Type),
			Duration: time.Since(start),
		}, nil
	}
}

func (a *ModelSelectorAgent) selectModel(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	a.log("选择模型...")

	result, err := a.tools.Execute(ctx, ToolTypeModelSwitch, task.Payload.(map[string]interface{}))
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			Success:  false,
			Error:    err.Error(),
			Duration: time.Since(start),
		}, nil
	}

	return &TaskResult{
		TaskID:    task.ID,
		Success:   true,
		Data:      result.Data,
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}, nil
}

func (a *ModelSelectorAgent) UpdateModelState(name string, state ModelState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.modelStates[name] = state
}

func (a *ModelSelectorAgent) GetBestModel() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var bestModel string
	var bestScore float64 = -1

	for name, state := range a.modelStates {
		if !state.IsAvailable || state.IsRateLimited {
			continue
		}
		score := state.SuccessRate
		if score > bestScore {
			bestScore = score
			bestModel = name
		}
	}

	return bestModel
}
