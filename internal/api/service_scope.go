package api

import (
	"net/http"
	"strconv"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) findServiceByWatcherAndID(c *gin.Context, db *gorm.DB) (*database.Service, error) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return nil, err
	}

	sid, err := strconv.ParseUint(c.Param("sid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid service id"})
		return nil, err
	}

	var svc database.Service
	if err := db.Preload("ConfigFiles").Where("id = ? AND watcher_id = ?", sid, watcher.ID).First(&svc).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "service not found"})
		return nil, err
	}
	normalizeServiceCollections(&svc)
	return &svc, nil
}
