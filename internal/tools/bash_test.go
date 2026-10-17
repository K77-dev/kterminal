package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"kterminal/internal/procmon"
)

func TestBashWaitDelayReturnsDespiteBackgroundStraggler(t *testing.T) {
	orig := bashWaitDelay
	bashWaitDelay = 300 * time.Millisecond
	t.Cleanup(func() { bashWaitDelay = orig })

	reg := NewRegistry()
	start := time.Now()
	res, err := reg.Execute(context.Background(), "bash", `{"command":"sleep 5 & echo ready"}`)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "ready") {
		t.Fatalf("output = %q, want the shell output before the straggler", res.Output)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("elapsed = %s, want well under the 5s straggler sleep: WaitDelay must close the pipes after the shell exits", elapsed)
	}
}

func TestBashCanceledContextKillsLongProcess(t *testing.T) {
	reg := NewRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		reg.Execute(ctx, "bash", `{"command":"sleep 60"}`)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Execute returned before context cancel")
	case <-time.After(200 * time.Millisecond):
	}

	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Execute did not return within 1s of context cancel")
	}
}

func TestBashDeadlineExceededReturnsTimeoutError(t *testing.T) {
	reg := NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := reg.Execute(ctx, "bash", `{"command":"sleep 60"}`)

	if err == nil {
		t.Fatal("Execute succeeded, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), bashTimeout.String()) {
		t.Fatalf("err = %v, want timeout error mentioning %s", err, bashTimeout)
	}
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Fatalf("Execute took %v to time out, want prompt failure", elapsed)
	}
}

func TestExecuteStreamEmitsLinesInOrder(t *testing.T) {
	reg := NewRegistry()
	var lines []string
	onLine := func(line string) { lines = append(lines, line) }

	res, err := reg.ExecuteStream(context.Background(), "bash", `{"command":"for i in $(seq 1 10); do echo $i; sleep 0.05; done"}`, onLine)
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}

	want := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("streamed lines = %v, want %v", lines, want)
	}
	if res.Output != strings.Join(want, "\n")+"\n" {
		t.Fatalf("Output = %q, want 10 lines", res.Output)
	}
}

func TestExecuteStreamMergesStdoutStderr(t *testing.T) {
	reg := NewRegistry()
	var lines []string
	onLine := func(line string) { lines = append(lines, line) }

	res, err := reg.ExecuteStream(context.Background(), "bash", `{"command":"echo out1; echo err1 >&2; echo out2; echo err2 >&2"}`, onLine)
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}

	want := []string{"out1", "err1", "out2", "err2"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("streamed lines = %v, want %v", lines, want)
	}
	if res.Output != strings.Join(want, "\n")+"\n" {
		t.Fatalf("Output = %q, want merged stdout and stderr", res.Output)
	}
}

func TestExecuteStreamNilCallbackMatchesExecute(t *testing.T) {
	reg := NewRegistry()
	argsJSON := `{"command":"echo hello; echo world >&2; printf tail"}`

	viaStream, err := reg.ExecuteStream(context.Background(), "bash", argsJSON, nil)
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}
	viaExecute, err := reg.Execute(context.Background(), "bash", argsJSON)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !reflect.DeepEqual(viaStream, viaExecute) {
		t.Fatalf("ExecuteStream(nil) = %+v, want %+v", viaStream, viaExecute)
	}
}

func TestExecuteStreamFinalLineWithoutNewline(t *testing.T) {
	reg := NewRegistry()
	var lines []string
	onLine := func(line string) { lines = append(lines, line) }

	res, err := reg.ExecuteStream(context.Background(), "bash", `{"command":"echo full; printf partial"}`, onLine)
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}

	want := []string{"full", "partial"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("streamed lines = %v, want %v", lines, want)
	}
	if res.Output != "full\npartial" {
		t.Fatalf("Output = %q, want %q", res.Output, "full\npartial")
	}
}

func TestExecuteStreamTimeoutKillsProcess(t *testing.T) {
	reg := NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	var lines []string
	onLine := func(line string) { lines = append(lines, line) }

	start := time.Now()
	_, err := reg.ExecuteStream(ctx, "bash", `{"command":"sleep 300"}`, onLine)

	if err == nil {
		t.Fatal("ExecuteStream succeeded, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), bashTimeout.String()) {
		t.Fatalf("err = %v, want timeout error mentioning %s", err, bashTimeout)
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Fatalf("ExecuteStream took %v to time out, want killed process", elapsed)
	}
	if len(lines) != 0 {
		t.Fatalf("streamed lines = %v, want none", lines)
	}
}

func TestBashBackgroundReturnsImmediately(t *testing.T) {
	pm := procmon.New()
	t.Cleanup(pm.KillAll)
	reg := NewRegistry()
	reg.SetProcMon(pm)

	begin := time.Now()
	res, err := reg.Execute(context.Background(), "bash", `{"command":"sleep 5","run_in_background":true}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if elapsed := time.Since(begin); elapsed >= time.Second {
		t.Fatalf("Execute took %s, want <1s", elapsed)
	}
	if res.Output != "background process 1 started: sleep 5" {
		t.Fatalf("Output = %q, want %q", res.Output, "background process 1 started: sleep 5")
	}
	if pm.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", pm.Count())
	}
}

func TestBashBackgroundSpawnError(t *testing.T) {
	pm := procmon.New()
	t.Cleanup(pm.KillAll)
	reg := NewRegistry()
	reg.SetProcMon(pm)

	_, err := reg.Execute(context.Background(), "bash", `{"command":"exit 1","run_in_background":true}`)
	if err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("Execute error = %v, want exit status 1", err)
	}
	if pm.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", pm.Count())
	}
}

func TestBashBackgroundNilManager(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Execute(context.Background(), "bash", `{"command":"sleep 5","run_in_background":true}`)
	if err == nil || !strings.Contains(err.Error(), "background execution unavailable") {
		t.Fatalf("Execute error = %v, want background execution unavailable", err)
	}

	res, err := reg.Execute(context.Background(), "bash", `{"command":"echo sync ok"}`)
	if err != nil {
		t.Fatalf("sync Execute: %v", err)
	}
	if !strings.Contains(res.Output, "sync ok") {
		t.Fatalf("sync Output = %q, want echo output", res.Output)
	}
}
