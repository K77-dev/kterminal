package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

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
