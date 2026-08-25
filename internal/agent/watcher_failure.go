package agent

import (
	"errors"
	"strings"

	"github.com/fanboykun/watcher/internal/database"
)

// recordDeployFailure persists a failed deployment and any compensation attempt.
func (r *RepoWatcher) recordDeployFailure(err error, targetVersion, localVersion string) bool {
	phase, originalErr, rollbackTo, rollbackFailed := classifyDeployFailure(err)
	_ = r.state.SetFailedWithPhase(originalErr, phase)
	if rollbackTo != "" {
		deployAttemptID, rootAttemptID := r.latestDeployAttemptIDs()
		rollbackID, rbErr := r.state.StartRollbackAttempt(rollbackTo, targetVersion, targetVersion, "auto_after_deploy_failure", "agent", deployAttemptID, rootAttemptID)
		if rbErr == nil {
			_ = r.state.CompleteRollbackAttempt(rollbackID, rollbackTo, "")
		}
		return true
	}
	if rollbackFailed {
		deployAttemptID, rootAttemptID := r.latestDeployAttemptIDs()
		rollbackID, rbErr := r.state.StartRollbackAttempt(localVersion, targetVersion, targetVersion, "auto_after_deploy_failure", "agent", deployAttemptID, rootAttemptID)
		if rbErr == nil {
			_ = r.state.FailRollbackAttempt(rollbackID, err.Error())
		}
	}
	return false
}

// latestDeployAttemptIDs loads the latest deploy attempt identifiers for rollback lineage.
func (r *RepoWatcher) latestDeployAttemptIDs() (*uint, *uint) {
	var attempt database.DeployLog
	if err := r.db.Where("watcher_id = ? AND kind = ?", r.watcherID, "deploy").Order("id desc").First(&attempt).Error; err != nil {
		return nil, nil
	}
	parent := attempt.ID
	root := parent
	if attempt.RootAttemptID != nil && *attempt.RootAttemptID != 0 {
		root = *attempt.RootAttemptID
	}
	return &parent, &root
}

// classifyDeployFailure extracts the root failure and rollback outcome from a deployment error.
func classifyDeployFailure(err error) (phase string, originalErr string, rollbackTo string, rollbackFailed bool) {
	if err == nil {
		return "", "", "", false
	}
	msg := err.Error()
	originalErr = msg
	phaseSource := msg
	var recoveryErr *deployRecoveryError
	if errors.As(err, &recoveryErr) {
		phaseSource = recoveryErr.cause.Error()
		rollbackTo = recoveryErr.rollbackVersion
		rollbackFailed = recoveryErr.rollbackErr != nil
		if rollbackTo != "" {
			// A successful rollback gets its own attempt record; keep the failed
			// deployment's root cause focused on the target deployment.
			originalErr = phaseSource
		}
	}
	phase = inferFailurePhase(phaseSource)
	return phase, originalErr, rollbackTo, rollbackFailed
}

// inferFailurePhase infers the deployment phase represented by an error message.
func inferFailurePhase(msg string) string {
	switch {
	case strings.Contains(msg, "create install dir"):
		return "prepare"
	case strings.Contains(msg, "artifact_url missing"):
		return "prepare"
	case strings.Contains(msg, "create downloads dir"), strings.Contains(msg, "download artifact"):
		return "download"
	case strings.Contains(msg, "extract"):
		return "extract"
	case strings.Contains(msg, "stop "):
		return "stop_services"
	case strings.Contains(msg, "start "):
		return "start_services"
	case strings.Contains(msg, "health check failed"):
		return "health_check"
	case strings.Contains(msg, "swap"):
		return "activate_release"
	default:
		return ""
	}
}
