package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
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

func TestAToolCallTakesTheTicketItsHookKeptByToolUseIDOnce(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir, err := ticketsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(dir, "toolu_1.json")
	if err := os.WriteFile(kept, []byte(`{"ticket_id":"abc123","ticket_tree":"root","session":"s-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	logger := log.New(io.Discard, "", 0)
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slack_send","arguments":{"channel_id":"C1","text":"hi"},"_meta":{"claudecode/toolUseId":"toolu_1"}}}`
	got, err := ticketed([]byte(call), logger)
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Params struct {
			Arguments map[string]string `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal(got, &req); err != nil {
		t.Fatal(err)
	}
	if req.Params.Arguments["ticket_id"] != "abc123" || req.Params.Arguments["ticket_tree"] != "root" || req.Params.Arguments["text"] != "hi" {
		t.Fatalf("arguments %v, want the kept ticket merged into the typed ones", req.Params.Arguments)
	}
	if _, err := os.Stat(kept); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ticket file still there: %v", err)
	}
	again, err := ticketed([]byte(call), logger)
	if err != nil || !strings.Contains(string(again), `"ticket_unhooked":"slack_send toolu_1"`) {
		t.Fatalf("second call = %s, %v; want it marked unhooked with no file left", again, err)
	}
}
