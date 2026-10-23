package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) ListAvailableVersions(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	versions, err := agent.ListAvailableVersions(watcher.InstallDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if versions == nil {
		versions = []agent.ReleaseInfo{}
	}
	currentVersion := strings.TrimSpace(watcher.CurrentVersion)
	if currentVersion != "" {
		for i := range versions {
			versions[i].IsCurrent = versions[i].Version == currentVersion
		}
	}

	downloads, err := h.catalogDownloads(watcher)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"downloads":       downloads,
		"watcher_id":      watcher.ID,
		"current_version": watcher.CurrentVersion,
		"versions":        versions,
	})
}

// RollbackWatcher rolls back a watcher to a specific version that exists on disk.
func (h *Handler) RollbackWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	if watcher.PendingCatalogID != 0 {
		catalogError(c, errCatalogConflict)
		return
	}

	var req RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	versions, err := agent.ListAvailableVersions(watcher.InstallDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	found := false
	for _, version := range versions {
		if version.Version == req.Version {
			found = true
			break
		}
	}
	if !found {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("version %s not found on disk", req.Version),
		})
		return
	}
	if err := agent.ValidateConfigSnapshot(watcher.InstallDir, req.Version); err != nil {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("version %s cannot be rolled back safely: %v", req.Version, err),
		})
		return
	}

	// Create deploy log first and process rollback asynchronously so API can return immediately.
	wcfg := agent.WatcherConfigFromDB(watcher)
	logger := h.log.WithWatcher(watcher.ID, wcfg.Name).WithTrace(agent.TraceFromContext(c.Request.Context()))

	now := time.Now().UTC()
	dlog := database.DeployLog{
		WatcherID:           watcher.ID,
		TriggeredBy:         "manual",
		Kind:                "rollback",
		Reason:              "manual_rollback",
		Version:             req.Version,
		FromVersion:         watcher.CurrentVersion,
		FailedTargetVersion: watcher.CurrentVersion,
		Status:              "in_progress",
		StartedAt:           &now,
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var live database.Watcher
		if err := tx.First(&live, watcher.ID).Error; err != nil {
			return err
		}
		if live.PendingCatalogID != 0 {
			return errCatalogConflict
		}
		var active int64
		if err := tx.Model(&database.DeployLog{}).Where("watcher_id = ? AND completed_at IS NULL", watcher.ID).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return errCatalogConflict
		}
		return tx.Create(&dlog).Error
	}); err != nil {
		catalogError(c, err)
		return
	}
	_ = h.db.Model(&dlog).Update("root_attempt_id", dlog.ID).Error
	if h.events != nil {
		h.events.Publish(watcher.ID, agent.WatcherEvent{
			Type: agent.EventDeployStarted,
			Data: map[string]any{
				"deploy_log_id": dlog.ID,
				"version":       req.Version,
				"from_version":  watcher.CurrentVersion,
				"triggered_by":  "manual",
			},
		})
	}

	reportGitHub := true
	if req.ReportGitHub != nil {
		reportGitHub = *req.ReportGitHub
	}
	if h.appCfg != nil && !h.appCfg.GitHubDeployEnabled {
		reportGitHub = false
	}

	_ = h.db.Model(watcher).Updates(map[string]any{
		"status": "deploying",
	})

	h.triggerSync()

	apiBaseURL := ""
	if h.appCfg != nil {
		apiBaseURL = h.appCfg.APIBaseURL
	}
	logURL := buildWatcherLogURL(apiBaseURL, watcher.ID, dlog.ID)
	go h.runRollback(watcher, dlog.ID, req.Version, watcher.CurrentVersion, reportGitHub, logger, wcfg)

	c.JSON(http.StatusAccepted, gin.H{
		"message":       fmt.Sprintf("rollback to %s started", req.Version),
		"version":       req.Version,
		"deploy_log_id": dlog.ID,
		"log_url":       logURL,
	})
}

func (h *Handler) runRollback(watcher *database.Watcher, deployLogID uint, targetVersion, previousVersion string, reportGitHub bool, logger *agent.Logger, wcfg *agent.WatcherConfig) {
	startedAt := time.Now().UTC()
	logger.Info("manual rollback started", "deploy_log_id", deployLogID, "target_version", targetVersion, "previous_version", previousVersion)
	appendRollbackLog := func(text string) {
		_ = h.db.Model(&database.DeployLog{}).Where("id = ?", deployLogID).
			UpdateColumn("logs", gorm.Expr("COALESCE(logs, '') || ?", text+"\n")).Error
	}

	deployer := agent.NewDeployer(wcfg, h.nssmPath, logger, appendRollbackLog)
	appendRollbackLog(fmt.Sprintf("rollback: started target=%s from=%s", targetVersion, previousVersion))

	rollbackErr := deployer.Rollback(context.Background(), targetVersion)
	if rollbackErr == nil {
		if _, err := agent.ReconcileConfigSnapshot(h.db, watcher.ID, targetVersion); err != nil {
			rollbackErr = fmt.Errorf("rollback activated %s but failed to reconcile persisted config: %w", targetVersion, err)
		}
	}
	if rollbackErr != nil {
		logger.Error("manual rollback failed", "deploy_log_id", deployLogID, "target_version", targetVersion, "error", rollbackErr)
		completed := time.Now().UTC()
		durationMs := completed.Sub(startedAt).Milliseconds()
		_ = h.db.Model(&database.DeployLog{}).Where("id = ?", deployLogID).Updates(map[string]any{
			"status":       "failed",
			"error":        rollbackErr.Error(),
			"completed_at": &completed,
			"duration_ms":  durationMs,
		}).Error
		_ = h.db.Model(&database.Watcher{}).Where("id = ?", watcher.ID).Updates(map[string]any{
			"status":     "failed",
			"last_error": rollbackErr.Error(),
		}).Error
		if h.events != nil {
			h.events.Publish(watcher.ID, agent.WatcherEvent{
				Type: agent.EventDeployFinished,
				Data: map[string]any{
					"deploy_log_id": deployLogID,
					"status":        "failed",
					"error":         rollbackErr.Error(),
				},
			})
		}
		var failedAttempt database.DeployLog
		if errFind := h.db.First(&failedAttempt, deployLogID).Error; errFind == nil && h.webhooks != nil {
			_ = h.webhooks.EmitAttemptEventTx(h.db, watcher, &failedAttempt)
		}
		h.triggerSync()
		return
	}

	logger.Info("manual rollback completed", "deploy_log_id", deployLogID, "target_version", targetVersion)
	completed := time.Now().UTC()
	durationMs := completed.Sub(startedAt).Milliseconds()
	maxIgnored := agent.RollbackHighWatermark(targetVersion, previousVersion)
	_ = h.db.Model(&database.DeployLog{}).Where("id = ?", deployLogID).Updates(map[string]any{
		"status":       "succeeded",
		"completed_at": &completed,
		"duration_ms":  durationMs,
	}).Error
	_ = h.db.Model(&database.Watcher{}).Where("id = ?", watcher.ID).Updates(map[string]any{
		"current_version":     targetVersion,
		"max_ignored_version": maxIgnored,
		"status":              "healthy",
		"last_deployed":       &completed,
		"last_error":          "",
	}).Error
	if h.events != nil {
		h.events.Publish(watcher.ID, agent.WatcherEvent{
			Type: agent.EventDeployFinished,
			Data: map[string]any{
				"deploy_log_id": deployLogID,
				"status":        "succeeded",
				"version":       targetVersion,
			},
		})
		h.events.Publish(watcher.ID, agent.WatcherEvent{
			Type: agent.EventVersionChanged,
			Data: map[string]any{
				"version": targetVersion,
			},
		})
	}
	var succeededAttempt database.DeployLog
	if errFind := h.db.First(&succeededAttempt, deployLogID).Error; errFind == nil && h.webhooks != nil {
		_ = h.webhooks.EmitAttemptEventTx(h.db, watcher, &succeededAttempt)
	}

	if reportGitHub {
		token := strings.TrimSpace(watcher.GitHubToken)
		if token == "" {
			token = strings.TrimSpace(h.githubToken)
		}
		env := strings.TrimSpace(watcher.DeploymentEnvironment)
		if env == "" && h.appCfg != nil {
			env = strings.TrimSpace(h.appCfg.Environment)
		}
		if token == "" || env == "" {
			appendRollbackLog("github_deployment: skipped for rollback (missing token or environment)")
		} else {
			owner, repo, deploymentRef, metadataSource, parseErr := resolveRollbackDeploymentTarget(watcher, targetVersion)
			if parseErr != nil {
				appendRollbackLog("github_deployment: rollback parse repo failed: " + parseErr.Error())
			} else {
				gh := agent.NewGitHubClient(token, logger)
				desc := fmt.Sprintf("Manual rollback %s to %s", watcher.ServiceName, targetVersion)
				appendRollbackLog(fmt.Sprintf("github_deployment: rollback create deployment repo=%s/%s ref=%s env=%q source=%s", owner, repo, deploymentRef, env, metadataSource))
				deploymentID, createErr := gh.CreateDeployment(context.Background(), owner, repo, deploymentRef, env, desc)
				if createErr != nil {
					appendRollbackLog("github_deployment: rollback create deployment failed: " + createErr.Error())
				} else {
					_ = h.db.Model(&database.DeployLog{}).Where("id = ?", deployLogID).Update("github_deployment_id", deploymentID).Error
					apiBaseURL := ""
					if h.appCfg != nil {
						apiBaseURL = h.appCfg.APIBaseURL
					}
					logURL := buildWatcherLogURL(apiBaseURL, watcher.ID, deployLogID)
					if statusErr := gh.UpdateDeploymentStatus(context.Background(), owner, repo, deploymentID, "success", logURL, desc); statusErr != nil {
						appendRollbackLog("github_deployment: rollback status=success failed: " + statusErr.Error())
					} else {
						appendRollbackLog(fmt.Sprintf("github_deployment: rollback status=success deployment_id=%d", deploymentID))
					}
				}
			}
		}
	} else {
		appendRollbackLog("github_deployment: skipped for rollback by request/config")
	}

	h.triggerSync()
}

// ResumeWatcherUpdates clears the max_ignored_version flag so polling updates resume.
func (h *Handler) ResumeWatcherUpdates(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	h.db.Model(watcher).Update("max_ignored_version", "")
	h.triggerSync()

	c.JSON(http.StatusOK, gin.H{"message": "auto-deploy resumed"})
}

// DeleteWatcherVersion removes a specific version directory from disk
func (h *Handler) DeleteWatcherVersion(c *gin.Context) {
	id := c.Param("id")
	version := c.Param("version")

	var watcher database.Watcher
	if err := h.db.First(&watcher, id).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "watcher not found"})
		return
	}

	if err := agent.DeleteVersion(watcher.InstallDir, version); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("deleted version %s", version),
		"version": version,
	})
}
