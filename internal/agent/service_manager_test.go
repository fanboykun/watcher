package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func newTestServiceManager(run serviceCommandRunner) *NSSMServiceManager {
	manager := NewNSSMServiceManager(
		"nssm.exe",
		WithServiceTransitionTimeout(20*time.Millisecond),
		WithServicePollInterval(time.Millisecond),
	)
	manager.run = run
	return manager
}

func TestNSSMServiceManagerStopAlreadyStopped(t *testing.T) {
	var calls []string
	manager := newTestServiceManager(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return []byte(ServiceStateStopped), nil
	})

	if err := manager.Stop(context.Background(), "api"); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if got, want := strings.Join(calls, ","), "status api"; got != want {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

func TestNSSMServiceManagerStopPendingToStopped(t *testing.T) {
	statuses := []ServiceState{ServiceStateRunning, ServiceStateStopPending, ServiceStateStopped}
	var stopCalled bool
	manager := newTestServiceManager(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "status":
			state := statuses[0]
			statuses = statuses[1:]
			return []byte(state), nil
		case "stop":
			stopCalled = true
			return []byte("SERVICE_STOP_PENDING"), nil
		default:
			t.Fatalf("unexpected command: %v", args)
			return nil, nil
		}
	})

	if err := manager.Stop(context.Background(), "api"); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if !stopCalled {
		t.Fatal("expected nssm stop command")
	}
}

func TestNSSMServiceManagerStartPendingToRunning(t *testing.T) {
	statuses := []ServiceState{ServiceStateStopped, ServiceStateStartPending, ServiceStateRunning}
	var startCalled bool
	manager := newTestServiceManager(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "status":
			state := statuses[0]
			statuses = statuses[1:]
			return []byte(state), nil
		case "start":
			startCalled = true
			return []byte("SERVICE_START_PENDING"), nil
		default:
			t.Fatalf("unexpected command: %v", args)
			return nil, nil
		}
	})

	if err := manager.Start(context.Background(), "api"); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !startCalled {
		t.Fatal("expected nssm start command")
	}
}

func TestNSSMServiceManagerTimeoutIncludesServiceAndLastState(t *testing.T) {
	manager := newTestServiceManager(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "start" {
			return []byte("SERVICE_START_PENDING"), nil
		}
		if args[0] == "status" {
			return []byte(ServiceStateStartPending), nil
		}
		return nil, fmt.Errorf("unexpected command: %v", args)
	})

	err := manager.Start(context.Background(), "api")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	for _, text := range []string{"api", string(ServiceStateStartPending)} {
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("error %q does not contain %q", err, text)
		}
	}
}

func TestNSSMServiceManagerServiceMissing(t *testing.T) {
	manager := newTestServiceManager(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("OpenService(): The specified service does not exist as an installed service."), errors.New("exit status 3")
	})

	err := manager.Stop(context.Background(), "missing-api")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("Stop error = %v, want ErrServiceNotFound", err)
	}
}

func TestNSSMServiceManagerUnexpectedState(t *testing.T) {
	manager := newTestServiceManager(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("SERVICE_ZOMBIE"), nil
	})

	err := manager.Start(context.Background(), "api")
	if err == nil || !strings.Contains(err.Error(), "unexpected state") {
		t.Fatalf("Start error = %v, want unexpected state", err)
	}
}

func TestNSSMServiceManagerContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manager := newTestServiceManager(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		t.Fatal("command should not run after context cancellation")
		return nil, nil
	})

	err := manager.Stop(ctx, "api")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop error = %v, want context canceled", err)
	}
}

func TestNSSMServiceManagerStopCommandFailure(t *testing.T) {
	manager := newTestServiceManager(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "status" {
			return []byte(ServiceStateRunning), nil
		}
		return []byte("Access is denied"), errors.New("exit status 5")
	})

	err := manager.Stop(context.Background(), "api")
	if err == nil || !strings.Contains(err.Error(), "nssm stop api") {
		t.Fatalf("Stop error = %v, want command failure", err)
	}
}

func TestNSSMServiceManagerTimeoutBoundsLifecycleCommand(t *testing.T) {
	manager := NewNSSMServiceManager(
		"nssm.exe",
		WithServiceTransitionTimeout(10*time.Millisecond),
		WithServicePollInterval(time.Millisecond),
	)
	manager.run = func(ctx context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "status" {
			return []byte(ServiceStateRunning), nil
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}

	err := manager.Stop(context.Background(), "api")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error = %v, want context deadline exceeded", err)
	}
	if !strings.Contains(err.Error(), string(ServiceStateRunning)) {
		t.Fatalf("Stop error = %v, want last SERVICE_RUNNING state", err)
	}
}

func TestNSSMServiceManagerStartCommandFailure(t *testing.T) {
	manager := newTestServiceManager(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "status" {
			return []byte(ServiceStateStopped), nil
		}
		return []byte("The service cannot be started"), errors.New("exit status 1")
	})

	err := manager.Start(context.Background(), "api")
	if err == nil || !strings.Contains(err.Error(), "nssm start api") {
		t.Fatalf("Start error = %v, want command failure", err)
	}
}

func TestNSSMServiceManagerRestartStopsBeforeStart(t *testing.T) {
	statuses := []ServiceState{
		ServiceStateRunning,
		ServiceStateStopPending,
		ServiceStateStopped,
		ServiceStateStopped,
		ServiceStateStartPending,
		ServiceStateRunning,
	}
	var lifecycleCommands []string
	manager := newTestServiceManager(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "status" {
			state := statuses[0]
			statuses = statuses[1:]
			return []byte(state), nil
		}
		lifecycleCommands = append(lifecycleCommands, args[0])
		return []byte("ok"), nil
	})

	if err := manager.Restart(context.Background(), "api"); err != nil {
		t.Fatalf("Restart returned error: %v", err)
	}
	if got, want := strings.Join(lifecycleCommands, ","), "stop,start"; got != want {
		t.Fatalf("lifecycle commands = %q, want %q", got, want)
	}
}
