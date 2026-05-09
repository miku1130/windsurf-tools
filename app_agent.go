package main

import (
	"log"
	"windsurf-tools-wails/backend/agent"
)

// ── Agent 编排器集成 ──

// initAgent 初始化 Agent 编排器并启动
func (a *App) initAgent() {
	o := agent.NewOrchestrator()

	// 注入实际业务回调
	opts := []agent.Option{}

	// 账号切换 → 复用已有 MITM 轮换逻辑
	opts = append(opts, agent.WithAccountSwitchFn(func(accountID string) error {
		_, err := a.SwitchMitmToAccount(accountID)
		return err
	}))

	// 设备指纹轮换
	opts = append(opts, agent.WithFingerprintRotateFn(func() error {
		return a.ResetMachineFingerprint()
	}))

	if err := o.Initialize(opts...); err != nil {
		log.Printf("[Agent] 初始化失败: %v", err)
		return
	}

	if err := o.Start(); err != nil {
		log.Printf("[Agent] 启动失败: %v", err)
		return
	}

	a.mu.Lock()
	a.orchestrator = o
	a.mu.Unlock()

	log.Printf("[Agent] 编排器已启动")
}

// stopAgent 停止 Agent 编排器
func (a *App) stopAgent() {
	a.mu.Lock()
	o := a.orchestrator
	a.mu.Unlock()
	if o != nil {
		_ = o.Stop()
		log.Printf("[Agent] 编排器已停止")
	}
}

// ── 暴露给 Wails 前端的方法 ──

// AgentSystemState 前端可用的系统状态
type AgentSystemState struct {
	Agents         []AgentInfo    `json:"agents"`
	RateLimit      interface{}    `json:"rate_limit"`
	ActiveWorkflow interface{}    `json:"active_workflow"`
	Workflows      []WorkflowDef `json:"workflows"`
	LastUpdate     string         `json:"last_update"`
}

// AgentInfo Agent 信息
type AgentInfo struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	TasksHandled int    `json:"tasks_handled"`
	ErrorCount   int    `json:"error_count"`
	LastActive   string `json:"last_active"`
}

// WorkflowDef 工作流定义（含步骤链）
type WorkflowDef struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Steps       []WorkflowStep `json:"steps"`
	StartStep   string         `json:"start_step"`
}

// WorkflowStep 工作流步骤
type WorkflowStep struct {
	Name     string   `json:"name"`
	Agent    string   `json:"agent"`
	TaskType string   `json:"task_type"`
	Next     []string `json:"next"`
	OnError  string   `json:"on_error"`
}

// agentTypeNames Agent 类型中文名映射
var agentTypeNames = map[string]string{
	"orchestrator":    "主控编排器",
	"rate_limiter":    "限速监控",
	"account_manager": "账号管理",
	"request_router":  "请求路由",
	"model_selector":  "模型选择",
}

// GetAgentSystemState 获取 Agent 系统状态（Wails 绑定）
func (a *App) GetAgentSystemState() *AgentSystemState {
	a.mu.Lock()
	o := a.orchestrator
	a.mu.Unlock()

	if o == nil {
		return &AgentSystemState{
			Agents:    []AgentInfo{},
			Workflows: []WorkflowDef{},
		}
	}

	state := o.GetState()

	// 转换 Agent 状态
	agents := make([]AgentInfo, 0, len(state.Agents))
	for agentType, agentState := range state.Agents {
		name := agentTypeNames[string(agentType)]
		if name == "" {
			name = string(agentType)
		}
		agents = append(agents, AgentInfo{
			Type:         string(agentType),
			Name:         name,
			Status:       string(agentState.Status),
			TasksHandled: agentState.TasksHandled,
			ErrorCount:   agentState.ErrorCount,
			LastActive:   agentState.LastActive.Format("2006-01-02 15:04:05"),
		})
	}

	// 转换工作流定义
	workflows := getWorkflowDefinitions()

	result := &AgentSystemState{
		Agents:    agents,
		RateLimit: state.RateLimit,
		Workflows: workflows,
		LastUpdate: state.LastUpdate.Format("2006-01-02 15:04:05"),
	}

	// 活跃工作流
	if state.ActiveWorkflow != nil {
		result.ActiveWorkflow = map[string]interface{}{
			"id":           state.ActiveWorkflow.ID,
			"workflow_name": state.ActiveWorkflow.Workflow.Name,
			"workflow_type": string(state.ActiveWorkflow.Workflow.Type),
			"current_step": state.ActiveWorkflow.CurrentStep,
			"status":       state.ActiveWorkflow.Status,
			"started_at":   state.ActiveWorkflow.StartedAt.Format("2006-01-02 15:04:05"),
			"results_count": len(state.ActiveWorkflow.Results),
		}
	}

	return result
}

// getWorkflowDefinitions 返回内置工作流定义（用于前端可视化）
func getWorkflowDefinitions() []WorkflowDef {
	return []WorkflowDef{
		{
			Name:        "请求处理",
			Type:        "request_process",
			Description: "处理用户请求，包括限速检查、账号选择、模型选择、请求转发",
			StartStep:   "check_rate_limit",
			Steps: []WorkflowStep{
				{Name: "check_rate_limit", Agent: "rate_limiter", TaskType: "check_rate_limit", Next: []string{"select_account"}, OnError: "handle_rate_limit"},
				{Name: "select_account", Agent: "account_manager", TaskType: "rotate_account", Next: []string{"select_model"}, OnError: "handle_account_error"},
				{Name: "select_model", Agent: "model_selector", TaskType: "select_model", Next: []string{"forward_request"}, OnError: "handle_model_error"},
				{Name: "forward_request", Agent: "request_router", TaskType: "route_request", Next: []string{}},
				{Name: "handle_rate_limit", Agent: "rate_limiter", TaskType: "check_rate_limit", Next: []string{"rotate_fingerprint"}},
				{Name: "rotate_fingerprint", Agent: "account_manager", TaskType: "rotate_fingerprint", Next: []string{"select_account"}},
				{Name: "handle_account_error", Agent: "account_manager", TaskType: "health_check", Next: []string{"select_account"}},
				{Name: "handle_model_error", Agent: "model_selector", TaskType: "select_model", Next: []string{"select_model"}},
			},
		},
		{
			Name:        "限速恢复",
			Type:        "rate_limit_recover",
			Description: "检测到限速后自动恢复：状态检查 → 指纹轮换 → 账号轮换 → 验证",
			StartStep:   "check_state",
			Steps: []WorkflowStep{
				{Name: "check_state", Agent: "rate_limiter", TaskType: "check_rate_limit", Next: []string{"rotate_fingerprint"}},
				{Name: "rotate_fingerprint", Agent: "account_manager", TaskType: "rotate_fingerprint", Next: []string{"rotate_account"}},
				{Name: "rotate_account", Agent: "account_manager", TaskType: "rotate_account", Next: []string{"verify_recovery"}},
				{Name: "verify_recovery", Agent: "rate_limiter", TaskType: "check_rate_limit", Next: []string{}},
			},
		},
		{
			Name:        "健康监控",
			Type:        "health_monitor",
			Description: "定期检查系统健康状态：账号检查 → 限速检查",
			StartStep:   "check_accounts",
			Steps: []WorkflowStep{
				{Name: "check_accounts", Agent: "account_manager", TaskType: "health_check", Next: []string{"check_rate_limit"}},
				{Name: "check_rate_limit", Agent: "rate_limiter", TaskType: "check_rate_limit", Next: []string{}},
			},
		},
	}
}
