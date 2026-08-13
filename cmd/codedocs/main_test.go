package main

import (
	"net"
	"strings"
	"testing"

	"codedocs/internal/config"
)

func TestFindAvailableListenerWithEphemeralPort(t *testing.T) {
	listener, err := findAvailableListener("127.0.0.1", 0, 10)
	if err != nil {
		t.Fatalf("findAvailableListener failed: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port == 0 {
		t.Fatalf("expected an assigned TCP port, got %v", listener.Addr())
	}
}

func TestLocalServerURL(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{host: "0.0.0.0", want: "http://127.0.0.1:8080/app"},
		{host: "::", want: "http://127.0.0.1:8080/app"},
		{host: "::1", want: "http://[::1]:8080/app"},
	}

	for _, tt := range tests {
		cfg := &config.Config{Host: tt.host, Port: 8080, BasePath: "/app"}
		if got := localServerURL(cfg); got != tt.want {
			t.Errorf("localServerURL(%q) = %q, want %q", tt.host, got, tt.want)
		}
	}
}

func TestFindAvailableListenerReportsRange(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	_, err = findAvailableListener("127.0.0.1", port, 1)
	if err == nil || !strings.Contains(err.Error(), "no available port") {
		t.Fatalf("expected useful port conflict error, got %v", err)
	}
}
