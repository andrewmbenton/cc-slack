package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"time"

	"github.com/yasyf/cc-interact/daemon"
)

const (
	cciBinary       = "cci"
	cciClaimTimeout = 30 * time.Second
)

var threadPostOps = []daemon.Op{opSend, opReply, opDMStatus}

type postedThread struct {
	ChannelID string `json:"channel_id"`
	ThreadTS  string `json:"thread_ts"`
}

func claimPostedThread(ctx context.Context, from origin, session string, op daemon.Op, body json.RawMessage) error {
	if session == "" || !slices.Contains(threadPostOps, op) {
		return nil
	}
	bin, err := exec.LookPath(cciBinary)
	if err != nil {
		return nil
	}
	var posted postedThread
	if err := json.Unmarshal(body, &posted); err != nil {
		return err
	}
	thread := posted.ChannelID + "/" + posted.ThreadTS
	ctx, cancel := context.WithTimeout(ctx, cciClaimTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, bin, "claim", "--thread", thread, "--rel", "posted") //nolint:gosec // G204: argv to the cci binary on PATH, never a shell
	c.Env, c.Dir = append(slices.Clip(from.env), "CLAUDE_CODE_SESSION_ID="+session), from.dir
	c.WaitDelay = time.Second
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("cci claim --thread %s: %w: %s", thread, err, clip(string(out)))
	}
	return nil
}
