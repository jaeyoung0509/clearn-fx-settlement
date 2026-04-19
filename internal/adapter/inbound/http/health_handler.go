package httpadapter

import (
	"context"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"

	"fx-settlement-lab/go-backend/internal/domain"
)

type ReadyChecker interface {
	Ping(context.Context) error
}

type HealthHandler struct {
	readyChecker ReadyChecker
}

func NewHealthHandler(readyChecker ReadyChecker) *HealthHandler {
	return &HealthHandler{readyChecker: readyChecker}
}

func (h *HealthHandler) RegisterRoutes(engine *gin.Engine) {
	engine.GET("/health", h.Health)
	engine.GET("/ready", h.Ready)
}

func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(stdhttp.StatusOK, gin.H{"status": "ok"})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	if err := h.readyChecker.Ping(c.Request.Context()); err != nil {
		_ = c.Error(domain.Internal("Database not ready", nil).WithCause(err))
		return
	}

	c.JSON(stdhttp.StatusOK, gin.H{"status": "ok"})
}
