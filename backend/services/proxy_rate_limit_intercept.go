package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ── 限速拦截配置 ──

// RateLimitInterceptConfig 控制限速拦截行为
type RateLimitInterceptConfig struct {
	// Enabled 是否启用限速拦截
	Enabled bool
	// InterceptCheckUserMessage 是否拦截 CheckUserMessageRateLimit 请求
	InterceptCheckUserMessage bool
	// StripRateLimitHeaders 是否清除响应中的 x-ratelimit-* 头
	StripRateLimitHeaders bool
	// BypassLocalCache 是否绕过本地限速缓存
	BypassLocalCache bool
}

// ── 限速检查路径 ──

const (
	// CheckUserMessageRateLimitPath 是限速检查的 API 路径
	CheckUserMessageRateLimitPath = "CheckUserMessageRateLimit"
	// CheckUserMessageRateLimitPathAlt 是备选路径
	CheckUserMessageRateLimitPathAlt = "check_user_message_rate_limit"
)

// ── 限速响应结构 ──

// RateLimitResponse 是拦截限速检查时返回的假响应
type RateLimitResponse struct {
	Allowed        bool   `json:"allowed"`
	Remaining      int    `json:"remaining"`
	ResetAt        string `json:"reset_at"`
	Reason         string `json:"reason"`
	IsRateLimited  bool   `json:"is_rate_limited"`
}

// ── 限速响应头列表 ──

var rateLimitHeaders = []string{
	"x-ratelimit-limit-requests",
	"x-ratelimit-reset-requests",
	"x-ratelimit-limit-tokens",
	"x-ratelimit-reset-tokens",
	"x-ratelimit-remaining-requests",
	"x-ratelimit-remaining-tokens",
	"retry-after",
	"Retry-After",
}

// ── 拦截逻辑 ──

// isRateLimitCheckPath 检查路径是否是限速检查请求
func isRateLimitCheckPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, strings.ToLower(CheckUserMessageRateLimitPath)) ||
		strings.Contains(lower, strings.ToLower(CheckUserMessageRateLimitPathAlt))
}

// interceptRateLimitCheck 拦截限速检查请求，返回"未限速"响应
func (p *MitmProxy) interceptRateLimitCheck(req *http.Request) *http.Response {
	// 构造假的成功响应
	responseBody := map[string]interface{}{
		"allowed":        true,
		"remaining":      999999,
		"reset_at":       "2099-01-01T00:00:00Z",
		"reason":         "intercepted by windsurf-tools",
		"is_rate_limited": false,
	}

	bodyBytes, err := json.Marshal(responseBody)
	if err != nil {
		p.log("限速拦截: 序列化响应失败: %v", err)
		return nil
	}

	p.log("★ 限速拦截: 已拦截 %s 请求，返回未限速响应", req.URL.Path)

	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		Header: http.Header{
			"Content-Type":   []string{"application/json"},
			"X-Mitm-Intercepted": []string{"rate-limit-check"},
		},
		ContentLength: int64(len(bodyBytes)),
		Request:       req,
	}
}

// stripRateLimitHeaders 从响应中移除限速相关的头
func (p *MitmProxy) stripRateLimitHeaders(resp *http.Response) {
	if resp == nil || resp.Header == nil {
		return
	}

	stripped := false
	for _, header := range rateLimitHeaders {
		if resp.Header.Get(header) != "" {
			resp.Header.Del(header)
			stripped = true
		}
	}

	if stripped {
		p.log("★ 已清除响应中的限速头: %s", resp.Request.URL.Path)
	}
}

// injectFakeRateLimitHeaders 注入假的限速头，让 IDE 认为没有限速
func (p *MitmProxy) injectFakeRateLimitHeaders(resp *http.Response) {
	if resp == nil || resp.Header == nil {
		return
	}

	// 注入假的限速头，表示有大量剩余配额
	resp.Header.Set("x-ratelimit-limit-requests", "999999")
	resp.Header.Set("x-ratelimit-remaining-requests", "999999")
	resp.Header.Set("x-ratelimit-reset-requests", "2524608000") // 2050年
	resp.Header.Set("x-ratelimit-limit-tokens", "999999999")
	resp.Header.Set("x-ratelimit-remaining-tokens", "999999999")
	resp.Header.Set("x-ratelimit-reset-tokens", "2524608000")
}

// ── 处理器集成 ──

// handleRateLimitInterceptRequest 处理请求阶段的限速拦截
// 返回非 nil 表示已拦截，应直接返回该响应
func (p *MitmProxy) handleRateLimitInterceptRequest(req *http.Request, cfg RateLimitInterceptConfig) *http.Response {
	if !cfg.Enabled {
		return nil
	}

	// 拦截 CheckUserMessageRateLimit 请求
	if cfg.InterceptCheckUserMessage && isRateLimitCheckPath(req.URL.Path) {
		return p.interceptRateLimitCheck(req)
	}

	return nil
}

// handleRateLimitInterceptResponse 处理响应阶段的限速拦截
func (p *MitmProxy) handleRateLimitInterceptResponse(resp *http.Response, cfg RateLimitInterceptConfig) {
	if !cfg.Enabled || resp == nil {
		return
	}

	// 清除限速响应头
	if cfg.StripRateLimitHeaders {
		p.stripRateLimitHeaders(resp)
	}

	// 注入假的限速头（可选，用于绕过本地缓存）
	if cfg.BypassLocalCache {
		p.injectFakeRateLimitHeaders(resp)
	}
}

// GetRateLimitInterceptConfig 从设置中获取限速拦截配置
func (p *MitmProxy) GetRateLimitInterceptConfig() RateLimitInterceptConfig {
	// 默认配置：启用所有拦截
	cfg := RateLimitInterceptConfig{
		Enabled:                   true,
		InterceptCheckUserMessage: true,
		StripRateLimitHeaders:     true,
		BypassLocalCache:          true,
	}

	// 如果有 store，从设置中读取配置
	if p.store != nil {
		settings := p.store.GetSettings()
		cfg.Enabled = settings.RateLimitInterceptEnabled
		cfg.InterceptCheckUserMessage = settings.RateLimitInterceptCheckUserMessage
		cfg.StripRateLimitHeaders = settings.RateLimitStripHeaders
		cfg.BypassLocalCache = settings.RateLimitBypassLocalCache
	}

	return cfg
}

// ── 辅助函数 ──

// isRateLimitResponse 检查响应是否包含限速错误
func isRateLimitResponse(resp *http.Response) bool {
	if resp == nil {
		return false
	}

	// 检查状态码
	if resp.StatusCode == 429 {
		return true
	}

	// 检查限速头
	for _, header := range rateLimitHeaders {
		if resp.Header.Get(header) != "" {
			return true
		}
	}

	return false
}

// logRateLimitState 记录限速状态（用于调试）
func (p *MitmProxy) logRateLimitState(resp *http.Response) {
	if resp == nil {
		return
	}

	state := map[string]string{
		"path":              resp.Request.URL.Path,
		"status":            fmt.Sprintf("%d", resp.StatusCode),
		"limit-requests":    resp.Header.Get("x-ratelimit-limit-requests"),
		"remaining-requests": resp.Header.Get("x-ratelimit-remaining-requests"),
		"reset-requests":    resp.Header.Get("x-ratelimit-reset-requests"),
		"limit-tokens":      resp.Header.Get("x-ratelimit-limit-tokens"),
		"remaining-tokens":  resp.Header.Get("x-ratelimit-remaining-tokens"),
		"reset-tokens":      resp.Header.Get("x-ratelimit-reset-tokens"),
		"retry-after":       resp.Header.Get("retry-after"),
	}

	p.log("限速状态: %v", state)
}
