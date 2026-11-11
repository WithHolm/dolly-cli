package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// run runs the program and gives its output. A command that does not stop (a server that started when it should not have) is killed after 15 seconds.
// It runs in an empty folder of its own, so a command that makes the default data folder makes it there.
func run(t *testing.T, bin string, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = t.TempDir()
	return cmd.CombinedOutput()
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "dolly")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return bin
}

// A command has the flags that it uses, and no others. The help of a command lists them, so a flag that does nothing is not there to be tried.
func TestFlags_EachCommandHasItsOwn(t *testing.T) {
	bin := buildBinary(t)
	for _, c := range []struct {
		args    []string
		has, no []string
	}{
		{[]string{"serve"}, []string{"--dir", "--port", "--host", "--allow-host", "--tls", "--tls-cert", "--users", "--seed", "--profile", "--page-size", "--throttle", "--usage-history", "--permissions", "--clone-from"}, nil},
		{[]string{"graph", "serve"}, []string{"--dir", "--port", "--users", "--clone-from"}, nil},
		{[]string{"graph", "generate"}, []string{"--dir", "--users", "--seed", "--profile", "--force"}, []string{"--port", "--host", "--tls", "--throttle", "--clone-from"}},
		{[]string{"graph", "show"}, []string{"--dir", "--port", "--tls"}, []string{"--users", "--seed", "--profile", "--host", "--throttle", "--clone-from"}},
		{[]string{"graph", "reset"}, []string{"--dir", "--port"}, []string{"--users", "--host"}},
		{[]string{"health"}, []string{"--dir", "--port", "--tls"}, []string{"--users", "--seed", "--host", "--throttle"}},
		{[]string{"tls", "remove"}, []string{"--dir"}, []string{"--port", "--users", "--tls-cert"}},
		{[]string{"profile", "default"}, nil, []string{"--dir", "--port", "--users"}},
		{[]string{"mcp"}, nil, []string{"--dir", "--port", "--users"}},
	} {
		out, err := run(t, bin, append(c.args, "--help")...)
		if err != nil {
			t.Errorf("%v --help: %v\n%s", c.args, err, out)
			continue
		}
		for _, f := range c.has {
			if !strings.Contains(string(out), f+" ") && !strings.Contains(string(out), f+"\n") {
				t.Errorf("%v must have %s", c.args, f)
			}
		}
		for _, f := range c.no {
			if strings.Contains(string(out), f+" ") || strings.Contains(string(out), f+"\n") {
				t.Errorf("%v must not have %s", c.args, f)
			}
		}
	}
}

// A flag that a command does not have is refused with the name of the flag. (A flag before the command goes to the command, so
// 'dolly --dir x graph generate' works as it did.)
func TestFlags_NotForThisCommandIsAnError(t *testing.T) {
	bin := buildBinary(t)
	for _, args := range [][]string{
		{"health", "--users", "5"},
		{"graph", "generate", "--port", "9000"},
		{"--users", "5", "health"},
	} {
		out, err := run(t, bin, args...)
		if err == nil || !strings.Contains(string(out), "unknown flag") {
			t.Errorf("%v: want unknown flag: %v\n%s", args, err, out)
		}
	}
}
