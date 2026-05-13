package handler

import (
	"log"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ProviderPricingHandler struct {
	service *service.ProviderPricingService
}

func NewProviderPricingHandler(service *service.ProviderPricingService) *ProviderPricingHandler {
	return &ProviderPricingHandler{service: service}
}

// Get returns the public provider pricing document.
// GET /api/provider/pricing
func (h *ProviderPricingHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusServiceUnavailable, service.ProviderPricingResponse{
			SchemaVersion: service.ProviderPricingSchemaVersion,
			Success:       false,
			Message:       "service temporarily unavailable",
		})
		return
	}

	pricing, err := h.service.GetProviderPricing(c.Request.Context())
	if err != nil {
		log.Printf("[ERROR] %s %s\n  Error: %s", c.Request.Method, c.Request.URL.Path, err.Error())
		c.JSON(http.StatusServiceUnavailable, service.ProviderPricingResponse{
			SchemaVersion: service.ProviderPricingSchemaVersion,
			Success:       false,
			Message:       "service temporarily unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, pricing)
}
