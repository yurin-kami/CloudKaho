package health

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status    string                 `json:"status"`
	Timestamp int64                  `json:"timestamp"`
	Checks    map[string]CheckResult `json:"checks,omitempty"`
}

// CheckResult 单项检查结果
type CheckResult struct {
	Status   string `json:"status"` // "pass", "fail", "warn"
	Duration int64  `json:"duration_ms"`
	Message  string `json:"message,omitempty"`
}
