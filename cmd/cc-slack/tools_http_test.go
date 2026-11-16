package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRegisteredWindow(t *testing.T) {
	pid := os.Getpid()
	tests := []struct {
		name    string
		entry   string
		want    registryEntry
		wantErr string
	}{
		{"live window", `{"pid":` + strconv.Itoa(pid) + `,"sessionId":"s-1","cwd":"/repo"}`, registryEntry{PID: pid, SessionID: "s-1", Cwd: "/repo"}, ""},
		{"no entry", "", registryEntry{}, "no Claude Code session registry entry"},
		{"another pid", `{"pid":1,"sessionId":"s-1","cwd":"/repo"}`, registryEntry{}, "is not a live window"},
		{"no session", `{"pid":` + strconv.Itoa(pid) + `,"cwd":"/repo"}`, registryEntry{}, "is not a live window"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "sessions"), 0o700); err != nil {
				t.Fatal(err)
			}
			if tt.entry != "" {
				if err := os.WriteFile(filepath.Join(dir, "sessions", strconv.Itoa(pid)+".json"), []byte(tt.entry), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := registeredWindow(dir, pid)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("registeredWindow() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("registeredWindow() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}
