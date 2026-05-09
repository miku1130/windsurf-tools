package agent

import (
	"context"
	"time"
)

// ── Agent 类型定义 ──

// AgentType Agent 类型
type AgentType string

const (
	AgentTypeOrchestrator   AgentType = "orchestrator"   // 主控 Agent
	AgentTypeRateLimiter    AgentType = "rate_limiter"   // 限速监控 Agent
	AgentTypeAccountManager AgentType = "account_manager" // 账号管理 Agent
	AgentTypeRequestRouter  AgentType = "request_router"  // 请求路由 Agent
	AgentTypeModelSelector  AgentType = "model_selector"  // 模型选择 Agent
)

// AgentStatus Agent 状态
type AgentStatus string

const (
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusRunning  AgentStatus = "running"
	AgentStatusWaiting  AgentStatus = "waiting"
	AgentStatusError    AgentStatus = "error"
	AgentStatusStopped  AgentStatus = "stopped"
)

// ── 消息类型 ──

// MessageType 消息类型
type MessageType string

const (
	MessageTypeTask      MessageType = "task"       // 任务消息
	MessageTypeResult    MessageType = "result"     // 结果消息
	MessageTypeEvent     MessageType = "event"      // 事件消息
	MessageTypeCommand   MessageType = "command"    // 命令消息
	MessageTypeBroadcast MessageType = "broadcast"  // 广播消息
)

// Message Agent 间通信消息
type Message struct {
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	From      AgentType   `json:"from"`
	To        AgentType   `json:"to"`        // 空表示广播
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
	ReplyTo   string      `json:"reply_to"`  // 回复目标消息ID
}

// ── 任务类型 ──

// TaskType 任务类型
type TaskType string

const (
	TaskTypeCheckRateLimit    TaskType = "check_rate_limit"    // 检查限速
	TaskTypeRotateAccount     TaskType = "rotate_account"      // 轮换账号
	TaskTypeSelectModel       TaskType = "select_model"        // 选择模型
	TaskTypeRouteRequest      TaskType = "route_request"       // 路由请求
	TaskTypeRotateFingerprint TaskType = "rotate_fingerprint"  // 轮换设备指纹
	TaskTypeRefreshQuota      TaskType = "refresh_quota"       // 刷新额度
	TaskTypeHealthCheck       TaskType = "health_check"        // 健康检查
)

// Task 任务定义
type Task struct {
	ID       string      `json:"id"`
	Type     TaskType    `json:"type"`
	Priority int         `json:"priority"` // 优先级，数字越小优先级越高
	Payload  interface{} `json:"payload"`
	Created  time.Time   `json:"created"`
	Timeout  time.Duration `json:"timeout"`
}

// TaskResult 任务结果
type TaskResult struct {
	TaskID    string      `json:"task_id"`
	Success   bool        `json:"success"`
	Data      interface{} `json:"data"`
	Error     string      `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time   `json:"timestamp"`
}

// ── 工具类型 ──

// ToolType 工具类型
type ToolType string

const (
	ToolTypeRateLimitCheck    ToolType = "rate_limit_check"     // 限速检测
	ToolTypeAccountSwitch     ToolType = "account_switch"       // 账号切换
	ToolTypeModelSwitch       ToolType = "model_switch"         // 模型切换
	ToolTypeFingerprintRotate ToolType = "fingerprint_rotate"   // 指纹轮换
	ToolTypeQuotaQuery        ToolType = "quota_query"          // 额度查询
	ToolTypeRequestForward    ToolType = "request_forward"      // 请求转发
	ToolTypeCacheManager      ToolType = "cache_manager"        // 缓存管理
)

// Tool 工具定义
type Tool struct {
	Name        ToolType    `json:"name"`
	Description string      `json:"description"`
	Parameters  []ToolParam `json:"parameters"`
	Handler     ToolHandler `json:"-"`
}

// ToolParam 工具参数
type ToolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolHandler 工具处理函数
type ToolHandler func(ctx context.Context, params map[string]interface{}) (interface{}, error)

// ToolResult 工具执行结果
type ToolResult struct {
	ToolName  ToolType    `json:"tool_name"`
	Success   bool        `json:"success"`
	Data      interface{} `json:"data"`
	Error     string      `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
}

// ── 工作流类型 ──

// WorkflowType 工作流类型
type WorkflowType string

const (
	WorkflowTypeRequestProcess   WorkflowType = "request_process"   // 请求处理工作流
	WorkflowTypeRateLimitRecover WorkflowType = "rate_limit_recover" // 限速恢复工作流
	WorkflowTypeAccountRotate    WorkflowType = "account_rotate"     // 账号轮换工作流
	WorkflowTypeHealthMonitor    WorkflowType = "health_monitor"     // 健康监控工作流
)

// WorkflowStep 工作流步骤
type WorkflowStep struct {
	Name     string      `json:"name"`
	Agent    AgentType   `json:"agent"`
	TaskType TaskType    `json:"task_type"`
	Payload  interface{} `json:"payload"`
	Next     []string    `json:"next"`     // 下一步名称列表
	OnError  string      `json:"on_error"` // 错误时跳转
	Timeout  time.Duration `json:"timeout"`
}

// Workflow 工作流定义
type Workflow struct {
	Name        string          `json:"name"`
	Type        WorkflowType    `json:"type"`
	Description string          `json:"description"`
	Steps       []WorkflowStep  `json:"steps"`
	StartStep   string          `json:"start_step"`
}

// WorkflowInstance 工作流实例
type WorkflowInstance struct {
	ID          string                 `json:"id"`
	Workflow    Workflow               `json:"workflow"`
	CurrentStep string                 `json:"current_step"`
	Status      string                 `json:"status"`
	Context     map[string]interface{} `json:"context"`
	StartedAt   time.Time              `json:"started_at"`
	FinishedAt  *time.Time             `json:"finished_at,omitempty"`
	Results     []TaskResult           `json:"results"`
}

// ── 状态类型 ──

// RateLimitState 限速状态
type RateLimitState struct {
	IsLimited         bool      `json:"is_limited"`
	RemainingRequests int       `json:"remaining_requests"`
	RemainingTokens   int       `json:"remaining_tokens"`
	ResetAt           time.Time `json:"reset_at"`
	RetryAfter        int       `json:"retry_after"` // 秒
	Model             string    `json:"model"`
	AccountID         string    `json:"account_id"`
}

// AccountState 账号状态
type AccountState struct {
	ID                string    `json:"id"`
	Email             string    `json:"email"`
	PlanName          string    `json:"plan_name"`
	DailyRemaining    float64   `json:"daily_remaining"`
	WeeklyRemaining   float64   `json:"weekly_remaining"`
	IsHealthy         bool      `json:"is_healthy"`
	IsRateLimited     bool      `json:"is_rate_limited"`
	LastUsed          time.Time `json:"last_used"`
	TotalRequests     int       `json:"total_requests"`
	SuccessRequests   int       `json:"success_requests"`
}

// AgentState Agent 状态
type AgentState struct {
	Type          AgentType   `json:"type"`
	Status        AgentStatus `json:"status"`
	CurrentTask   *Task       `json:"current_task,omitempty"`
	TasksHandled  int         `json:"tasks_handled"`
	LastActive    time.Time   `json:"last_active"`
	ErrorCount    int         `json:"error_count"`
}

// SystemState 系统整体状态
type SystemState struct {
	Agents        map[AgentType]AgentState `json:"agents"`
	RateLimit     RateLimitState           `json:"rate_limit"`
	Accounts      []AccountState           `json:"accounts"`
	ActiveWorkflow *WorkflowInstance        `json:"active_workflow,omitempty"`
	LastUpdate    time.Time                `json:"last_update"`
}
