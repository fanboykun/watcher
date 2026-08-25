package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNSSMServiceManagerStatusMatrix(t *testing.T) {
	states := []ServiceState{
		ServiceStateStopped,
		ServiceStateStartPending,
		ServiceStateStopPending,
		ServiceStateRunning,
		ServiceStateContinuePending,
		ServiceStatePausePending,
		ServiceStatePaused,
	}
	for _, want := range states {
		t.Run(string(want), func(t *testing.T) {
			runner := newScriptedNSSMRunner(t, nssmCommandStep{
				args:   "status api",
				output: "api: " + string(want),
			})
			manager := newTestServiceManager(runner.run)
			got, err := manager.Status(context.Background(), "api")
			if err != nil || got != want {
				t.Fatalf("Status = %s, %v; want %s, nil", got, err, want)
			}
			runner.assertDone()
		})
	}

	t.Run("recognized state with non-zero exit", func(t *testing.T) {
		runner := newScriptedNSSMRunner(t, nssmCommandStep{
			args:   "status api",
			output: string(ServiceStateRunning),
			err:    errors.New("exit status 1"),
		})
		manager := newTestServiceManager(runner.run)
		got, err := manager.Status(context.Background(), "api")
		if err != nil || got != ServiceStateRunning {
			t.Fatalf("Status = %s, %v; want SERVICE_RUNNING, nil", got, err)
		}
		runner.assertDone()
	})

	missingOutputs := []string{
		"Can't open service api!",
		"The specified service does not exist as an installed service.",
		"SERVICE_DOES_NOT_EXIST",
	}
	for _, output := range missingOutputs {
		t.Run("missing "+output, func(t *testing.T) {
			runner := newScriptedNSSMRunner(t, nssmCommandStep{
				args: "status api", output: output, err: errors.New("exit status 3"),
			})
			manager := newTestServiceManager(runner.run)
			_, err := manager.Status(context.Background(), "api")
			if !errors.Is(err, ErrServiceNotFound) {
				t.Fatalf("Status error = %v, want ErrServiceNotFound", err)
			}
			runner.assertDone()
		})
	}

	tests := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{name: "unknown successful output", output: "SERVICE_ZOMBIE", want: "unexpected state"},
		{name: "empty successful output", output: "", want: "unexpected state"},
		{name: "access denied", output: "Access is denied", err: errors.New("exit status 5"), want: "Access is denied"},
		{name: "empty command failure", output: "", err: errors.New("exit status 1"), want: "exit status 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newScriptedNSSMRunner(t, nssmCommandStep{args: "status api", output: tt.output, err: tt.err})
			manager := newTestServiceManager(runner.run)
			_, err := manager.Status(context.Background(), "api")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Status error = %v, want text %q", err, tt.want)
			}
			runner.assertDone()
		})
	}
}

func TestNSSMServiceManagerStatusCancellationDuringQuery(t *testing.T) {
	manager := newTestServiceManager(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err := manager.Status(ctx, "api")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Status error = %v, want context deadline exceeded", err)
	}
}

func TestNSSMServiceManagerStartInitialStateMatrix(t *testing.T) {
	tests := []struct {
		name  string
		steps []nssmCommandStep
	}{
		{name: "running is no-op", steps: []nssmCommandStep{stateStep("api", ServiceStateRunning)}},
		{name: "stopped starts", steps: []nssmCommandStep{
			stateStep("api", ServiceStateStopped),
			{args: "start api", output: string(ServiceStateStartPending)},
			stateStep("api", ServiceStateStartPending),
			stateStep("api", ServiceStateRunning),
		}},
		{name: "start pending waits", steps: []nssmCommandStep{
			stateStep("api", ServiceStateStartPending),
			stateStep("api", ServiceStateStartPending),
			stateStep("api", ServiceStateRunning),
		}},
		{name: "continue pending waits", steps: []nssmCommandStep{
			stateStep("api", ServiceStateContinuePending),
			stateStep("api", ServiceStateContinuePending),
			stateStep("api", ServiceStateRunning),
		}},
		{name: "stop pending settles before start", steps: []nssmCommandStep{
			stateStep("api", ServiceStateStopPending),
			stateStep("api", ServiceStateStopPending),
			stateStep("api", ServiceStateStopped),
			{args: "start api", output: string(ServiceStateStartPending)},
			stateStep("api", ServiceStateStartPending),
			stateStep("api", ServiceStateRunning),
		}},
		{name: "paused continues", steps: []nssmCommandStep{
			stateStep("api", ServiceStatePaused),
			{args: "continue api", output: string(ServiceStateContinuePending)},
			stateStep("api", ServiceStateContinuePending),
			stateStep("api", ServiceStateRunning),
		}},
		{name: "pause pending settles before continue", steps: []nssmCommandStep{
			stateStep("api", ServiceStatePausePending),
			stateStep("api", ServiceStatePausePending),
			stateStep("api", ServiceStatePaused),
			{args: "continue api", output: string(ServiceStateContinuePending)},
			stateStep("api", ServiceStateContinuePending),
			stateStep("api", ServiceStateRunning),
		}},
		{name: "pause pending returns to running", steps: []nssmCommandStep{
			stateStep("api", ServiceStatePausePending),
			stateStep("api", ServiceStateRunning),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newScriptedNSSMRunner(t, tt.steps...)
			manager := newTestServiceManager(runner.run)
			if err := manager.Start(context.Background(), "api"); err != nil {
				t.Fatalf("Start returned error: %v", err)
			}
			runner.assertDone()
		})
	}
}

func TestNSSMServiceManagerStopInitialStateMatrix(t *testing.T) {
	tests := []struct {
		name  string
		steps []nssmCommandStep
	}{
		{name: "stopped is no-op", steps: []nssmCommandStep{stateStep("api", ServiceStateStopped)}},
		{name: "stop pending waits", steps: []nssmCommandStep{
			stateStep("api", ServiceStateStopPending),
			stateStep("api", ServiceStateStopPending),
			stateStep("api", ServiceStateStopped),
		}},
		{name: "running stops", steps: stopFromStateSteps(ServiceStateRunning)},
		{name: "paused stops", steps: stopFromStateSteps(ServiceStatePaused)},
		{name: "continue pending stops", steps: stopFromStateSteps(ServiceStateContinuePending)},
		{name: "pause pending stops", steps: stopFromStateSteps(ServiceStatePausePending)},
		{name: "start pending settles running before stop", steps: []nssmCommandStep{
			stateStep("api", ServiceStateStartPending),
			stateStep("api", ServiceStateStartPending),
			stateStep("api", ServiceStateRunning),
			{args: "stop api confirm", output: string(ServiceStateStopPending)},
			stateStep("api", ServiceStateStopPending),
			stateStep("api", ServiceStateStopped),
		}},
		{name: "start pending settles stopped", steps: []nssmCommandStep{
			stateStep("api", ServiceStateStartPending),
			stateStep("api", ServiceStateStopped),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newScriptedNSSMRunner(t, tt.steps...)
			manager := newTestServiceManager(runner.run)
			if err := manager.Stop(context.Background(), "api"); err != nil {
				t.Fatalf("Stop returned error: %v", err)
			}
			runner.assertDone()
		})
	}
}

func stopFromStateSteps(state ServiceState) []nssmCommandStep {
	return []nssmCommandStep{
		stateStep("api", state),
		{args: "stop api confirm", output: string(ServiceStateStopPending)},
		stateStep("api", ServiceStateStopPending),
		stateStep("api", ServiceStateStopped),
	}
}

func TestNSSMServiceManagerStartFailsOnTerminalState(t *testing.T) {
	runner := newScriptedNSSMRunner(t,
		stateStep("api", ServiceStateStartPending),
		stateStep("api", ServiceStateStartPending),
		stateStep("api", ServiceStateStopped),
	)
	manager := newTestServiceManager(runner.run)
	err := manager.Start(context.Background(), "api")
	if err == nil || !strings.Contains(err.Error(), "terminal state SERVICE_STOPPED") {
		t.Fatalf("Start error = %v, want terminal startup failure", err)
	}
	runner.assertDone()
}

func TestNSSMServiceManagerPollingStatusFailure(t *testing.T) {
	runner := newScriptedNSSMRunner(t,
		stateStep("api", ServiceStateStopped),
		nssmCommandStep{args: "start api", output: string(ServiceStateStartPending)},
		nssmCommandStep{args: "status api", output: "Access is denied", err: errors.New("exit status 5")},
	)
	manager := newTestServiceManager(runner.run)
	err := manager.Start(context.Background(), "api")
	if err == nil || !strings.Contains(err.Error(), "Access is denied") {
		t.Fatalf("Start error = %v, want status polling failure", err)
	}
	runner.assertDone()
}

func TestNSSMServiceManagerCommandFailureIncludesVerificationFailure(t *testing.T) {
	runner := newScriptedNSSMRunner(t,
		stateStep("api", ServiceStateStopped),
		nssmCommandStep{args: "start api", output: "control failed", err: errors.New("exit status 1")},
		nssmCommandStep{args: "status api", output: "Access is denied", err: errors.New("exit status 5")},
	)
	manager := newTestServiceManager(runner.run)
	err := manager.Start(context.Background(), "api")
	if err == nil || !strings.Contains(err.Error(), "nssm start api") || !strings.Contains(err.Error(), "verify service api state") {
		t.Fatalf("Start error = %v, want command and verification failures", err)
	}
	runner.assertDone()
}

func TestNSSMServiceManagerRestartFailureOrdering(t *testing.T) {
	t.Run("stop failure prevents start", func(t *testing.T) {
		runner := newScriptedNSSMRunner(t,
			stateStep("api", ServiceStateRunning),
			nssmCommandStep{args: "stop api confirm", output: "Access is denied", err: errors.New("exit status 5")},
			stateStep("api", ServiceStateRunning),
		)
		manager := newTestServiceManager(runner.run)
		if err := manager.Restart(context.Background(), "api"); err == nil {
			t.Fatal("Restart returned nil; want stop failure")
		}
		runner.assertDone()
	})

	t.Run("start failure follows confirmed stop", func(t *testing.T) {
		runner := newScriptedNSSMRunner(t,
			stateStep("api", ServiceStateRunning),
			nssmCommandStep{args: "stop api confirm", output: string(ServiceStateStopPending)},
			stateStep("api", ServiceStateStopped),
			stateStep("api", ServiceStateStopped),
			nssmCommandStep{args: "start api", output: "cannot start", err: errors.New("exit status 1")},
			stateStep("api", ServiceStateStopped),
		)
		manager := newTestServiceManager(runner.run)
		err := manager.Restart(context.Background(), "api")
		if err == nil || !strings.Contains(err.Error(), "nssm start api") {
			t.Fatalf("Restart error = %v, want start failure", err)
		}
		runner.assertDone()
	})
}
