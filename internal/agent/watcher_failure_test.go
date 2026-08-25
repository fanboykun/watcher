package agent

import (
	"errors"
	"testing"
)

func TestClassifyDeployFailureUsesRecoveryOutcome(t *testing.T) {
	cause := errors.New("start service api during deploy: service did not reach SERVICE_RUNNING")

	tests := []struct {
		name               string
		err                error
		wantOriginal       string
		wantRollbackTo     string
		wantRollbackFailed bool
	}{
		{
			name: "version rollback",
			err: &deployRecoveryError{
				cause:           cause,
				rollbackVersion: "v1.0.0",
			},
			wantOriginal:   cause.Error(),
			wantRollbackTo: "v1.0.0",
		},
		{
			name: "release backup restoration is not a rollback",
			err: &deployRecoveryError{
				cause:                 cause,
				restoredBackupVersion: "v1.1.0",
			},
			wantOriginal: "deploy failed, restored release backup for v1.1.0: " + cause.Error(),
		},
		{
			name: "rollback failure",
			err: &deployRecoveryError{
				cause:       cause,
				rollbackErr: errors.New("rollback service failed"),
			},
			wantOriginal:       "deploy failed AND rollback failed: deploy=" + cause.Error() + " rollback=rollback service failed",
			wantRollbackFailed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phase, originalErr, rollbackTo, rollbackFailed := classifyDeployFailure(tt.err)
			if phase != "start_services" {
				t.Fatalf("phase = %q, want start_services", phase)
			}
			if originalErr != tt.wantOriginal {
				t.Fatalf("original error = %q, want %q", originalErr, tt.wantOriginal)
			}
			if rollbackTo != tt.wantRollbackTo {
				t.Fatalf("rollbackTo = %q, want %q", rollbackTo, tt.wantRollbackTo)
			}
			if rollbackFailed != tt.wantRollbackFailed {
				t.Fatalf("rollbackFailed = %v, want %v", rollbackFailed, tt.wantRollbackFailed)
			}
		})
	}
}
