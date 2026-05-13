//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProviderPricingService_GetProviderPricing_ExportsCNYPerMTok(t *testing.T) {
	inputPrice := 3e-6
	outputPrice := 15e-6
	cacheWritePrice := 3.75e-6
	cacheReadPrice := 0.3e-6
	perRequestPrice := 0.02
	channels := []Channel{{
		ID:       1,
		Name:     "main",
		Status:   StatusActive,
		GroupIDs: []int64{10, 20},
		ModelPricing: []ChannelModelPricing{
			{
				Platform:        PlatformAnthropic,
				Models:          []string{"claude-sonnet-4"},
				BillingMode:     BillingModeToken,
				InputPrice:      &inputPrice,
				OutputPrice:     &outputPrice,
				CacheWritePrice: &cacheWritePrice,
				CacheReadPrice:  &cacheReadPrice,
			},
			{
				Platform:        PlatformOpenAI,
				Models:          []string{"gpt-image-request"},
				BillingMode:     BillingModePerRequest,
				PerRequestPrice: &perRequestPrice,
			},
		},
	}}

	channelSvc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{
		activeGroups: []Group{
			{ID: 10, Name: "cc", Platform: PlatformAnthropic, RateMultiplier: 2.0, Status: StatusActive},
			{ID: 20, Name: "codex", Platform: PlatformOpenAI, RateMultiplier: 1.0, Status: StatusActive},
		},
	})
	repo := newMockSettingRepo()
	repo.data[SettingKeySiteName] = "Demo API"
	repo.data[SettingKeyAPIBaseURL] = "https://api.example.com/v1"
	repo.data[SettingBalanceRechargeMult] = "0.5"
	settingSvc := NewSettingService(repo, &config.Config{})
	paymentCfg := NewPaymentConfigService(nil, repo, nil)
	svc := NewProviderPricingService(channelSvc, settingSvc, paymentCfg)

	resp, err := svc.GetProviderPricing(context.Background())
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, ProviderPricingSchemaVersion, resp.SchemaVersion)
	require.NotNil(t, resp.Data)
	require.Equal(t, ProviderPricingCurrency, resp.Data.Currency)
	require.Equal(t, ProviderPricingUnit, resp.Data.PriceUnit)
	require.Equal(t, "Demo API", resp.Data.SiteName)
	require.Equal(t, "api.example.com", resp.Data.SiteDomain)
	require.Len(t, resp.Data.Models, 1, "per-request pricing is not token pricing and must not be exported")

	got := resp.Data.Models[0]
	require.Equal(t, "claude-sonnet-4", got.ModelName)
	require.Equal(t, "cc", got.GroupName)
	require.True(t, got.Enabled)
	require.InDelta(t, 12.0, got.InputPrice, 1e-12)
	require.NotNil(t, got.OutputPrice)
	require.InDelta(t, 60.0, *got.OutputPrice, 1e-12)
	require.NotNil(t, got.CacheInputPrice)
	require.InDelta(t, 1.2, *got.CacheInputPrice, 1e-12)
	require.NotNil(t, got.CacheCreatePrice)
	require.InDelta(t, 15.0, *got.CacheCreatePrice, 1e-12)
	require.Nil(t, got.CacheCreatePrice1h)
}

func TestProviderPricingService_GetProviderPricing_RequiresInputPrice(t *testing.T) {
	outputPrice := 15e-6
	channels := []Channel{{
		ID:       1,
		Name:     "main",
		Status:   StatusActive,
		GroupIDs: []int64{10},
		ModelPricing: []ChannelModelPricing{{
			Platform:    PlatformAnthropic,
			Models:      []string{"output-only"},
			BillingMode: BillingModeToken,
			OutputPrice: &outputPrice,
		}},
	}}

	channelSvc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{
		activeGroups: []Group{{ID: 10, Name: "cc", Platform: PlatformAnthropic, RateMultiplier: 1.0}},
	})
	svc := NewProviderPricingService(channelSvc, nil, nil)

	resp, err := svc.GetProviderPricing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp.Data)
	require.Empty(t, resp.Data.Models)
}

func TestProviderPricingService_GetProviderPricing_UsesGlobalPricingFallback(t *testing.T) {
	channels := []Channel{{
		ID:       1,
		Name:     "openai",
		Status:   StatusActive,
		GroupIDs: []int64{10},
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"gpt-test": "gpt-test"},
		},
	}}

	channelSvc := newAvailableChannelServiceWithPricing(channels, &stubGroupRepoForAvailable{
		activeGroups: []Group{{ID: 10, Name: "pro", Platform: PlatformOpenAI, RateMultiplier: 1.0}},
	}, &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-test": {InputCostPerToken: 2e-6, OutputCostPerToken: 10e-6},
		},
	})
	svc := NewProviderPricingService(channelSvc, nil, nil)

	resp, err := svc.GetProviderPricing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp.Data)
	require.Len(t, resp.Data.Models, 1)
	got := resp.Data.Models[0]
	require.Equal(t, "gpt-test", got.ModelName)
	require.Equal(t, "pro", got.GroupName)
	require.InDelta(t, 2.0, got.InputPrice, 1e-12)
	require.NotNil(t, got.OutputPrice)
	require.InDelta(t, 10.0, *got.OutputPrice, 1e-12)
}

func TestProviderPricingService_GetProviderPricing_UsesPlatformDefaultsWhenChannelHasNoModels(t *testing.T) {
	channels := []Channel{{
		ID:       1,
		Name:     "openai",
		Status:   StatusActive,
		GroupIDs: []int64{10},
	}}

	channelSvc := newAvailableChannelServiceWithPricing(channels, &stubGroupRepoForAvailable{
		activeGroups: []Group{{ID: 10, Name: "pro", Platform: PlatformOpenAI, RateMultiplier: 0.3}},
	}, &PricingService{})
	svc := NewProviderPricingService(channelSvc, nil, nil)

	resp, err := svc.GetProviderPricing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp.Data)

	var got *ProviderPricingModel
	for i := range resp.Data.Models {
		if resp.Data.Models[i].ModelName == "gpt-5.4" && resp.Data.Models[i].GroupName == "pro" {
			got = &resp.Data.Models[i]
			break
		}
	}
	require.NotNil(t, got)
	require.True(t, got.Enabled)
	require.InDelta(t, 0.75, got.InputPrice, 1e-12)
	require.NotNil(t, got.OutputPrice)
	require.InDelta(t, 4.5, *got.OutputPrice, 1e-12)
	require.NotNil(t, got.CacheInputPrice)
	require.InDelta(t, 0.075, *got.CacheInputPrice, 1e-12)
	require.Nil(t, got.CacheCreatePrice)
}

func TestProviderPricingService_GetProviderPricing_DividesByRechargeMultiplier(t *testing.T) {
	inputPrice := 2e-6
	channels := []Channel{{
		ID:       1,
		Name:     "openai",
		Status:   StatusActive,
		GroupIDs: []int64{10},
		ModelPricing: []ChannelModelPricing{{
			Platform:    PlatformOpenAI,
			Models:      []string{"gpt-test"},
			BillingMode: BillingModeToken,
			InputPrice:  &inputPrice,
		}},
	}}

	channelSvc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{
		activeGroups: []Group{{ID: 10, Name: "pro", Platform: PlatformOpenAI, RateMultiplier: 0.5}},
	})
	repo := newMockSettingRepo()
	repo.data[SettingBalanceRechargeMult] = "2"
	paymentCfg := NewPaymentConfigService(nil, repo, nil)
	svc := NewProviderPricingService(channelSvc, nil, paymentCfg)

	resp, err := svc.GetProviderPricing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp.Data)
	require.Len(t, resp.Data.Models, 1)
	require.InDelta(t, 0.5, resp.Data.Models[0].InputPrice, 1e-12)
}

func TestProviderPricingService_GetProviderPricing_UsesFrontendURLDomainFallback(t *testing.T) {
	inputPrice := 1e-6
	channels := []Channel{{
		ID:       1,
		Name:     "main",
		Status:   StatusActive,
		GroupIDs: []int64{10},
		ModelPricing: []ChannelModelPricing{{
			Platform:    PlatformAnthropic,
			Models:      []string{"claude-haiku"},
			BillingMode: BillingModeToken,
			InputPrice:  &inputPrice,
		}},
	}}

	channelSvc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{
		activeGroups: []Group{{ID: 10, Name: "cc", Platform: PlatformAnthropic, RateMultiplier: 1.0}},
	})
	repo := newMockSettingRepo()
	repo.data[SettingKeyFrontendURL] = "https://www.portal.example.com/app"
	settingSvc := NewSettingService(repo, nil)
	svc := NewProviderPricingService(channelSvc, settingSvc, nil)

	resp, err := svc.GetProviderPricing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp.Data)
	require.Equal(t, "portal.example.com", resp.Data.SiteDomain)
}

func TestProviderPricingService_GetProviderPricing_NegativeRateMultiplierIsFree(t *testing.T) {
	inputPrice := 1e-6
	channels := []Channel{{
		ID:       1,
		Name:     "main",
		Status:   StatusActive,
		GroupIDs: []int64{10},
		ModelPricing: []ChannelModelPricing{{
			Platform:    PlatformAnthropic,
			Models:      []string{"claude-free"},
			BillingMode: BillingModeToken,
			InputPrice:  &inputPrice,
		}},
	}}

	channelSvc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{
		activeGroups: []Group{{ID: 10, Name: "cc", Platform: PlatformAnthropic, RateMultiplier: -1}},
	})
	svc := NewProviderPricingService(channelSvc, nil, nil)

	resp, err := svc.GetProviderPricing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp.Data)
	require.Len(t, resp.Data.Models, 1)
	require.Zero(t, resp.Data.Models[0].InputPrice)
}

func TestExtractProviderPricingDomain(t *testing.T) {
	require.Equal(t, "example.com", extractProviderPricingDomain("https://www.example.com/api"))
	require.Equal(t, "api.example.com", extractProviderPricingDomain("https://api.example.com:8443/v1"))
	require.Empty(t, extractProviderPricingDomain("/relative"))
	require.Empty(t, extractProviderPricingDomain(""))
}
