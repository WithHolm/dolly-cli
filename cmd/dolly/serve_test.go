package main

import (
	"net"
	"testing"
)

func TestReachable_AnAddressOnEveryInterfaceIsReachedAtLoopback(t *testing.T) {
	for addr, want := range map[string]string{
		"0.0.0.0:8650":   "127.0.0.1:8650",
		"[::]:8650":      "127.0.0.1:8650",
		"127.0.0.1:8650": "127.0.0.1:8650",
		"10.1.2.3:9000":  "10.1.2.3:9000",
	} {
		tcp, err := net.ResolveTCPAddr("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := reachable(tcp); got != want {
			t.Errorf("%s is reached at %s, want %s", addr, got, want)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1": true, "localhost": true, "LOCALHOST": true, "::1": true, "127.0.0.2": true,
		"0.0.0.0": false, "::": false, "10.0.0.5": false, "dolly": false, "": false,
	} {
		if got := isLoopback(host); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestEnvList(t *testing.T) {
	t.Setenv("DOLLY_TEST_LIST", " dolly, dolly.internal ,,x ")
	got := envList("DOLLY_TEST_LIST")
	if len(got) != 3 || got[0] != "dolly" || got[1] != "dolly.internal" || got[2] != "x" {
		t.Errorf("%q", got)
	}
	if got := envList("DOLLY_TEST_UNSET"); len(got) != 0 {
		t.Errorf("nothing set: %q", got)
	}
}
