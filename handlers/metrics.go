package handlers

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MetricsHandler provides system performance metrics
type MetricsHandler struct {
	mu             sync.Mutex
	prevCpuTotal   uint64
	prevCpuIdle    uint64
	prevCpuTime    time.Time
	prevRxBytes    uint64
	prevTxBytes    uint64
	prevNetTime    time.Time
	prevNetIface   string
}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{}
}

// SystemMetrics represents the API response
type SystemMetrics struct {
	CPU struct {
		UsagePercent float64 `json:"usage_percent"`
		Cores        int     `json:"cores"`
	} `json:"cpu"`
	Memory struct {
		Total       uint64  `json:"total"`
		Available   uint64  `json:"available"`
		Used        uint64  `json:"used"`
		UsedPercent float64 `json:"used_percent"`
	} `json:"memory"`
	Network struct {
		RxBytesPerSec uint64 `json:"rx_bytes_per_sec"`
		TxBytesPerSec uint64 `json:"tx_bytes_per_sec"`
	} `json:"network"`
}

// GetMetrics returns current system metrics
func (h *MetricsHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := SystemMetrics{}

	// CPU
	cpuPercent, cores := h.getCPU()
	metrics.CPU.UsagePercent = cpuPercent
	metrics.CPU.Cores = cores

	// Memory
	total, available, used, usedPct := h.getMemory()
	metrics.Memory.Total = total
	metrics.Memory.Available = available
	metrics.Memory.Used = used
	metrics.Memory.UsedPercent = usedPct

	// Network
	rxPerSec, txPerSec := h.getNetwork()
	metrics.Network.RxBytesPerSec = rxPerSec
	metrics.Network.TxBytesPerSec = txPerSec

	writeJSON(w, http.StatusOK, metrics)
}

// getCPU reads /proc/stat and computes delta-based usage
func (h *MetricsHandler) getCPU() (float64, int) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, runtime.NumCPU()
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0, runtime.NumCPU()
	}

	// Parse first line: "cpu  user nice system idle iowait irq softirq steal"
	fields := strings.Fields(lines[0])
	if len(fields) < 5 {
		return 0, runtime.NumCPU()
	}

	var total uint64
	for i := 1; i < len(fields); i++ {
		v, _ := strconv.ParseUint(fields[i], 10, 64)
		total += v
	}

	idle, _ := strconv.ParseUint(fields[4], 10, 64)
	if len(fields) > 5 {
		iowait, _ := strconv.ParseUint(fields[5], 10, 64)
		idle += iowait
	}

	now := time.Now()

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.prevCpuTotal == 0 {
		h.prevCpuTotal = total
		h.prevCpuIdle = idle
		h.prevCpuTime = now
		return 0, runtime.NumCPU()
	}

	elapsed := now.Sub(h.prevCpuTime).Seconds()
	if elapsed <= 0 {
		return 0, runtime.NumCPU()
	}

	deltaTotal := total - h.prevCpuTotal
	deltaIdle := idle - h.prevCpuIdle

	h.prevCpuTotal = total
	h.prevCpuIdle = idle
	h.prevCpuTime = now

	if deltaTotal == 0 {
		return 0, runtime.NumCPU()
	}

	usage := float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100.0
	return usage, runtime.NumCPU()
}

// getMemory reads /proc/meminfo
func (h *MetricsHandler) getMemory() (total, available, used uint64, usedPct float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			total = parseMemInfoKB(line) * 1024
		} else if strings.HasPrefix(line, "MemAvailable:") {
			available = parseMemInfoKB(line) * 1024
		}
	}

	if total > 0 {
		used = total - available
		usedPct = float64(used) / float64(total) * 100.0
	}
	return
}

func parseMemInfoKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, _ := strconv.ParseUint(fields[1], 10, 64)
	return v
}

// getNetwork reads /proc/net/dev and computes throughput
func (h *MetricsHandler) getNetwork() (rxPerSec, txPerSec uint64) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "eth0") || strings.HasPrefix(line, "ens") {
			// Format: "eth0: rx_bytes ... tx_bytes ..."
			parts := strings.SplitN(line, ":", 2)
			if len(parts) < 2 {
				continue
			}

			iface := strings.TrimSpace(parts[0])
			fields := strings.Fields(parts[1])
			if len(fields) < 10 {
				continue
			}

			rxBytes, _ := strconv.ParseUint(fields[0], 10, 64) // rx_bytes
			txBytes, _ := strconv.ParseUint(fields[8], 10, 64) // tx_bytes

			now := time.Now()

			h.mu.Lock()
			if h.prevNetIface == iface && h.prevNetTime.Before(now) {
				elapsed := now.Sub(h.prevNetTime).Seconds()
				if elapsed > 0 {
					rxPerSec = uint64(float64(rxBytes-h.prevRxBytes) / elapsed)
					txPerSec = uint64(float64(txBytes-h.prevTxBytes) / elapsed)
				}
			}
			h.prevRxBytes = rxBytes
			h.prevTxBytes = txBytes
			h.prevNetTime = now
			h.prevNetIface = iface
			h.mu.Unlock()

			return
		}
	}

	// Fallback: try the first non-lo interface
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "lo") && !strings.HasPrefix(line, "Inter") && !strings.HasPrefix(line, "face") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) < 2 {
				continue
			}

			iface := strings.TrimSpace(parts[0])
			fields := strings.Fields(parts[1])
			if len(fields) < 10 {
				continue
			}

			rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
			txBytes, _ := strconv.ParseUint(fields[8], 10, 64)

			now := time.Now()

			h.mu.Lock()
			if h.prevNetIface == iface && h.prevNetTime.Before(now) {
				elapsed := now.Sub(h.prevNetTime).Seconds()
				if elapsed > 0 {
					rxPerSec = uint64(float64(rxBytes-h.prevRxBytes) / elapsed)
					txPerSec = uint64(float64(txBytes-h.prevTxBytes) / elapsed)
				}
			}
			h.prevRxBytes = rxBytes
			h.prevTxBytes = txBytes
			h.prevNetTime = now
			h.prevNetIface = iface
			h.mu.Unlock()

			return
		}
	}
	_ = fmt.Sprintf // suppress unused import
	return
}
