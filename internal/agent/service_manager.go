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
			return serviceWaitContextError(name, []ServiceState{ServiceStateStopped}, "", ctxErr)
		}
		return err
	}
	switch state {
	case ServiceStateStopped:
		return nil
	case ServiceStateStopPending:
		return m.waitForState(transitionCtx, name, ServiceStateStopped)
	case ServiceStateStartPending:
		// SCM does not accept stop controls while start is pending. Wait for
		// startup to settle, then stop only if the service became running.
		settled, waitErr := m.waitForStates(
			transitionCtx,
			name,
			[]ServiceState{ServiceStateRunning, ServiceStateStopped},
			nil,
		)
		if waitErr != nil {
			return waitErr
		}
		if settled == ServiceStateStopped {
			return nil
		}
	}

	if err := m.runLifecycleCommand(
		transitionCtx,
		"stop",
		name,
		[]ServiceState{ServiceStateStopPending, ServiceStateStopped},
		"confirm",
	); err != nil {
		if ctxErr := transitionCtx.Err(); ctxErr != nil {
			return serviceWaitContextError(name, []ServiceState{ServiceStateStopped}, state, ctxErr)
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
			return serviceWaitContextError(name, []ServiceState{ServiceStateRunning}, "", ctxErr)
		}
		return err
	}
	action := "start"
	switch state {
	case ServiceStateRunning:
		return nil
	case ServiceStateStartPending, ServiceStateContinuePending:
		return m.waitForStateUntil(
			transitionCtx,
			name,
			ServiceStateRunning,
			ServiceStateStopped,
			ServiceStatePaused,
		)
	case ServiceStateStopPending:
		if err := m.waitForState(transitionCtx, name, ServiceStateStopped); err != nil {
			return err
		}
	case ServiceStatePausePending:
		settled, waitErr := m.waitForStates(
			transitionCtx,
			name,
			[]ServiceState{ServiceStatePaused, ServiceStateRunning},
			[]ServiceState{ServiceStateStopped},
		)
		if waitErr != nil {
			return waitErr
		}
		if settled == ServiceStateRunning {
			return nil
		}
		action = "continue"
	case ServiceStatePaused:
		action = "continue"
	}

	if err := m.runLifecycleCommand(
		transitionCtx,
		action,
		name,
		[]ServiceState{ServiceStateStartPending, ServiceStateContinuePending, ServiceStateRunning},
	); err != nil {
		if ctxErr := transitionCtx.Err(); ctxErr != nil {
			return serviceWaitContextError(name, []ServiceState{ServiceStateRunning}, state, ctxErr)
		}
		return err
	}
	return m.waitForStateUntil(
		transitionCtx,
		name,
		ServiceStateRunning,
		ServiceStateStopped,
		ServiceStatePaused,
	)
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

// runLifecycleCommand runs an NSSM lifecycle command and verifies the resulting service state.
func (m *NSSMServiceManager) runLifecycleCommand(
	ctx context.Context,
	action string,
	name string,
	acceptedStates []ServiceState,
	extra ...string,
) error {
	args := append([]string{action, name}, extra...)
	out, err := m.run(ctx, m.nssmPath, args...)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("nssm %s %s: %w", action, name, ctxErr)
	}
	if err == nil {
		return nil
	}

	text := strings.TrimSpace(string(out))
	if isAcceptedServiceState(parseServiceState(text), acceptedStates) {
		return nil
	}

	// NSSM can return a non-zero exit status after SCM accepted the control
	// request (for example while the service is SERVICE_START_PENDING). Query
	// SCM-backed status before deciding that the lifecycle command failed.
	state, statusErr := m.Status(ctx, name)
	if statusErr == nil && isAcceptedServiceState(state, acceptedStates) {
		return nil
	}

	commandErr := fmt.Errorf("nssm %s %s: %w (output: %s)", action, name, err, text)
	if statusErr != nil {
		return errors.Join(commandErr, fmt.Errorf("verify service %s state after %s: %w", name, action, statusErr))
	}
	return fmt.Errorf("%w (observed state: %s)", commandErr, displayServiceState(state))
}

// isAcceptedServiceState reports whether a service state belongs to the accepted set.
func isAcceptedServiceState(state ServiceState, accepted []ServiceState) bool {
	for _, candidate := range accepted {
		if state == candidate {
			return true
		}
	}
	return false
}

// waitForState waits until an NSSM service reaches one expected state.
func (m *NSSMServiceManager) waitForState(ctx context.Context, name string, expected ServiceState) error {
	_, err := m.waitForStates(ctx, name, []ServiceState{expected}, nil)
	return err
}

// waitForStateUntil waits until an NSSM service reaches one state before a fixed deadline.
func (m *NSSMServiceManager) waitForStateUntil(
	ctx context.Context,
	name string,
	expected ServiceState,
	terminalStates ...ServiceState,
) error {
	_, err := m.waitForStates(ctx, name, []ServiceState{expected}, terminalStates)
	return err
}

// waitForStates polls NSSM until a service reaches any accepted state or the context ends.
func (m *NSSMServiceManager) waitForStates(
	ctx context.Context,
	name string,
	expectedStates []ServiceState,
	terminalStates []ServiceState,
) (ServiceState, error) {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	lastState := ServiceState("")
	query := func() (bool, error) {
		state, err := m.Status(ctx, name)
		if err != nil {
			return false, err
		}
		lastState = state
		if isAcceptedServiceState(state, expectedStates) {
			return true, nil
		}
		if isAcceptedServiceState(state, terminalStates) {
			return false, fmt.Errorf(
				"service %s reached terminal state %s while waiting for %s",
				name,
				state,
				displayExpectedStates(expectedStates),
			)
		}
		return false, nil
	}

	if reached, err := query(); reached || err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", serviceWaitContextError(name, expectedStates, lastState, ctxErr)
		}
		return lastState, err
	}

	for {
		select {
		case <-ctx.Done():
			return "", serviceWaitContextError(name, expectedStates, lastState, ctx.Err())
		case <-ticker.C:
			if reached, err := query(); reached || err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return "", serviceWaitContextError(name, expectedStates, lastState, ctxErr)
				}
				return lastState, err
			}
		}
	}
}

// serviceWaitContextError wraps a cancelled or expired wait with the last observed service state.
func serviceWaitContextError(name string, expected []ServiceState, lastState ServiceState, err error) error {
	return fmt.Errorf("waiting for service %s to reach %s (last state: %s): %w", name, displayExpectedStates(expected), displayServiceState(lastState), err)
}

// displayExpectedStates formats a set of expected NSSM states for diagnostics.
func displayExpectedStates(states []ServiceState) string {
	if len(states) == 1 {
		return string(states[0])
	}
	parts := make([]string, 0, len(states))
	for _, state := range states {
		parts = append(parts, string(state))
	}
	return strings.Join(parts, " or ")
}

// displayServiceState formats an NSSM state for diagnostics.
func displayServiceState(state ServiceState) string {
	if state == "" {
		return "unknown"
	}
	return string(state)
}

// parseServiceState normalizes NSSM status output into a ServiceState.
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

// isServiceMissingOutput reports whether NSSM output means the service is not installed.
func isServiceMissingOutput(output string) bool {
	upper := strings.ToUpper(output)
	return strings.Contains(upper, "CAN'T OPEN SERVICE") ||
		strings.Contains(upper, "DOES NOT EXIST") ||
		strings.Contains(upper, "SERVICE_DOES_NOT_EXIST")
}
