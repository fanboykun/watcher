package agent

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultServiceTransitionTimeout = 30 * time.Second
	defaultServicePollInterval      = 350 * time.Millisecond
)

// ServiceState is the lifecycle state reported by the Windows Service Control Manager.
type ServiceState string

const (
	ServiceStateStopped         ServiceState = "SERVICE_STOPPED"
	ServiceStateStartPending    ServiceState = "SERVICE_START_PENDING"
	ServiceStateStopPending     ServiceState = "SERVICE_STOP_PENDING"
	ServiceStateRunning         ServiceState = "SERVICE_RUNNING"
	ServiceStateContinuePending ServiceState = "SERVICE_CONTINUE_PENDING"
	ServiceStatePausePending    ServiceState = "SERVICE_PAUSE_PENDING"
	ServiceStatePaused          ServiceState = "SERVICE_PAUSED"
)

// ErrServiceNotFound indicates that Windows SCM does not know the requested service.
var ErrServiceNotFound = errors.New("service not found")

// ServiceManager synchronizes service commands with states reported by Windows SCM.
type ServiceManager interface {
	Stop(ctx context.Context, name string) error
	Start(ctx context.Context, name string) error
	Restart(ctx context.Context, name string) error
	Status(ctx context.Context, name string) (ServiceState, error)
}

// NSSMServiceManagerOption customizes NSSM lifecycle synchronization.
type NSSMServiceManagerOption func(*NSSMServiceManager)

// WithServiceTransitionTimeout sets the maximum time to wait for a requested state.
func WithServiceTransitionTimeout(timeout time.Duration) NSSMServiceManagerOption {
	return func(manager *NSSMServiceManager) {
		if timeout > 0 {
			manager.transitionTimeout = timeout
		}
	}
}

// WithServicePollInterval sets how frequently SCM state is queried.
func WithServicePollInterval(interval time.Duration) NSSMServiceManagerOption {
	return func(manager *NSSMServiceManager) {
		if interval > 0 {
			manager.pollInterval = interval
		}
	}
}

type serviceCommandRunner func(ctx context.Context, command string, args ...string) ([]byte, error)

// NSSMServiceManager uses NSSM for commands and its SCM-backed status query for synchronization.
type NSSMServiceManager struct {
	nssmPath          string
	transitionTimeout time.Duration
	pollInterval      time.Duration
	run               serviceCommandRunner
}

var _ ServiceManager = (*NSSMServiceManager)(nil)

// NewNSSMServiceManager creates an NSSM manager with a 30-second transition timeout.
func NewNSSMServiceManager(nssmPath string, options ...NSSMServiceManagerOption) *NSSMServiceManager {
	manager := &NSSMServiceManager{
		nssmPath:          nssmPath,
		transitionTimeout: defaultServiceTransitionTimeout,
		pollInterval:      defaultServicePollInterval,
		run: func(ctx context.Context, command string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, command, args...).CombinedOutput()
		},
	}
	for _, option := range options {
		option(manager)
	}
	return manager
}

// Status returns the current SCM service state.
func (m *NSSMServiceManager) Status(ctx context.Context, name string) (ServiceState, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("query service %s status: %w", name, err)
	}

	out, err := m.run(ctx, m.nssmPath, "status", name)
	text := strings.TrimSpace(string(out))
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("query service %s status: %w", name, ctxErr)
	}
	if isServiceMissingOutput(text) {
		return "", fmt.Errorf("service %s: %w", name, ErrServiceNotFound)
	}
	if state := parseServiceState(text); state != "" {
		return state, nil
	}
	if err != nil {
		return "", fmt.Errorf("nssm status %s: %w (output: %s)", name, err, text)
	}
	return "", fmt.Errorf("service %s returned unexpected state (output: %s)", name, text)
}

// Stop returns only after SCM reports SERVICE_STOPPED.
func (m *NSSMServiceManager) Stop(ctx context.Context, name string) error {
	transitionCtx, cancel := context.WithTimeout(ctx, m.transitionTimeout)
	defer cancel()

	state, err := m.Status(transitionCtx, name)
	if err != nil {
		if ctxErr := transitionCtx.Err(); ctxErr != nil {
			return serviceWaitContextError(name, ServiceStateStopped, "", ctxErr)
		}
		return err
	}
	if state == ServiceStateStopped {
		return nil
	}

	if err := m.runLifecycleCommand(transitionCtx, "stop", name, "confirm"); err != nil {
		if ctxErr := transitionCtx.Err(); ctxErr != nil {
			return serviceWaitContextError(name, ServiceStateStopped, state, ctxErr)
		}
		return err
	}
	return m.waitForState(transitionCtx, name, ServiceStateStopped)
}

// Start returns only after SCM reports SERVICE_RUNNING.
func (m *NSSMServiceManager) Start(ctx context.Context, name string) error {
	transitionCtx, cancel := context.WithTimeout(ctx, m.transitionTimeout)
	defer cancel()

	state, err := m.Status(transitionCtx, name)
	if err != nil {
		if ctxErr := transitionCtx.Err(); ctxErr != nil {
			return serviceWaitContextError(name, ServiceStateRunning, "", ctxErr)
		}
		return err
	}
	if state == ServiceStateRunning {
		return nil
	}

	if err := m.runLifecycleCommand(transitionCtx, "start", name); err != nil {
		if ctxErr := transitionCtx.Err(); ctxErr != nil {
			return serviceWaitContextError(name, ServiceStateRunning, state, ctxErr)
		}
		return err
	}
	return m.waitForState(transitionCtx, name, ServiceStateRunning)
}

// Restart guarantees a completed stop before issuing start and waiting for running.
func (m *NSSMServiceManager) Restart(ctx context.Context, name string) error {
	if err := m.Stop(ctx, name); err != nil {
		return fmt.Errorf("restart service %s: %w", name, err)
	}
	if err := m.Start(ctx, name); err != nil {
		return fmt.Errorf("restart service %s: %w", name, err)
	}
	return nil
}

func (m *NSSMServiceManager) runLifecycleCommand(ctx context.Context, action, name string, extra ...string) error {
	args := append([]string{action, name}, extra...)
	out, err := m.run(ctx, m.nssmPath, args...)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("nssm %s %s: %w", action, name, ctxErr)
	}
	if err != nil {
		return fmt.Errorf("nssm %s %s: %w (output: %s)", action, name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *NSSMServiceManager) waitForState(ctx context.Context, name string, expected ServiceState) error {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	lastState := ServiceState("")
	query := func() error {
		state, err := m.Status(ctx, name)
		if err != nil {
			return err
		}
		lastState = state
		if state == expected {
			return nil
		}
		return errServiceStatePending
	}

	if err := query(); err != errServiceStatePending {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return serviceWaitContextError(name, expected, lastState, ctxErr)
		}
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return serviceWaitContextError(name, expected, lastState, ctx.Err())
		case <-ticker.C:
			if err := query(); err != errServiceStatePending {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return serviceWaitContextError(name, expected, lastState, ctxErr)
				}
				return err
			}
		}
	}
}

func serviceWaitContextError(name string, expected, lastState ServiceState, err error) error {
	return fmt.Errorf("waiting for service %s to reach %s (last state: %s): %w", name, expected, displayServiceState(lastState), err)
}

var errServiceStatePending = errors.New("service state transition pending")

func displayServiceState(state ServiceState) string {
	if state == "" {
		return "unknown"
	}
	return string(state)
}

func parseServiceState(output string) ServiceState {
	upper := strings.ToUpper(output)
	for _, state := range []ServiceState{
		ServiceStateContinuePending,
		ServiceStateStartPending,
		ServiceStateStopPending,
		ServiceStatePausePending,
		ServiceStateRunning,
		ServiceStateStopped,
		ServiceStatePaused,
	} {
		if strings.Contains(upper, string(state)) {
			return state
		}
	}
	return ""
}

func isServiceMissingOutput(output string) bool {
	upper := strings.ToUpper(output)
	return strings.Contains(upper, "CAN'T OPEN SERVICE") ||
		strings.Contains(upper, "DOES NOT EXIST") ||
		strings.Contains(upper, "SERVICE_DOES_NOT_EXIST")
}
