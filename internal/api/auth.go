package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type authLoginRequest struct {
	Password string `json:"password" binding:"required"`
}

type authPasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

type authStatusResponse struct {
	Authenticated        bool `json:"authenticated"`
	UsingDefaultPassword bool `json:"using_default_password"`
}

// RequireAuth protects API routes with the single dashboard/API password.
func (h *Handler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		password, ok := bearerPassword(c.GetHeader("Authorization"))
		if !ok || !h.validateAuthPassword(password) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "authentication required"})
			return
		}
		c.Next()
	}
}

func (h *Handler) AuthLogin(c *gin.Context) {
	var req authLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if !h.validateAuthPassword(req.Password) {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid password"})
		return
	}

	cred, err := h.authCredential()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "auth credential is not available"})
		return
	}
	c.JSON(http.StatusOK, authStatusResponse{
		Authenticated:        true,
		UsingDefaultPassword: cred.UsingDefaultPassword,
	})
}

func (h *Handler) AuthStatus(c *gin.Context) {
	cred, err := h.authCredential()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "auth credential is not available"})
		return
	}
	c.JSON(http.StatusOK, authStatusResponse{
		Authenticated:        true,
		UsingDefaultPassword: cred.UsingDefaultPassword,
	})
}

func (h *Handler) UpdateAuthPassword(c *gin.Context) {
	var req authPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	currentPassword := strings.TrimSpace(req.CurrentPassword)
	newPassword := strings.TrimSpace(req.NewPassword)
	if newPassword == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "new_password cannot be empty"})
		return
	}
	if !h.validateAuthPassword(currentPassword) {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "current password is invalid"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to hash password"})
		return
	}

	cred, err := h.authCredential()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "auth credential is not available"})
		return
	}
	cred.PasswordHash = string(hash)
	cred.UsingDefaultPassword = false
	if err := h.db.Save(cred).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password updated", "using_default_password": false})
}

func (h *Handler) validateAuthPassword(password string) bool {
	password = strings.TrimSpace(password)
	if password == "" {
		return false
	}
	cred, err := h.authCredential()
	if err != nil || strings.TrimSpace(cred.PasswordHash) == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(password)) == nil
}

func (h *Handler) authCredential() (*database.AuthCredential, error) {
	var cred database.AuthCredential
	err := h.db.Order("id asc").First(&cred).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return &cred, nil
}

func bearerPassword(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	password := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return password, password != ""
}
