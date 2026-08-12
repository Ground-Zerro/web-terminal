package handlers

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

const (
	procStat = "/proc/stat"
	procMem  = "/proc/meminfo"
	procNet  = "/proc/net/dev"

	cpuFields = 8
	cpuIdle   = 3
	cpuIOWait = 4

	netRxField = 0
	netTxField = 8

	procBufSize = 8 << 10
)

var (
	cpuLabel     = []byte("cpu")
	loopbackName = []byte("lo")
	memTotalKey  = []byte("MemTotal:")
	memAvailKey  = []byte("MemAvailable:")
	preferredNet = [][]byte{[]byte("eth0"), []byte("ens"), []byte("enp")}
)

type MetricsHandler struct {
	mu   sync.Mutex
	proc procReader

	prevCPUTotal uint64
	prevCPUIdle  uint64

	prevRxBytes uint64
	prevTxBytes uint64
	prevNetTime time.Time
	prevNetName string
}

func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{}
}

type SystemMetrics struct {
	Response
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

func (h *MetricsHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	only(http.MethodGet, h.report)(w, r)
}

func (h *MetricsHandler) report(w http.ResponseWriter, r *http.Request) {
	metrics := SystemMetrics{Response: Response{Success: true}}
	metrics.CPU.Cores = runtime.NumCPU()

	h.mu.Lock()
	metrics.CPU.UsagePercent = h.readCPU()
	metrics.Memory.Total, metrics.Memory.Available, metrics.Memory.Used, metrics.Memory.UsedPercent = h.readMemory()
	metrics.Network.RxBytesPerSec, metrics.Network.TxBytesPerSec = h.readNetwork()
	h.mu.Unlock()

	writeJSON(w, http.StatusOK, metrics)
}

func (h *MetricsHandler) readCPU() float64 {
	data, err := h.proc.read(procStat)
	if err != nil {
		return 0
	}

	line, _ := nextLine(data)
	label, rest := nextField(line)
	if !bytes.Equal(label, cpuLabel) {
		return 0
	}

	var values [cpuFields]uint64
	for i := range values {
		field, remainder := nextField(rest)
		if len(field) == 0 {
			break
		}
		values[i] = parseUint(field)
		rest = remainder
	}

	var total uint64
	for _, v := range values {
		total += v
	}
	idle := values[cpuIdle] + values[cpuIOWait]

	prevTotal, prevIdle := h.prevCPUTotal, h.prevCPUIdle
	h.prevCPUTotal, h.prevCPUIdle = total, idle

	if prevTotal == 0 || total <= prevTotal {
		return 0
	}

	deltaTotal := total - prevTotal
	deltaIdle := idle - prevIdle
	if deltaIdle > deltaTotal {
		return 0
	}
	return float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
}

func (h *MetricsHandler) readMemory() (total, available, used uint64, usedPercent float64) {
	data, err := h.proc.read(procMem)
	if err != nil {
		return
	}

	for rest := data; len(rest) > 0; {
		var line []byte
		line, rest = nextLine(rest)

		switch {
		case bytes.HasPrefix(line, memTotalKey):
			total = kilobytesOf(line) * 1024
		case bytes.HasPrefix(line, memAvailKey):
			available = kilobytesOf(line) * 1024
		default:
			continue
		}

		if total > 0 && available > 0 {
			break
		}
	}

	if total > 0 && available <= total {
		used = total - available
		usedPercent = float64(used) / float64(total) * 100
	}
	return
}

func (h *MetricsHandler) readNetwork() (rxPerSec, txPerSec uint64) {
	data, err := h.proc.read(procNet)
	if err != nil {
		return
	}

	var (
		name       []byte
		rx, tx     uint64
		bestRank   = -1
		haveSample bool
	)

	for rest := data; len(rest) > 0; {
		var line []byte
		line, rest = nextLine(rest)

		colon := bytes.IndexByte(line, ':')
		if colon < 0 {
			continue
		}

		iface := bytes.TrimSpace(line[:colon])
		if len(iface) == 0 || bytes.Equal(iface, loopbackName) {
			continue
		}

		lineRx, lineTx, ok := netCounters(line[colon+1:])
		if !ok {
			continue
		}

		rank := netRank(iface)
		if rank <= bestRank {
			continue
		}

		name, rx, tx, bestRank, haveSample = iface, lineRx, lineTx, rank, true
		if rank == len(preferredNet) {
			break
		}
	}

	if !haveSample {
		return
	}

	now := time.Now()
	if h.prevNetName == string(name) && now.After(h.prevNetTime) {
		elapsed := now.Sub(h.prevNetTime).Seconds()
		if rx >= h.prevRxBytes && tx >= h.prevTxBytes {
			rxPerSec = uint64(float64(rx-h.prevRxBytes) / elapsed)
			txPerSec = uint64(float64(tx-h.prevTxBytes) / elapsed)
		}
	} else {
		h.prevNetName = string(name)
	}

	h.prevRxBytes, h.prevTxBytes, h.prevNetTime = rx, tx, now
	return
}

func netRank(iface []byte) int {
	for i, prefix := range preferredNet {
		if bytes.HasPrefix(iface, prefix) {
			return len(preferredNet) - i
		}
	}
	return 0
}

func netCounters(fields []byte) (rx, tx uint64, ok bool) {
	for i := 0; i <= netTxField; i++ {
		var field []byte
		field, fields = nextField(fields)
		if len(field) == 0 {
			return 0, 0, false
		}
		switch i {
		case netRxField:
			rx = parseUint(field)
		case netTxField:
			tx = parseUint(field)
		}
	}
	return rx, tx, true
}

func kilobytesOf(line []byte) uint64 {
	_, rest := nextField(line)
	value, _ := nextField(rest)
	return parseUint(value)
}

type procReader struct {
	buf []byte
}

func (p *procReader) read(name string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if p.buf == nil {
		p.buf = make([]byte, 0, procBufSize)
	}
	p.buf = p.buf[:0]

	for {
		if len(p.buf) == cap(p.buf) {
			p.buf = append(p.buf, 0)[:len(p.buf)]
		}
		n, err := f.Read(p.buf[len(p.buf):cap(p.buf)])
		p.buf = p.buf[:len(p.buf)+n]
		if err != nil {
			if err == io.EOF {
				return p.buf, nil
			}
			return nil, err
		}
	}
}

func nextLine(data []byte) (line, rest []byte) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return data[:i], data[i+1:]
	}
	return data, nil
}

func nextField(data []byte) (field, rest []byte) {
	start := 0
	for start < len(data) && isSpace(data[start]) {
		start++
	}
	end := start
	for end < len(data) && !isSpace(data[end]) {
		end++
	}
	return data[start:end], data[end:]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r'
}

func parseUint(field []byte) uint64 {
	var value uint64
	for _, c := range field {
		if c < '0' || c > '9' {
			return value
		}
		value = value*10 + uint64(c-'0')
	}
	return value
}
