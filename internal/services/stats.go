package services

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/moby/moby/client"
)

type ContainerStats struct {
	ServerID    string    `json:"server_id"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryUsed  uint64    `json:"memory_used"`
	MemoryLimit uint64    `json:"memory_limit"`
	MemoryPct   float64   `json:"memory_percent"`
	NetRx       uint64    `json:"net_rx"`
	NetTx       uint64    `json:"net_tx"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type StatsService struct {
	docker *client.Client
	cache  map[string]*ContainerStats
	mu     sync.RWMutex
}

func NewStatsService(docker *client.Client) *StatsService {
	return &StatsService{
		docker: docker,
		cache:  make(map[string]*ContainerStats),
	}
}

func (s *StatsService) Get(serverID string) *ContainerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[serverID]
}

func (s *StatsService) Update(ctx context.Context, serverID string, containerID string) error {
	resp, err := s.docker.ContainerStats(ctx, containerID, client.ContainerStatsOptions{
		Stream:                false,
		IncludePreviousSample: true,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var stats struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
			OnlineCPUs     uint64 `json:"online_cpus"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
		} `json:"memory_stats"`
		Networks map[string]struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		} `json:"networks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return err
	}

	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage)
	cpuPercent := 0.0
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(stats.CPUStats.OnlineCPUs) * 100.0
	}

	var netRx, netTx uint64
	for _, net := range stats.Networks {
		netRx += net.RxBytes
		netTx += net.TxBytes
	}

	memPct := 0.0
	if stats.MemoryStats.Limit > 0 {
		memPct = float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}

	s.mu.Lock()
	s.cache[serverID] = &ContainerStats{
		ServerID:    serverID,
		CPUPercent:  cpuPercent,
		MemoryUsed:  stats.MemoryStats.Usage,
		MemoryLimit: stats.MemoryStats.Limit,
		MemoryPct:   memPct,
		NetRx:       netRx,
		NetTx:       netTx,
		UpdatedAt:   time.Now(),
	}
	s.mu.Unlock()

	return nil
}

func (s *StatsService) Remove(serverID string) {
	s.mu.Lock()
	delete(s.cache, serverID)
	s.mu.Unlock()
}
