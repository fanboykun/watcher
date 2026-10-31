package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type catalogClient interface {
	ListReleaseCatalog(context.Context, string, int) (*agent.ReleaseCatalog, error)
	ResolveCatalogAsset(context.Context, *database.Watcher, int64, int64) (*agent.CatalogRelease, *agent.CatalogAsset, string, error)
	DownloadCatalogAsset(context.Context, string, int64, string, string) (string, error)
}

func (h *Handler) catalogClient(c *gin.Context, watcher *database.Watcher) (catalogClient, error) {
	token := strings.TrimSpace(watcher.GitHubToken)
	if token == "" {
		token = h.githubToken
	}
	log := h.log.WithWatcher(watcher.ID, watcher.Name).WithTrace(agent.TraceFromContext(c.Request.Context()))
	if h.catalogFactory != nil {
		return h.catalogFactory(token, log), nil
	}
	if !agent.IsGitHubSource(watcher.SourceType) {
		return agent.NewFeedCatalog(watcher, log)
	}
	return agent.NewGitHubClient(token, log), nil
}

func (h *Handler) ListCatalogReleases(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "page must be a positive integer"})
		return
	}
	client, err := h.catalogClient(c, watcher)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	catalog, err := client.ListReleaseCatalog(c.Request.Context(), watcher.MetadataURL, page)
	if err != nil {
		githubError(c, http.StatusBadGateway, err)
		return
	}
	artifacts, err := h.catalogDownloads(watcher)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	repository, _ := agent.CatalogRepository(watcher)
	c.JSON(http.StatusOK, gin.H{"releases": catalog.Releases, "page": catalog.Page, "has_next": catalog.HasNext, "downloads": artifacts, "repository": repository, "source_type": watcher.SourceType})
}

func (h *Handler) DownloadCatalogRelease(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	var req struct {
		ReleaseID int64 `json:"release_id" binding:"required,gt=0"`
		AssetID   int64 `json:"asset_id" binding:"required,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	client, err := h.catalogClient(c, watcher)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	release, asset, version, err := client.ResolveCatalogAsset(c.Request.Context(), watcher, req.ReleaseID, req.AssetID)
	if err != nil {
		githubError(c, http.StatusBadRequest, err)
		return
	}
	repository, err := agent.CatalogRepository(watcher)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	artifact := database.CatalogArtifact{WatcherID: watcher.ID, ServiceName: watcher.ServiceName, Repository: repository, InstallDir: watcher.InstallDir, ReleaseID: release.ID, AssetID: asset.ID, Tag: release.TagName, Version: version, AssetName: asset.Name, ArtifactURL: asset.BrowserDownloadURL}
	if err := h.db.Create(&artifact).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	path := agent.CatalogArtifactPath(&artifact)
	sum, err := client.DownloadCatalogAsset(c.Request.Context(), watcher.MetadataURL, asset.ID, path, asset.Digest)
	if err == nil {
		err = c.Request.Context().Err()
	}
	if err == nil {
		now := time.Now().UTC()
		artifact.SHA256, artifact.DownloadedAt = sum, &now
		err = h.db.WithContext(c.Request.Context()).Save(&artifact).Error
	}
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(path))
		_ = h.db.Delete(&artifact).Error
		githubError(c, http.StatusBadGateway, err)
		return
	}
	h.log.WithWatcher(watcher.ID, watcher.Name).WithTrace(agent.TraceFromContext(c.Request.Context())).Info("release artifact staged", "catalog_id", artifact.ID, "version", version, "asset", asset.Name)
	c.JSON(http.StatusCreated, artifact)
}

var errCatalogConflict = errors.New("another deployment or release candidate is active; finish or discard it first")

func (h *Handler) findCatalogArtifact(c *gin.Context, watcher *database.Watcher, id uint) (*database.CatalogArtifact, bool) {
	var artifact database.CatalogArtifact
	if err := h.db.Where("id = ? AND watcher_id = ?", id, watcher.ID).First(&artifact).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Downloaded release not found for this watcher"})
		return nil, false
	}
	if err := agent.ValidateCatalogArtifact(watcher, &artifact); err != nil {
		c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
		return nil, false
	}
	return &artifact, true
}

func catalogID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("cid"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid downloaded release ID"})
		return 0, false
	}
	return uint(id), true
}

func reserveCatalogCandidate(tx *gorm.DB, watcher *database.Watcher, artifact *database.CatalogArtifact) error {
	var live database.Watcher
	if err := tx.First(&live, watcher.ID).Error; err != nil {
		return err
	}
	var active int64
	if err := tx.Model(&database.DeployLog{}).Where("watcher_id = ? AND completed_at IS NULL", watcher.ID).Count(&active).Error; err != nil {
		return err
	}
	if live.SourceType != watcher.SourceType || live.MetadataURL != watcher.MetadataURL || live.InstallDir != watcher.InstallDir || live.ServiceName != watcher.ServiceName {
		return fmt.Errorf("watcher configuration changed; refresh the catalog")
	}
	if active != 0 || live.Status == "deploying" || live.Status == "approved" || (live.PendingVersion != "" && (live.PendingVersion != artifact.Version || (live.PendingCatalogID != 0 && live.PendingCatalogID != artifact.ID))) {
		return errCatalogConflict
	}
	return tx.Model(&live).UpdateColumns(map[string]any{"pending_catalog_id": artifact.ID, "pending_version": artifact.Version, "approved_version": "", "status": "pending_approval"}).Error
}

func (h *Handler) SelectCatalogCandidate(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	id, ok := catalogID(c)
	if !ok {
		return
	}
	artifact, ok := h.findCatalogArtifact(c, watcher, id)
	if !ok {
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error { return reserveCatalogCandidate(tx, watcher, artifact) }); err != nil {
		catalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Release selected; configure its services in Candidates", "version": artifact.Version, "catalog_id": artifact.ID})
}

func (h *Handler) DeployCatalogRelease(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	id, ok := catalogID(c)
	if !ok {
		return
	}
	h.queueCatalogDeploy(c, watcher, id)
}

func (h *Handler) queueCatalogDeploy(c *gin.Context, watcher *database.Watcher, id uint) {
	artifact, ok := h.findCatalogArtifact(c, watcher, id)
	if !ok {
		return
	}
	if h.checkTrigger == nil {
		h.pollQueueUnavailable(c)
		return
	}
	trace := agent.NewPollTrace(agent.TraceFromContext(c.Request.Context()).RequestID, "manual")
	now := time.Now().UTC()
	dlog := database.DeployLog{WatcherID: watcher.ID, TriggeredBy: "manual", Kind: "deploy", Reason: "catalog_release", Version: artifact.Version, FromVersion: watcher.CurrentVersion, Status: "in_progress", StartedAt: &now, Logs: fmt.Sprintf("catalog: queued %s (%s)", artifact.Tag, artifact.AssetName)}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := reserveCatalogCandidate(tx, watcher, artifact); err != nil {
			return err
		}
		if err := tx.Create(&dlog).Error; err != nil {
			return err
		}
		if err := tx.Model(&dlog).UpdateColumn("root_attempt_id", dlog.ID).Error; err != nil {
			return err
		}
		return tx.Model(watcher).UpdateColumns(map[string]any{"approved_version": artifact.Version, "status": "approved"}).Error
	})
	if err != nil {
		catalogError(c, err)
		return
	}
	select {
	case h.checkTrigger <- agent.CheckTrigger{WatcherID: watcher.ID, CatalogID: artifact.ID, DeployLogID: dlog.ID, Trace: trace}:
		c.Header("X-Poll-ID", trace.PollID)
		c.JSON(http.StatusAccepted, gin.H{"message": "Selected release queued for deployment", "deploy_log_id": dlog.ID, "version": artifact.Version, "catalog_id": artifact.ID, "poll_id": trace.PollID, "correlation_id": trace.CorrelationID})
	default:
		_ = h.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Delete(&dlog).Error; err != nil {
				return err
			}
			return tx.Model(watcher).Where("pending_catalog_id = ? AND status = ?", artifact.ID, "approved").UpdateColumns(map[string]any{"approved_version": "", "status": "pending_approval"}).Error
		})
		h.pollQueueUnavailable(c)
	}
}

func catalogError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, errCatalogConflict) {
		status = http.StatusConflict
	}
	c.JSON(status, ErrorResponse{Error: err.Error()})
}

func (h *Handler) catalogDownloads(watcher *database.Watcher) ([]database.CatalogArtifact, error) {
	artifacts := []database.CatalogArtifact{}
	repository, err := agent.CatalogRepository(watcher)
	// A source that names no catalog still supports retained versions.
	if err != nil {
		return artifacts, nil
	}
	err = h.db.Where("watcher_id = ? AND repository = ? AND install_dir = ? AND service_name = ? AND downloaded_at IS NOT NULL", watcher.ID, repository, watcher.InstallDir, watcher.ServiceName).Order("id desc").Find(&artifacts).Error
	if err != nil {
		return nil, err
	}
	available := artifacts[:0]
	for _, artifact := range artifacts {
		if info, err := os.Stat(agent.CatalogArtifactPath(&artifact)); err == nil && info.Mode().IsRegular() {
			available = append(available, artifact)
		}
	}
	return available, nil
}
