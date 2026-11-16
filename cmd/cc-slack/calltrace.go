package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"maps"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yasyf/cc-interact/daemon"
)

const (
	slowOp     = 2 * time.Second
	tallyEvery = 10 * time.Minute
)

type traceCtxKey struct{}

type sourceCtxKey struct{}

type callTally struct {
	mu     sync.Mutex
	since  time.Time
	counts map[string]map[string]int
}

var slackCalls = &callTally{since: time.Now(), counts: map[string]map[string]int{}}

func withSource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, sourceCtxKey{}, source)
}

func callSource(ctx context.Context) string {
	if t, ok := ctx.Value(traceCtxKey{}).(*callTrace); ok {
		return string(t.op)
	}
	if source, ok := ctx.Value(sourceCtxKey{}).(string); ok {
		return source
	}
	if ctx.Value(sweepKey{}) != nil {
		return "card sweep"
	}
	return "background"
}

func (t *callTally) add(method, source string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if now.Sub(t.since) >= tallyEvery {
		log.Printf("slack calls in the last %s: %s", now.Sub(t.since).Round(time.Second), t.line())
		t.since, t.counts = now, map[string]map[string]int{}
	}
	if t.counts[method] == nil {
		t.counts[method] = map[string]int{}
	}
	t.counts[method][source]++
}

func (t *callTally) line() string {
	methods := slices.SortedFunc(maps.Keys(t.counts), func(a, b string) int { return cmp.Compare(total(t.counts[b]), total(t.counts[a])) })
	parts := make([]string, 0, len(methods))
	for _, method := range methods {
		sources := slices.SortedFunc(maps.Keys(t.counts[method]), func(a, b string) int { return cmp.Compare(t.counts[method][b], t.counts[method][a]) })
		by := make([]string, 0, len(sources))
		for _, source := range sources {
			by = append(by, fmt.Sprintf("%s %d", source, t.counts[method][source]))
		}
		parts = append(parts, fmt.Sprintf("%s %d (%s)", method, total(t.counts[method]), strings.Join(by, ", ")))
	}
	return strings.Join(parts, "; ")
}

func total(counts map[string]int) int {
	n := 0
	for _, count := range counts {
		n += count
	}
	return n
}

type callSpan struct {
	calls int
	took  time.Duration
}

type callTrace struct {
	mu     sync.Mutex
	op     daemon.Op
	began  time.Time
	posted time.Duration
	spans  map[string]*callSpan
	order  []string
}

func (t *callTrace) add(name string, took time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	span, seen := t.spans[name]
	if !seen {
		span = &callSpan{}
		t.spans[name] = span
		t.order = append(t.order, name)
	}
	span.calls++
	span.took += took
	if t.posted == 0 && (name == "chat.postMessage" || name == "chat.update") {
		t.posted = time.Since(t.began)
	}
}

func (t *callTrace) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.order) == 0 {
		return "no outbound calls"
	}
	parts := make([]string, 0, len(t.order))
	for _, name := range t.order {
		span := t.spans[name]
		parts = append(parts, fmt.Sprintf("%s x%d %s", name, span.calls, span.took.Round(10*time.Millisecond)))
	}
	if t.posted == 0 {
		return strings.Join(parts, ", ")
	}
	return fmt.Sprintf("first Slack write done at %s; %s", t.posted.Round(10*time.Millisecond), strings.Join(parts, ", "))
}

func traced(ctx context.Context, name string, took time.Duration) {
	if t, ok := ctx.Value(traceCtxKey{}).(*callTrace); ok {
		t.add(name, took)
	}
}

func timedOp(op daemon.Op, h daemon.HandlerFunc) daemon.HandlerFunc {
	return func(hc daemon.HandlerCtx) daemon.Reply {
		trace := &callTrace{op: op, began: time.Now(), spans: map[string]*callSpan{}}
		hc.Ctx = context.WithValue(hc.Ctx, traceCtxKey{}, trace)
		reply := h(hc)
		if took := time.Since(trace.began); took >= slowOp {
			log.Printf("slow op %s: %s, ok=%t; %s", op, took.Round(10*time.Millisecond), reply.OK, trace)
		}
		return reply
	}
}

type timedDoer struct{ next doer }

func (d timedDoer) Do(req *http.Request) (*http.Response, error) {
	began := time.Now()
	resp, err := d.next.Do(req) //nolint:gosec // G704: the caller built the request for Slack or the doc-cards URL the config names
	name := req.URL.Host + " " + path.Base(req.URL.Path)
	if strings.HasSuffix(req.URL.Host, "slack.com") {
		name = path.Base(req.URL.Path)
		slackCalls.add(name, callSource(req.Context()), time.Now())
	}
	traced(req.Context(), name, time.Since(began))
	return resp, err
}
