package models

import "time"

// HealthStatus 健康状态
type HealthStatus struct {
	Healthy      bool      `json:"healthy"`
	ResponseTime int64     `json:"response_time_ms"`
	LastCheck    time.Time `json:"last_check"`
	Error        string    `json:"error,omitempty"`
}
