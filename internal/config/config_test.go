package config

import "testing"

func TestDefaultConfigUsesLoopback(t *testing.T) {
	cfg := DefaultConfig()
	if !IsLoopbackHost(cfg.Host) {
		t.Fatalf("desktop server must default to loopback, got %q", cfg.Host)
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1", "[::1]", "localhost", "LOCALHOST"} {
		if !IsLoopbackHost(host) {
			t.Errorf("expected %q to be loopback", host)
		}
	}
	for _, host := range []string{"", "0.0.0.0", "::", "192.168.1.2", "example.com"} {
		if IsLoopbackHost(host) {
			t.Errorf("did not expect %q to be loopback", host)
		}
	}
}
