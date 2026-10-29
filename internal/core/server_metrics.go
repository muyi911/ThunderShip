package core

import (
	"context"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ServerCPU struct {
	Name   string `json:"name"`
	User   uint64 `json:"user"`
	System uint64 `json:"system"`
	Idle   uint64 `json:"idle"`
	IOWait uint64 `json:"iowait"`
	Total  uint64 `json:"total"`
}
type ServerMemory struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
	SwapTotal uint64 `json:"swapTotal"`
	SwapFree  uint64 `json:"swapFree"`
}
type ServerNetwork struct {
	Name      string   `json:"name"`
	Received  uint64   `json:"received"`
	Sent      uint64   `json:"sent"`
	Addresses []string `json:"addresses"`
}
type ServerDisk struct {
	Source    string `json:"source"`
	Mount     string `json:"mount"`
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
}
type ServerProcess struct {
	PID    int     `json:"pid"`
	Name   string  `json:"name"`
	CPU    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
}
type ServerMetrics struct {
	Timestamp int64           `json:"timestamp"`
	Hostname  string          `json:"hostname"`
	OS        string          `json:"os"`
	Uptime    float64         `json:"uptime"`
	CPUCount  int             `json:"cpuCount"`
	CPU       ServerCPU       `json:"cpu"`
	Cores     []ServerCPU     `json:"cores"`
	Memory    ServerMemory    `json:"memory"`
	Networks  []ServerNetwork `json:"networks"`
	Disks     []ServerDisk    `json:"disks"`
	Processes []ServerProcess `json:"processes"`
	Warnings  []string        `json:"warnings"`
}

var serverDiskRow = regexp.MustCompile(`^(.+?)\s+(\d+)\s+\d+\s+(\d+)\s+\d+%\s+(.+)$`)

// Values from os-release are parsed as data; never source a remote file into the shell.
const serverMetricsScript = `LC_ALL=C
export LC_ALL
if [ "$(uname -s)" != Linux ] || [ ! -r /proc/stat ] || [ ! -r /proc/meminfo ]; then
  printf '__THUNDERSHIP_UNSUPPORTED__\n'
  exit 1
fi
warn() { printf '\n__THUNDERSHIP_WARNING__%s\n' "$1"; }
printf '__THUNDERSHIP_HOST__\n'
cat /proc/sys/kernel/hostname 2>/dev/null || uname -n
printf '\n__THUNDERSHIP_OS__\n'
cat /etc/os-release 2>/dev/null || uname -sr
printf '\n__THUNDERSHIP_UPTIME__\n'
cat /proc/uptime
printf '\n__THUNDERSHIP_CPU__\n'
cat /proc/stat
printf '\n__THUNDERSHIP_MEMORY__\n'
cat /proc/meminfo
printf '\n__THUNDERSHIP_NETWORK__\n'
cat /proc/net/dev 2>/dev/null || warn network
printf '\n__THUNDERSHIP_ADDRESSES__\n'
ip -o addr show 2>/dev/null || warn addresses
printf '\n__THUNDERSHIP_PROCESSES__\n'
if processes=$(ps -eo pid=,pcpu=,rss=,comm= --sort=-pcpu 2>/dev/null); then
  printf '%s\n' "$processes" | head -n 100
elif processes=$(ps -eo pid,pcpu,rss,comm 2>/dev/null); then
  printf '%s\n' "$processes" | sort -k2,2nr | head -n 100
else
  warn processes
fi
printf '\n__THUNDERSHIP_DISKS__\n'
df -kP 2>/dev/null || warn disks
`

type metricsBuffer struct{ data []byte }

func (b *metricsBuffer) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 2*1024*1024 {
		return 0, fmt.Errorf("服务器信息超过 2 MB，已停止采集")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *metricsBuffer) String() string { return string(b.data) }

func (r *Remote) ReadServerMetrics(ctx context.Context) (ServerMetrics, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	session, err := r.newSession(ctx)
	if err != nil {
		return ServerMetrics{}, err
	}
	defer session.Close()
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()
	var output metricsBuffer
	session.Stdout, session.Stderr = &output, io.Discard
	err = session.Run("sh -c " + Quote(serverMetricsScript))
	if ctx.Err() != nil {
		return ServerMetrics{}, fmt.Errorf("服务器信息采集超时或已取消: %w", ctx.Err())
	}
	if strings.Contains(output.String(), "__THUNDERSHIP_UNSUPPORTED__") {
		return ServerMetrics{}, fmt.Errorf("服务器信息目前支持可读取 /proc 的 Linux 系统")
	}
	if err != nil {
		return ServerMetrics{}, fmt.Errorf("服务器信息采集失败: %w", err)
	}
	return parseServerMetrics(output.String(), time.Now().UnixMilli())
}

func parseServerMetrics(text string, timestamp int64) (ServerMetrics, error) {
	m := ServerMetrics{Timestamp: timestamp, Cores: []ServerCPU{}, Networks: []ServerNetwork{}, Disks: []ServerDisk{}, Processes: []ServerProcess{}, Warnings: []string{}}
	sections := map[string][]string{}
	section := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if warning, ok := strings.CutPrefix(line, "__THUNDERSHIP_WARNING__"); ok {
			message := map[string]string{"network": "无法读取网络流量", "addresses": "无法读取网络接口 IP（ip 命令不可用或权限不足）", "processes": "无法读取进程列表（ps 命令不可用或不支持）", "disks": "部分磁盘信息无法读取"}[warning]
			if message != "" {
				m.Warnings = append(m.Warnings, message)
			}
			continue
		}
		if strings.HasPrefix(line, "__THUNDERSHIP_") && strings.HasSuffix(line, "__") {
			section = strings.TrimSuffix(strings.TrimPrefix(line, "__THUNDERSHIP_"), "__")
		} else if line != "" {
			sections[section] = append(sections[section], line)
		}
	}
	m.Hostname = strings.Join(sections["HOST"], " ")
	for _, line := range sections["OS"] {
		if value, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			m.OS = strings.Trim(value, "\"'")
			break
		}
	}
	if m.OS == "" {
		m.OS = "Linux"
		for _, line := range sections["OS"] {
			if !strings.Contains(line, "=") {
				m.OS = line
				break
			}
		}
	}
	if rows := sections["UPTIME"]; len(rows) > 0 {
		m.Uptime, _ = strconv.ParseFloat(strings.Fields(rows[0])[0], 64)
	}
	for _, line := range sections["CPU"] {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		var ticks [8]uint64
		valid := true
		for i, field := range fields[1:min(len(fields), 9)] {
			n, err := strconv.ParseUint(field, 10, 64)
			ticks[i] = n
			valid = valid && err == nil
		}
		if !valid {
			continue
		}
		cpu := ServerCPU{Name: fields[0], User: ticks[0] + ticks[1], System: ticks[2] + ticks[5] + ticks[6], Idle: ticks[3], IOWait: ticks[4]}
		// Guest ticks are already included in user/nice and must not be counted twice.
		for _, n := range ticks {
			cpu.Total += n
		}
		if cpu.Name == "cpu" {
			m.CPU = cpu
		} else {
			m.Cores = append(m.Cores, cpu)
		}
	}
	m.CPUCount = len(m.Cores)
	memory := map[string]uint64{}
	for _, line := range sections["MEMORY"] {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err == nil {
				memory[strings.TrimSuffix(fields[0], ":")] = value * 1024
			}
		}
	}
	available, ok := memory["MemAvailable"]
	if !ok {
		available = memory["MemFree"] + memory["Buffers"] + memory["Cached"] + memory["SReclaimable"]
		available -= min(available, memory["Shmem"])
	}
	m.Memory = ServerMemory{Total: memory["MemTotal"], Available: min(available, memory["MemTotal"]), SwapTotal: memory["SwapTotal"], SwapFree: min(memory["SwapFree"], memory["SwapTotal"])}
	if m.CPU.Total == 0 || m.CPUCount == 0 || m.Memory.Total == 0 || m.Uptime <= 0 || math.IsNaN(m.Uptime) || math.IsInf(m.Uptime, 0) {
		return ServerMetrics{}, fmt.Errorf("服务器返回的 CPU、内存或运行时间信息不完整")
	}
	addresses := map[string][]string{}
	for _, line := range sections["ADDRESSES"] {
		fields := strings.Fields(line)
		if len(fields) >= 4 && (fields[2] == "inet" || fields[2] == "inet6") {
			name, _, _ := strings.Cut(fields[1], "@")
			addresses[name] = append(addresses[name], fields[3])
		}
	}
	for _, line := range sections["NETWORK"] {
		name, data, ok := strings.Cut(line, ":")
		fields := strings.Fields(data)
		if !ok || len(fields) < 16 {
			continue
		}
		rx, rxErr := strconv.ParseUint(fields[0], 10, 64)
		tx, txErr := strconv.ParseUint(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}
		name = strings.TrimSpace(name)
		ips := addresses[name]
		if ips == nil {
			ips = []string{}
		}
		m.Networks = append(m.Networks, ServerNetwork{Name: name, Received: rx, Sent: tx, Addresses: ips})
	}
	for _, line := range sections["DISKS"] {
		fields := serverDiskRow.FindStringSubmatch(line)
		if fields == nil {
			continue
		}
		total, totalErr := strconv.ParseUint(fields[2], 10, 64)
		available, availableErr := strconv.ParseUint(fields[3], 10, 64)
		if totalErr == nil && availableErr == nil && total > 0 {
			m.Disks = append(m.Disks, ServerDisk{Source: fields[1], Mount: fields[4], Total: total * 1024, Available: min(total, available) * 1024})
		}
	}
	for _, line := range sections["PROCESSES"] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		cpu, cpuErr := strconv.ParseFloat(fields[1], 64)
		memory, memoryErr := strconv.ParseUint(fields[2], 10, 64)
		if pidErr == nil && cpuErr == nil && memoryErr == nil && pid > 0 && cpu >= 0 && !math.IsInf(cpu, 0) {
			m.Processes = append(m.Processes, ServerProcess{PID: pid, Name: strings.Join(fields[3:], " "), CPU: cpu, Memory: memory * 1024})
		}
	}
	return m, nil
}
