package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterProviderPricingRoutes registers the public hvoy.ai provider pricing API.
func RegisterProviderPricingRoutes(r *gin.Engine, h *handler.Handlers) {
	r.GET("/api/provider/pricing", h.ProviderPricing.Get)
}
