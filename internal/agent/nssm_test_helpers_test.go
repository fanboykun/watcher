package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

type nssmCommandStep struct {
	args   string
	output string
	err    error
}

type scriptedNSSMRunner struct {
	t     *testing.T
	mu    sync.Mutex
	steps []nssmCommandStep
	calls []string
}

func newScriptedNSSMRunner(t *testing.T, steps ...nssmCommandStep) *scriptedNSSMRunner {
	t.Helper()
	return &scriptedNSSMRunner{t: t, steps: append([]nssmCommandStep(nil), steps...)}
}

func (r *scriptedNSSMRunner) run(ctx context.Context, _ string, args ...string) ([]byte, error) {
	r.t.Helper()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	call := strings.Join(args, " ")
	r.calls = append(r.calls, call)
	if len(r.steps) == 0 {
		r.t.Fatalf("unexpected NSSM command %q; script is exhausted", call)
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	if call != step.args {
		r.t.Fatalf("NSSM command = %q, want %q", call, step.args)
	}
	return []byte(step.output), step.err
}

func (r *scriptedNSSMRunner) assertDone() {
	r.t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.steps) != 0 {
		r.t.Fatalf("%d scripted NSSM commands were not called; next=%q", len(r.steps), r.steps[0].args)
	}
}

func stateStep(name string, state ServiceState) nssmCommandStep {
	return nssmCommandStep{args: fmt.Sprintf("status %s", name), output: string(state)}
}
