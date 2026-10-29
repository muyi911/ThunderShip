package core

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

const serverMetricsFixture = `__THUNDERSHIP_HOST__
test-server
__THUNDERSHIP_OS__
NAME="CentOS Stream"
PRETTY_NAME="CentOS Stream 9"
__THUNDERSHIP_UPTIME__
172800.50 992200.00
__THUNDERSHIP_CPU__
cpu  100 10 20 400 5 3 2 1 50 4
cpu0 60 5 10 200 3 2 1 0 25 2
cpu1 40 5 10 200 2 1 1 1 25 2
intr 123 456
__THUNDERSHIP_MEMORY__
MemTotal: 16384 kB
MemFree: 1000 kB
MemAvailable: 8192 kB
Buffers: 100 kB
Cached: 500 kB
SReclaimable: 200 kB
Shmem: 50 kB
SwapTotal: 4096 kB
SwapFree: 2048 kB
__THUNDERSHIP_NETWORK__
Inter-| Receive | Transmit
 face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed
 eth0: 10000 100 0 0 0 0 0 0 20000 100 0 0 0 0 0 0
 lo: 500 10 0 0 0 0 0 0 500 10 0 0 0 0 0 0
__THUNDERSHIP_ADDRESSES__
2: eth0@if4 inet 192.168.1.2/24 brd 192.168.1.255 scope global eth0
2: eth0@if4 inet6 fe80::2/64 scope link
1: lo inet 127.0.0.1/8 scope host lo
__THUNDERSHIP_PROCESSES__
 PID %CPU RSS COMMAND
 123 121.5 262144 java
 456 7.8 2048 process with spaces
 789 NaN 20 broken
 790 +Inf 20 broken
__THUNDERSHIP_DISKS__
Filesystem 1024-blocks Used Available Capacity Mounted on
/dev/vda1 1024000 100000 924000 10% /
tmpfs 2048 1024 1024 50% /run/shared  memory
none 0 0 0 - /proc
`

func TestServerMetricsParser(t *testing.T) {
	m, err := parseServerMetrics(serverMetricsFixture, 1234)
	if err != nil {
		t.Fatal(err)
	}
	if m.Timestamp != 1234 || m.Hostname != "test-server" || m.OS != "CentOS Stream 9" || m.Uptime != 172800.5 {
		t.Fatalf("identity: %+v", m)
	}
	if m.CPUCount != 2 || m.CPU.Total != 541 || m.CPU.User != 110 || m.CPU.System != 25 || m.CPU.Idle != 400 || m.CPU.IOWait != 5 {
		t.Fatalf("cpu, including guest double-count protection: %+v", m.CPU)
	}
	if m.Memory != (ServerMemory{16384 * 1024, 8192 * 1024, 4096 * 1024, 2048 * 1024}) {
		t.Fatalf("memory: %+v", m.Memory)
	}
	if len(m.Networks) != 2 || m.Networks[0].Sent != 20000 || m.Networks[0].Received != 10000 || strings.Join(m.Networks[0].Addresses, ",") != "192.168.1.2/24,fe80::2/64" {
		t.Fatalf("network: %+v", m.Networks)
	}
	if len(m.Disks) != 2 || m.Disks[0].Available != 924000*1024 || m.Disks[1].Mount != "/run/shared  memory" {
		t.Fatalf("disks: %+v", m.Disks)
	}
	if len(m.Processes) != 2 || m.Processes[0].CPU != 121.5 || m.Processes[0].Memory != 262144*1024 || m.Processes[1].Name != "process with spaces" {
		t.Fatalf("processes: %+v", m.Processes)
	}
	if m.Warnings == nil || len(m.Warnings) != 0 {
		t.Fatal("expected non-null warnings", m.Warnings)
	}
}

func TestServerMetricsPartialAndInvalid(t *testing.T) {
	text := strings.ReplaceAll(serverMetricsFixture, "MemAvailable: 8192 kB\n", "")
	text = strings.Split(text, "__THUNDERSHIP_NETWORK__")[0] + "__THUNDERSHIP_WARNING__network\n__THUNDERSHIP_WARNING__addresses\n__THUNDERSHIP_WARNING__processes\n__THUNDERSHIP_WARNING__disks\n"
	m, err := parseServerMetrics(text, 1)
	if err != nil || m.Memory.Available != 1750*1024 || len(m.Warnings) != 4 || m.Networks == nil || m.Disks == nil || m.Processes == nil {
		t.Fatalf("partial metrics: %+v, %v", m, err)
	}
	for _, text := range []string{"", "cpu 10 10 10 10", strings.ReplaceAll(serverMetricsFixture, "MemTotal: 16384 kB", "MemTotal: broken kB"), strings.ReplaceAll(serverMetricsFixture, "172800.50", "NaN")} {
		if _, err := parseServerMetrics(text, 1); err == nil {
			t.Fatal("accepted incomplete mandatory metrics")
		}
	}
	var buffer metricsBuffer
	if _, err := io.Copy(&buffer, strings.NewReader(strings.Repeat("x", 2*1024*1024+1))); err == nil || len(buffer.data) != 0 {
		t.Fatal("output size not bounded", err)
	}
}

func TestServerMetricsUsesExistingTransportAndCancellation(t *testing.T) {
	var calls atomic.Int32
	peer := newReusePeer(t, &reusePeer{exec: func(ctx context.Context, channel ssh.Channel, command string) {
		if command != "sh -c "+Quote(serverMetricsScript) {
			t.Error("collector command changed unexpectedly")
			return
		}
		if calls.Add(1) == 2 {
			for {
				if _, err := io.WriteString(channel, "waiting\n"); err != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Millisecond):
				}
			}
		}
		io.WriteString(channel, serverMetricsFixture)
	}})
	r, err := DialSSH(context.Background(), peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.ReadServerMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := r.ReadServerMetrics(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected bounded cancellation", err)
	}
	if _, err := r.ReadServerMetrics(context.Background()); err != nil {
		t.Fatal("collector cancellation closed shared transport", err)
	}
	if peer.connections.Load() != 1 || peer.commands.Load() != 3 {
		t.Fatal("unexpected connection or command count", peer.connections.Load(), peer.commands.Load())
	}
}

func TestServerMetricsCollectorSyntaxAndUnsupportedSystem(t *testing.T) {
	cmd := exec.Command(commandShell(t), "-n")
	cmd.Stdin = strings.NewReader(serverMetricsScript)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("collector shell syntax: %v: %s", err, output)
	}
	peer := newReusePeer(t, &reusePeer{exitCode: 1, exec: func(_ context.Context, channel ssh.Channel, _ string) {
		io.WriteString(channel, "__THUNDERSHIP_UNSUPPORTED__\n")
	}})
	r, err := DialSSH(context.Background(), peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.ReadServerMetrics(context.Background()); err == nil || !strings.Contains(err.Error(), "Linux") {
		t.Fatal("unsupported system needs a useful error", err)
	}
}
