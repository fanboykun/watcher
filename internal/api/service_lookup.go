package api

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func (h *Handler) findServiceByID(c *gin.Context) (*database.Service, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid service id"})
		return nil, err
	}

	var svc database.Service
	if err := h.db.Preload("ConfigFiles").First(&svc, id).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "service not found"})
		return nil, err
	}
	normalizeServiceCollections(&svc)
	return &svc, nil
}

func (h *Handler) requireWindows(c *gin.Context) error {
	if !h.runningOnWindows() {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error: fmt.Sprintf("NSSM service management only available on Windows (running on %s)", runtime.GOOS),
		})
		return fmt.Errorf("not windows")
	}
	return nil
}

func (h *Handler) runningOnWindows() bool {
	if h.isWindows != nil {
		return h.isWindows()
	}
	return runtime.GOOS == "windows"
}

// tailFile reads the last N lines from a file.
