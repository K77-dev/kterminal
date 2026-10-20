package rtk

import (
	"context"
	"os/exec"
	"testing"
)

func TestShouldRewrite(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"empty", "", false},
		{"simple", "git status", true},
		{"already rtk", "rtk git status", false},
		{"already rtk trimmed", "  rtk git status", false},
		{"just rtk", "rtk", false},
		{"pipe", "git status | head", false},
		{"ampersand", "go build &", false},
		{"semicolon", "echo a; echo b", false},
		{"redirect", "echo > file", false},
		{"backtick", "echo `date`", false},
		{"command sub", "echo $(date)", false},
		{"var sub", "echo ${HOME}", false},
		{"heredoc", "cat <<EOF", false},
		{"append", "echo >> file", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRewrite(tt.cmd); got != tt.want {
				t.Fatalf("shouldRewrite(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestHasLeadingRtk(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"rtk git status", true},
		{"  rtk git status", true},
		{"rtk", true},
		{"git status", false},
		{"rtkx", false},
	}
	for _, tt := range tests {
		if got := hasLeadingRtk(tt.cmd); got != tt.want {
			t.Fatalf("hasLeadingRtk(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestRewriteWithoutRtkReturnsOriginal(t *testing.T) {
	if _, err := exec.LookPath("rtk"); err == nil {
		t.Skip("rtk is installed on this machine; skipping fallback test")
	}
	original := "git status"
	got := Rewrite(context.Background(), original)
	if got != original {
		t.Fatalf("Rewrite without rtk installed = %q, want %q", got, original)
	}
}

func TestRewriteWithShellMetacharactersReturnsOriginal(t *testing.T) {
	original := "git status | head -5"
	got := Rewrite(context.Background(), original)
	if got != original {
		t.Fatalf("Rewrite with metacharacters = %q, want %q", got, original)
	}
}

func TestRewriteAlreadyPrefixedReturnsOriginal(t *testing.T) {
	original := "rtk git status"
	got := Rewrite(context.Background(), original)
	if got != original {
		t.Fatalf("Rewrite already prefixed = %q, want %q", got, original)
	}
}

func TestRewriteEmptyReturnsEmpty(t *testing.T) {
	got := Rewrite(context.Background(), "")
	if got != "" {
		t.Fatalf("Rewrite empty = %q, want empty", got)
	}
}

func TestDetectorAvailableCaches(t *testing.T) {
	var d Detector
	_ = d.Available()
	second := d.Available()
	if d.checked != true {
		t.Fatal("checked should be true after Available()")
	}
	_ = second
}

func TestCallRewriteTimeoutDoesNotHang(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, ok := callRewrite(ctx, "git status")
	if ok {
		t.Fatal("callRewrite with canceled context should return false")
	}
}

func TestHasShellMetacharacters(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"git status", false},
		{"a | b", true},
		{"a && b", true},
		{"a; b", true},
		{"a > b", true},
		{"a < b", true},
		{"a `b`", true},
		{"$(x)", true},
		{"${x}", true},
		{"<<EOF", true},
		{">> file", true},
	}
	for _, tt := range tests {
		if got := hasShellMetacharacters(tt.cmd); got != tt.want {
			t.Fatalf("hasShellMetacharacters(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}
