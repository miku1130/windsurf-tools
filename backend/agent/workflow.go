package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WorkflowEngine 工作流引擎
type WorkflowEngine struct {
	mu         sync.RWMutex
	agents     map[AgentType]Agent
	workflows  map[WorkflowType]*Workflow
	instances  map[string]*WorkflowInstance
	tools      *ToolRegistry
	messageBus *MessageBus
	stopCh     chan struct{}
	logFn      func(string, ...interface{})
}

// NewWorkflowEngine 创建工作流引擎
func NewWorkflowEngine() *WorkflowEngine {
	engine := &WorkflowEngine{
		agents:     make(map[AgentType]Agent),
		workflows:  make(map[WorkflowType]*Workflow),
		instances:  make(map[string]*WorkflowInstance),
		tools:      NewToolRegistry(),
		messageBus: NewMessageBus(),
		stopCh:     make(chan struct{}),
		logFn: func(format string, args ...interface{}) {
			log.Printf("[WorkflowEngine] %s", fmt.Sprintf(format, args...))
		},
	}

	// 注册内置工作流
	engine.registerBuiltinWorkflows()

	return engine
}

func (e *WorkflowEngine) log(format string, args ...interface{}) {
	if e.logFn != nil {
		e.logFn(format, args...)
	}
}

// RegisterAgent 注册 Agent
func (e *WorkflowEngine) RegisterAgent(agent Agent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agents[agent.GetType()] = agent
	e.messageBus.RegisterAgent(agent)
}

// RegisterWorkflow 注册工作流
func (e *WorkflowEngine) RegisterWorkflow(workflow *Workflow) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.workflows[workflow.Type] = workflow
}

// GetTools 获取工具注册表
func (e *WorkflowEngine) GetTools() *ToolRegistry {
	return e.tools
}

// Start 启动引擎
func (e *WorkflowEngine) Start(ctx context.Context) error {
	e.log("启动工作流引擎...")

	// 启动所有 Agent
	for _, agent := range e.agents {
		if err := agent.Start(ctx); err != nil {
			return fmt.Errorf("启动 Agent %s 失败: %w", agent.GetType(), err)
		}
	}

	// 启动消息总线
	e.messageBus.Start(ctx)

	e.log("工作流引擎已启动")
	return nil
}

// Stop 停止引擎
func (e *WorkflowEngine) Stop() error {
	e.log("停止工作流引擎...")

	// 停止所有 Agent
	for _, agent := range e.agents {
		if err := agent.Stop(); err != nil {
			e.log("停止 Agent %s 失败: %v", agent.GetType(), err)
		}
	}

	// 停止消息总线
	e.messageBus.Stop()

	close(e.stopCh)
	e.log("工作流引擎已停止")
	return nil
}

// ExecuteWorkflow 执行工作流
func (e *WorkflowEngine) ExecuteWorkflow(ctx context.Context, workflowType WorkflowType, params map[string]interface{}) (*WorkflowInstance, error) {
	e.mu.RLock()
	workflow, ok := e.workflows[workflowType]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("工作流未找到: %s", workflowType)
	}

	// 创建工作流实例
	if params == nil {
		params = make(map[string]interface{})
	}
	instance := &WorkflowInstance{
		ID:          uuid.New().String(),
		Workflow:    *workflow,
		CurrentStep: workflow.StartStep,
		Status:      "running",
		Context:     params,
		StartedAt:   time.Now(),
		Results:     make([]TaskResult, 0),
	}

	e.mu.Lock()
	e.instances[instance.ID] = instance
	e.mu.Unlock()

	e.log("启动工作流: %s (ID: %s)", workflow.Name, instance.ID)

	// 异步执行工作流
	go e.runWorkflow(ctx, instance)

	return instance, nil
}

func (e *WorkflowEngine) runWorkflow(ctx context.Context, instance *WorkflowInstance) {
	for {
		// 查找当前步骤
		step := e.findStep(instance.Workflow, instance.CurrentStep)
		if step == nil {
			e.log("工作流步骤未找到: %s", instance.CurrentStep)
			instance.Status = "failed"
			now := time.Now()
			instance.FinishedAt = &now
			return
		}

		e.log("执行步骤: %s (Agent: %s)", step.Name, step.Agent)

		// 获取对应的 Agent
		agent, ok := e.agents[step.Agent]
		if !ok {
			e.log("Agent 未找到: %s", step.Agent)
			instance.Status = "failed"
			now := time.Now()
			instance.FinishedAt = &now
			return
		}

		// 创建任务
		task := &Task{
			ID:       uuid.New().String(),
			Type:     step.TaskType,
			Priority: 1,
			Payload:  instance.Context,
			Created:  time.Now(),
			Timeout:  step.Timeout,
		}

		// 执行任务
		taskCtx := ctx
		if step.Timeout > 0 {
			var cancel context.CancelFunc
			taskCtx, cancel = context.WithTimeout(ctx, step.Timeout)
			defer cancel()
		}

		result, err := agent.HandleTask(taskCtx, task)
		if err != nil {
			e.log("任务执行失败: %v", err)
			instance.Results = append(instance.Results, TaskResult{
				TaskID:    task.ID,
				Success:   false,
				Error:     err.Error(),
				Timestamp: time.Now(),
			})

			// 处理错误
			if step.OnError != "" {
				instance.CurrentStep = step.OnError
				continue
			}

			instance.Status = "failed"
			now := time.Now()
			instance.FinishedAt = &now
			return
		}

		// 记录结果
		instance.Results = append(instance.Results, *result)

		// 更新上下文
		if result.Data != nil {
			if dataMap, ok := result.Data.(map[string]interface{}); ok {
				if instance.Context == nil {
					instance.Context = make(map[string]interface{})
				}
				for k, v := range dataMap {
					instance.Context[k] = v
				}
			}
		}

		// 查找下一步
		if len(step.Next) == 0 {
			// 没有下一步，工作流完成
			instance.Status = "completed"
			now := time.Now()
			instance.FinishedAt = &now
			e.log("工作流完成: %s", instance.ID)
			return
		}

		// 根据结果选择下一步
		nextStep := e.selectNextStep(step, result)
		instance.CurrentStep = nextStep
	}
}

func (e *WorkflowEngine) findStep(workflow Workflow, stepName string) *WorkflowStep {
	for _, step := range workflow.Steps {
		if step.Name == stepName {
			return &step
		}
	}
	return nil
}

func (e *WorkflowEngine) selectNextStep(currentStep *WorkflowStep, result *TaskResult) string {
	if len(currentStep.Next) == 1 {
		return currentStep.Next[0]
	}

	// 根据结果选择下一步
	if result.Success {
		for _, next := range currentStep.Next {
			if next != currentStep.OnError {
				return next
			}
		}
	}

	return currentStep.Next[0]
}

// GetInstance 获取工作流实例
func (e *WorkflowEngine) GetInstance(id string) *WorkflowInstance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances[id]
}

// GetActiveWorkflows 获取活跃工作流
func (e *WorkflowEngine) GetActiveWorkflows() []*WorkflowInstance {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var active []*WorkflowInstance
	for _, instance := range e.instances {
		if instance.Status == "running" {
			active = append(active, instance)
		}
	}
	return active
}

// registerBuiltinWorkflows 注册内置工作流
func (e *WorkflowEngine) registerBuiltinWorkflows() {
	// 请求处理工作流
	e.RegisterWorkflow(&Workflow{
		Name:        "请求处理",
		Type:        WorkflowTypeRequestProcess,
		Description: "处理用户请求，包括限速检查、账号选择、模型选择、请求转发",
		StartStep:   "check_rate_limit",
		Steps: []WorkflowStep{
			{
				Name:     "check_rate_limit",
				Agent:    AgentTypeRateLimiter,
				TaskType: TaskTypeCheckRateLimit,
				Next:     []string{"select_account"},
				OnError:  "handle_rate_limit",
				Timeout:  5 * time.Second,
			},
			{
				Name:     "select_account",
				Agent:    AgentTypeAccountManager,
				TaskType: TaskTypeRotateAccount,
				Next:     []string{"select_model"},
				OnError:  "handle_account_error",
				Timeout:  10 * time.Second,
			},
			{
				Name:     "select_model",
				Agent:    AgentTypeModelSelector,
				TaskType: TaskTypeSelectModel,
				Next:     []string{"forward_request"},
				OnError:  "handle_model_error",
				Timeout:  5 * time.Second,
			},
			{
				Name:     "forward_request",
				Agent:    AgentTypeRequestRouter,
				TaskType: TaskTypeRouteRequest,
				Next:     []string{},
				Timeout:  30 * time.Second,
			},
			{
				Name:     "handle_rate_limit",
				Agent:    AgentTypeRateLimiter,
				TaskType: TaskTypeCheckRateLimit,
				Next:     []string{"rotate_fingerprint"},
				Timeout:  5 * time.Second,
			},
			{
				Name:     "rotate_fingerprint",
				Agent:    AgentTypeAccountManager,
				TaskType: TaskTypeRotateFingerprint,
				Next:     []string{"select_account"},
				Timeout:  10 * time.Second,
			},
			{
				Name:     "handle_account_error",
				Agent:    AgentTypeAccountManager,
				TaskType: TaskTypeHealthCheck,
				Next:     []string{"select_account"},
				Timeout:  5 * time.Second,
			},
			{
				Name:     "handle_model_error",
				Agent:    AgentTypeModelSelector,
				TaskType: TaskTypeSelectModel,
				Next:     []string{"select_model"},
				Timeout:  5 * time.Second,
			},
		},
	})

	// 限速恢复工作流
	e.RegisterWorkflow(&Workflow{
		Name:        "限速恢复",
		Type:        WorkflowTypeRateLimitRecover,
		Description: "检测到限速后自动恢复",
		StartStep:   "check_state",
		Steps: []WorkflowStep{
			{
				Name:     "check_state",
				Agent:    AgentTypeRateLimiter,
				TaskType: TaskTypeCheckRateLimit,
				Next:     []string{"rotate_fingerprint"},
				Timeout:  5 * time.Second,
			},
			{
				Name:     "rotate_fingerprint",
				Agent:    AgentTypeAccountManager,
				TaskType: TaskTypeRotateFingerprint,
				Next:     []string{"rotate_account"},
				Timeout:  10 * time.Second,
			},
			{
				Name:     "rotate_account",
				Agent:    AgentTypeAccountManager,
				TaskType: TaskTypeRotateAccount,
				Next:     []string{"verify_recovery"},
				Timeout:  10 * time.Second,
			},
			{
				Name:     "verify_recovery",
				Agent:    AgentTypeRateLimiter,
				TaskType: TaskTypeCheckRateLimit,
				Next:     []string{},
				Timeout:  5 * time.Second,
			},
		},
	})

	// 健康监控工作流
	e.RegisterWorkflow(&Workflow{
		Name:        "健康监控",
		Type:        WorkflowTypeHealthMonitor,
		Description: "定期检查系统健康状态",
		StartStep:   "check_accounts",
		Steps: []WorkflowStep{
			{
				Name:     "check_accounts",
				Agent:    AgentTypeAccountManager,
				TaskType: TaskTypeHealthCheck,
				Next:     []string{"check_rate_limit"},
				Timeout:  10 * time.Second,
			},
			{
				Name:     "check_rate_limit",
				Agent:    AgentTypeRateLimiter,
				TaskType: TaskTypeCheckRateLimit,
				Next:     []string{},
				Timeout:  5 * time.Second,
			},
		},
	})
}

// ── 消息总线 ──

// MessageBus 消息总线
type MessageBus struct {
	mu      sync.RWMutex
	agents  map[AgentType]Agent
	channel chan *Message
	stopCh  chan struct{}
}

// NewMessageBus 创建消息总线
func NewMessageBus() *MessageBus {
	return &MessageBus{
		agents:  make(map[AgentType]Agent),
		channel: make(chan *Message, 1000),
		stopCh:  make(chan struct{}),
	}
}

func (b *MessageBus) RegisterAgent(agent Agent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.agents[agent.GetType()] = agent
}

func (b *MessageBus) Start(ctx context.Context) {
	go b.dispatchLoop(ctx)
}

func (b *MessageBus) Stop() {
	close(b.stopCh)
}

func (b *MessageBus) Send(msg *Message) error {
	select {
	case b.channel <- msg:
		return nil
	default:
		return fmt.Errorf("消息队列已满")
	}
}

func (b *MessageBus) dispatchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stopCh:
			return
		case msg := <-b.channel:
			b.dispatch(msg)
		}
	}
}

func (b *MessageBus) dispatch(msg *Message) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if msg.To != "" {
		// 发送到指定 Agent
		if agent, ok := b.agents[msg.To]; ok {
			if err := agent.HandleMessage(msg); err != nil {
				log.Printf("[MessageBus] 发送消息失败: %v", err)
			}
		}
	} else {
		// 广播到所有 Agent
		for _, agent := range b.agents {
			if err := agent.HandleMessage(msg); err != nil {
				log.Printf("[MessageBus] 广播消息失败: %v", err)
			}
		}
	}
}

// ── 辅助函数 ──

// GetSystemState 获取系统状态
func (e *WorkflowEngine) GetSystemState() SystemState {
	e.mu.RLock()
	defer e.mu.RUnlock()

	state := SystemState{
		Agents:     make(map[AgentType]AgentState),
		LastUpdate: time.Now(),
	}

	// 收集 Agent 状态
	for agentType, agent := range e.agents {
		state.Agents[agentType] = agent.GetState()
	}

	// 获取限速状态
	if rateLimiter, ok := e.agents[AgentTypeRateLimiter]; ok {
		if rl, ok := rateLimiter.(*RateLimitMonitorAgent); ok {
			state.RateLimit = rl.GetRateLimitState()
		}
	}

	// 获取活跃工作流
	activeWorkflows := e.GetActiveWorkflows()
	if len(activeWorkflows) > 0 {
		state.ActiveWorkflow = activeWorkflows[0]
	}

	return state
}
