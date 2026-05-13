//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProviderPricingHandler_UnavailableShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewProviderPricingHandler(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/provider/pricing", nil)

	h.Get(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp service.ProviderPricingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, service.ProviderPricingSchemaVersion, resp.SchemaVersion)
	require.False(t, resp.Success)
	require.Equal(t, "service temporarily unavailable", resp.Message)
	require.Nil(t, resp.Data)
}
