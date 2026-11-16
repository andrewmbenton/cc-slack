package main

import (
	"testing"
	"time"
)

func TestTheCallTallyNamesEachMethodsCallersBusiestFirst(t *testing.T) {
	began := time.Unix(1_800_000_000, 0)
	tally := &callTally{since: began, counts: map[string]map[string]int{}}
	for _, call := range []struct{ method, source string }{
		{"conversations.replies", "poller"},
		{"conversations.replies", "thread"},
		{"conversations.replies", "thread"},
		{"chat.update", "card sweep"},
	} {
		tally.add(call.method, call.source, began)
	}
	if got, want := tally.line(), "conversations.replies 3 (thread 2, poller 1); chat.update 1 (card sweep 1)"; got != want {
		t.Fatalf("line() = %q, want %q", got, want)
	}
	tally.add("chat.update", "dm-status", began.Add(tallyEvery))
	if got, want := tally.line(), "chat.update 1 (dm-status 1)"; got != want {
		t.Fatalf("line() after the window = %q, want %q", got, want)
	}
}
