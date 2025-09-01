package flink

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FlinkService Flink 服务管理
type FlinkService struct {
	baseURL    string
	httpClient *http.Client
}

// NewFlinkService 创建 Flink 服务实例
func NewFlinkService(baseURL string) *FlinkService {
	return &FlinkService{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ClusterOverview 集群概览响应
type ClusterOverview struct {
	TaskManagers   int    `json:"taskmanagers"`
	SlotsTotal     int    `json:"slots-total"`
	SlotsAvailable int    `json:"slots-available"`
	JobsRunning    int    `json:"jobs-running"`
	JobsFinished   int    `json:"jobs-finished"`
	JobsCancelled  int    `json:"jobs-cancelled"`
	JobsFailed     int    `json:"jobs-failed"`
	FlinkVersion   string `json:"flink-version"`
	FlinkCommit    string `json:"flink-commit"`
}

// Job 作业信息
type Job struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	State            string `json:"state"`
	StartTime        int64  `json:"start-time"`
	EndTime          int64  `json:"end-time"`
	Duration         int64  `json:"duration"`
	LastModification int64  `json:"last-modification"`
}

// JobsResponse 作业列表响应
type JobsResponse struct {
	Jobs []Job `json:"jobs"`
}

// JobDetails 作业详细信息
type JobDetails struct {
	JID        string                 `json:"jid"`
	Name       string                 `json:"name"`
	State      string                 `json:"state"`
	StartTime  int64                  `json:"start-time"`
	EndTime    int64                  `json:"end-time"`
	Duration   int64                  `json:"duration"`
	Now        int64                  `json:"now"`
	Timestamps map[string]int64       `json:"timestamps"`
	Vertices   []JobVertex            `json:"vertices"`
	StatusCounts map[string]int       `json:"status-counts"`
	Plan       map[string]interface{} `json:"plan"`
}

// JobVertex 作业顶点信息
type JobVertex struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Parallelism int    `json:"parallelism"`
	Status      string `json:"status"`
}

// TaskManager TaskManager 信息
type TaskManager struct {
	ID                        string                 `json:"id"`
	Path                      string                 `json:"path"`
	DataPort                  int                    `json:"dataPort"`
	JmxPort                   int                    `json:"jmxPort"`
	TimeSinceLastHeartbeat    int64                  `json:"timeSinceLastHeartbeat"`
	SlotsNumber               int                    `json:"slotsNumber"`
	FreeSlots                 int                    `json:"freeSlots"`
	TotalResource             ResourceInfo           `json:"totalResource"`
	FreeResource              ResourceInfo           `json:"freeResource"`
	Hardware                  HardwareInfo           `json:"hardware"`
	MemoryConfiguration       map[string]interface{} `json:"memoryConfiguration"`
}

// ResourceInfo 资源信息
type ResourceInfo struct {
	CPUCores         float64                `json:"cpuCores"`
	TaskHeapMemory   int64                  `json:"taskHeapMemory"`
	TaskOffHeapMemory int64                 `json:"taskOffHeapMemory"`
	ManagedMemory    int64                  `json:"managedMemory"`
	NetworkMemory    int64                  `json:"networkMemory"`
	ExtendedResources map[string]interface{} `json:"extendedResources"`
}

// HardwareInfo 硬件信息
type HardwareInfo struct {
	CPUCores       int   `json:"cpuCores"`
	PhysicalMemory int64 `json:"physicalMemory"`
	FreeMemory     int64 `json:"freeMemory"`
	ManagedMemory  int64 `json:"managedMemory"`
}

// TaskManagersResponse TaskManager 列表响应
type TaskManagersResponse struct {
	TaskManagers []TaskManager `json:"taskmanagers"`
}

// FlinkConfig Flink 配置信息
type FlinkConfig struct {
	RefreshInterval int                    `json:"refresh-interval"`
	TimezoneName    string                 `json:"timezone-name"`
	TimezoneOffset  int                    `json:"timezone-offset"`
	FlinkVersion    string                 `json:"flink-version"`
	FlinkRevision   string                 `json:"flink-revision"`
	Features        map[string]interface{} `json:"features"`
}

// JobMetrics 作业指标信息
type JobMetrics struct {
	JobID   string                 `json:"job_id"`
	Metrics map[string]interface{} `json:"metrics"`
}

// GetClusterOverview 获取集群概览
func (s *FlinkService) GetClusterOverview(ctx context.Context) (*ClusterOverview, error) {
	var overview ClusterOverview
	err := s.makeRequest(ctx, "/overview", &overview)
	return &overview, err
}

// GetJobs 获取所有作业
func (s *FlinkService) GetJobs(ctx context.Context) (*JobsResponse, error) {
	var jobs JobsResponse
	err := s.makeRequest(ctx, "/jobs", &jobs)
	return &jobs, err
}

// GetJobDetails 获取作业详细信息
func (s *FlinkService) GetJobDetails(ctx context.Context, jobID string) (*JobDetails, error) {
	var jobDetails JobDetails
	err := s.makeRequest(ctx, fmt.Sprintf("/jobs/%s", jobID), &jobDetails)
	return &jobDetails, err
}

// GetTaskManagers 获取所有 TaskManager
func (s *FlinkService) GetTaskManagers(ctx context.Context) (*TaskManagersResponse, error) {
	var taskManagers TaskManagersResponse
	err := s.makeRequest(ctx, "/taskmanagers", &taskManagers)
	return &taskManagers, err
}

// GetConfig 获取 Flink 配置
func (s *FlinkService) GetConfig(ctx context.Context) (*FlinkConfig, error) {
	var config FlinkConfig
	err := s.makeRequest(ctx, "/config", &config)
	return &config, err
}

// GetJobMetrics 获取作业指标
func (s *FlinkService) GetJobMetrics(ctx context.Context, jobID string) (*JobMetrics, error) {
	var metrics map[string]interface{}
	err := s.makeRequest(ctx, fmt.Sprintf("/jobs/%s/metrics", jobID), &metrics)
	if err != nil {
		return nil, err
	}
	
	return &JobMetrics{
		JobID:   jobID,
		Metrics: metrics,
	}, nil
}

// TestConnection 测试 Flink 连接
func (s *FlinkService) TestConnection(ctx context.Context) error {
	_, err := s.GetClusterOverview(ctx)
	return err
}

// makeRequest 发起 HTTP 请求的通用方法
func (s *FlinkService) makeRequest(ctx context.Context, endpoint string, result interface{}) error {
	url := s.baseURL + endpoint
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request to %s: %w", url, err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	return nil
}

// GetJobsOverview 获取作业概览统计
func (s *FlinkService) GetJobsOverview(ctx context.Context) (map[string]interface{}, error) {
	overview, err := s.GetClusterOverview(ctx)
	if err != nil {
		return nil, err
	}
	
	jobs, err := s.GetJobs(ctx)
	if err != nil {
		return nil, err
	}
	
	// 统计作业状态
	statusCounts := make(map[string]int)
	var runningJobs, finishedJobs, failedJobs, cancelledJobs []Job
	
	for _, job := range jobs.Jobs {
		statusCounts[job.State]++
		switch job.State {
		case "RUNNING":
			runningJobs = append(runningJobs, job)
		case "FINISHED":
			finishedJobs = append(finishedJobs, job)
		case "FAILED":
			failedJobs = append(failedJobs, job)
		case "CANCELLED":
			cancelledJobs = append(cancelledJobs, job)
		}
	}
	
	return map[string]interface{}{
		"cluster_overview": overview,
		"total_jobs":       len(jobs.Jobs),
		"status_counts":    statusCounts,
		"running_jobs":     runningJobs,
		"finished_jobs":    finishedJobs,
		"failed_jobs":      failedJobs,
		"cancelled_jobs":   cancelledJobs,
		"queried_at":       time.Now(),
	}, nil
}

// GetTaskManagersOverview 获取 TaskManager 概览
func (s *FlinkService) GetTaskManagersOverview(ctx context.Context) (map[string]interface{}, error) {
	taskManagers, err := s.GetTaskManagers(ctx)
	if err != nil {
		return nil, err
	}
	
	var totalSlots, freeSlots int
	var totalCPU, freeCPU float64
	var totalMemory, freeMemory int64
	var healthyTMs, unhealthyTMs int
	
	for _, tm := range taskManagers.TaskManagers {
		totalSlots += tm.SlotsNumber
		freeSlots += tm.FreeSlots
		totalCPU += tm.TotalResource.CPUCores
		freeCPU += tm.FreeResource.CPUCores
		totalMemory += tm.Hardware.PhysicalMemory
		freeMemory += tm.Hardware.FreeMemory
		
		// 简单的健康检查：如果心跳时间过长则认为不健康
		if time.Now().UnixMilli()-tm.TimeSinceLastHeartbeat > 60000 { // 60秒
			unhealthyTMs++
		} else {
			healthyTMs++
		}
	}
	
	return map[string]interface{}{
		"total_taskmanagers":   len(taskManagers.TaskManagers),
		"healthy_taskmanagers": healthyTMs,
		"unhealthy_taskmanagers": unhealthyTMs,
		"slots": map[string]int{
			"total": totalSlots,
			"free":  freeSlots,
			"used":  totalSlots - freeSlots,
		},
		"cpu": map[string]float64{
			"total": totalCPU,
			"free":  freeCPU,
			"used":  totalCPU - freeCPU,
		},
		"memory": map[string]int64{
			"total": totalMemory,
			"free":  freeMemory,
			"used":  totalMemory - freeMemory,
		},
		"taskmanagers": taskManagers.TaskManagers,
		"queried_at":   time.Now(),
	}, nil
}
