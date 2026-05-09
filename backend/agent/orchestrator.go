package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Orchestrator 主控编排器
type Orchestrator struct {
	mu      sync.RWMutex
	engine  *WorkflowEngine
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	logFn   func(string, ...interface{})
}

// NewOrchestrator 创建编排器
func NewOrchestrator() *Orchestrator {
	o := &Orchestrator{
		engine: NewWorkflowEngine(),
		logFn: func(format string, args ...interface{}) {
			log.Printf("[Orchestrator] %s", fmt.Sprintf(format, args...))
		},
	}
	return o
}

func (o *Orchestrator) log(format string, args ...interface{}) {
	if o.logFn != nil {
		o.logFn(format, args...)
	}
}

// GetEngine 获取工作流引擎
func (o *Orchestrator) GetEngine() *WorkflowEngine {
	return o.engine
}

// Initialize 初始化编排器
func (o *Orchestrator) Initialize(opts ...Option) error {
	o.log("初始化编排器...")

	// 应用选项
	config := &Config{}
	for _, opt := range opts {
		opt(config)
	}

	// 创建工具
	tools := o.engine.GetTools()

	// 注册限速检测工具
	rateLimitTool := NewRateLimitCheckTool()
	tools.Register(&Tool{
		Name:        ToolTypeRateLimitCheck,
		Description: "检查当前限速状态",
		Handler:     rateLimitTool.Execute,
	})

	// 注册账号切换工具
	if config.AccountSwitchFn != nil {
		accountSwitchTool := NewAccountSwitchTool(config.AccountSwitchFn)
		tools.Register(&Tool{
			Name:        ToolTypeAccountSwitch,
			Description: "切换到指定账号",
			Handler:     accountSwitchTool.Execute,
		})
	}

	// 注册模型切换工具
	modelSwitchTool := NewModelSwitchTool()
	tools.Register(&Tool{
		Name:        ToolTypeModelSwitch,
		Description: "切换到可用模型",
		Handler:     modelSwitchTool.Execute,
	})

	// 注册设备指纹轮换工具
	if config.FingerprintRotateFn != nil {
		fingerprintTool := NewFingerprintRotateTool(config.FingerprintRotateFn)
		tools.Register(&Tool{
			Name:        ToolTypeFingerprintRotate,
			Description: "轮换设备指纹",
			Handler:     fingerprintTool.Execute,
		})
	}

	// 注册额度查询工具
	if config.QuotaQueryFn != nil {
		quotaTool := NewQuotaQueryTool(config.QuotaQueryFn)
		tools.Register(&Tool{
			Name:        ToolTypeQuotaQuery,
			Description: "查询账号额度",
			Handler:     quotaTool.Execute,
		})
	}

	// 注册请求转发工具
	if config.RequestForwardFn != nil {
		forwardTool := NewRequestForwardTool(config.RequestForwardFn)
		tools.Register(&Tool{
			Name:        ToolTypeRequestForward,
			Description: "转发请求到上游",
			Handler:     forwardTool.Execute,
		})
	}

	// 注册缓存管理工具
	cacheTool := NewCacheManagerTool()
	tools.Register(&Tool{
		Name:        ToolTypeCacheManager,
		Description: "管理本地缓存",
		Handler:     cacheTool.Execute,
	})

	// 创建 Agent
	rateLimitAgent := NewRateLimitMonitorAgent(tools)
	accountAgent := NewAccountManagerAgent(tools)
	modelAgent := NewModelSelectorAgent(tools)

	// 注册 Agent
	o.engine.RegisterAgent(rateLimitAgent)
	o.engine.RegisterAgent(accountAgent)
	o.engine.RegisterAgent(modelAgent)

	o.log("编排器初始化完成")
	return nil
}

// Start 启动编排器
func (o *Orchestrator) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.running {
		return fmt.Errorf("编排器已在运行")
	}

	o.ctx, o.cancel = context.WithCancel(context.Background())

	if err := o.engine.Start(o.ctx); err != nil {
		return fmt.Errorf("启动工作流引擎失败: %w", err)
	}

	o.running = true
	o.log("编排器已启动")

	// 启动定期健康检查
	go o.healthCheckLoop()

	return nil
}

// Stop 停止编排器
func (o *Orchestrator) Stop() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.running {
		return nil
	}

	if o.cancel != nil {
		o.cancel()
	}

	if err := o.engine.Stop(); err != nil {
		o.log("停止工作流引擎失败: %v", err)
	}

	o.running = false
	o.log("编排器已停止")
	return nil
}

// ProcessRequest 处理请求
func (o *Orchestrator) ProcessRequest(ctx context.Context, params map[string]interface{}) (*WorkflowInstance, error) {
	o.log("处理请求...")

	// 执行请求处理工作流
	instance, err := o.engine.ExecuteWorkflow(ctx, WorkflowTypeRequestProcess, params)
	if err != nil {
		return nil, fmt.Errorf("执行请求处理工作流失败: %w", err)
	}

	return instance, nil
}

// RecoverFromRateLimit 从限速中恢复
func (o *Orchestrator) RecoverFromRateLimit(ctx context.Context) (*WorkflowInstance, error) {
	o.log("执行限速恢复...")

	instance, err := o.engine.ExecuteWorkflow(ctx, WorkflowTypeRateLimitRecover, nil)
	if err != nil {
		return nil, fmt.Errorf("执行限速恢复工作流失败: %w", err)
	}

	return instance, nil
}

// GetState 获取系统状态
func (o *Orchestrator) GetState() SystemState {
	return o.engine.GetSystemState()
}

// healthCheckLoop 定期健康检查
func (o *Orchestrator) healthCheckLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.performHealthCheck()
		}
	}
}

func (o *Orchestrator) performHealthCheck() {
	o.log("执行健康检查...")

	instance, err := o.engine.ExecuteWorkflow(o.ctx, WorkflowTypeHealthMonitor, nil)
	if err != nil {
		o.log("健康检查失败: %v", err)
		return
	}

	// 等待完成
	for instance.Status == "running" {
		time.Sleep(100 * time.Millisecond)
	}

	o.log("健康检查完成: %s", instance.Status)
}

// ── 配置选项 ──

// Config 编排器配置
type Config struct {
	AccountSwitchFn    func(accountID string) error
	FingerprintRotateFn func() error
	QuotaQueryFn       func(accountID string) (*QuotaInfo, error)
	RequestForwardFn   func(req *RequestData) (*ResponseData, error)
}

// Option 配置选项
type Option func(*Config)

// WithAccountSwitchFn 设置账号切换函数
func WithAccountSwitchFn(fn func(string) error) Option {
	return func(c *Config) {
		c.AccountSwitchFn = fn
	}
}

// WithFingerprintRotateFn 设置指纹轮换函数
func WithFingerprintRotateFn(fn func() error) Option {
	return func(c *Config) {
		c.FingerprintRotateFn = fn
	}
}

// WithQuotaQueryFn 设置额度查询函数
func WithQuotaQueryFn(fn func(string) (*QuotaInfo, error)) Option {
	return func(c *Config) {
		c.QuotaQueryFn = fn
	}
}

// WithRequestForwardFn 设置请求转发函数
func WithRequestForwardFn(fn func(*RequestData) (*ResponseData, error)) Option {
	return func(c *Config) {
		c.RequestForwardFn = fn
	}
}
