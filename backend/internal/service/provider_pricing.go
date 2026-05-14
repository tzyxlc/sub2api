package service

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const (
	ProviderPricingSchemaVersion = "1.0"
	ProviderPricingCurrency      = "CNY"
	ProviderPricingUnit          = "per_1m_tokens"

	tokensPerMillion = 1_000_000
)

// ProviderPricingResponse follows the hvoy.ai provider pricing schema.
type ProviderPricingResponse struct {
	SchemaVersion string               `json:"schema_version"`
	Success       bool                 `json:"success"`
	Message       string               `json:"message"`
	Data          *ProviderPricingData `json:"data,omitempty"`
}

type ProviderPricingData struct {
	Currency   string                 `json:"currency"`
	PriceUnit  string                 `json:"price_unit"`
	SiteName   string                 `json:"site_name,omitempty"`
	SiteDomain string                 `json:"site_domain,omitempty"`
	UpdatedAt  string                 `json:"updated_at"`
	Models     []ProviderPricingModel `json:"models"`
}

type ProviderPricingModel struct {
	ModelName          string   `json:"model_name"`
	GroupName          string   `json:"group_name"`
	InputPrice         float64  `json:"input_price"`
	OutputPrice        *float64 `json:"output_price"`
	CacheInputPrice    *float64 `json:"cache_input_price"`
	CacheCreatePrice   *float64 `json:"cache_create_price"`
	CacheCreatePrice1h *float64 `json:"cache_create_price_1h"`
	Enabled            bool     `json:"enabled"`
	Note               string   `json:"note"`
}

type ProviderPricingService struct {
	channelService *ChannelService
	settingService *SettingService
	paymentConfig  *PaymentConfigService
}

func NewProviderPricingService(channelService *ChannelService, settingService *SettingService, paymentConfig *PaymentConfigService) *ProviderPricingService {
	return &ProviderPricingService{
		channelService: channelService,
		settingService: settingService,
		paymentConfig:  paymentConfig,
	}
}

// GetProviderPricing exports current active channel prices as CNY / 1M tokens.
func (s *ProviderPricingService) GetProviderPricing(ctx context.Context) (*ProviderPricingResponse, error) {
	if s == nil || s.channelService == nil {
		return nil, fmt.Errorf("provider pricing service is not ready")
	}

	channels, err := s.channelService.ListAvailable(ctx)
	if err != nil {
		return nil, err
	}

	rechargeMultiplier := s.balanceRechargeMultiplier(ctx)
	siteName, siteDomain := s.siteMetadata(ctx)
	models := make([]ProviderPricingModel, 0)
	for _, ch := range channels {
		for _, group := range ch.Groups {
			if group.Name == "" || group.IsExclusive {
				continue
			}
			enabled := ch.Status == StatusActive
			rateMultiplier := group.RateMultiplier
			if rateMultiplier < 0 {
				rateMultiplier = 0
			}
			for _, model := range s.supportedModelsForProviderPricing(ch.SupportedModels, group.Platform) {
				if model.Platform != group.Platform || model.Pricing == nil {
					continue
				}
				pricing := providerPricingFromModel(model.Name, group.Name, model.Pricing, rateMultiplier, rechargeMultiplier, enabled)
				if pricing == nil {
					continue
				}
				models = append(models, *pricing)
			}
		}
	}

	sort.SliceStable(models, func(i, j int) bool {
		if models[i].GroupName != models[j].GroupName {
			return models[i].GroupName < models[j].GroupName
		}
		return models[i].ModelName < models[j].ModelName
	})

	return &ProviderPricingResponse{
		SchemaVersion: ProviderPricingSchemaVersion,
		Success:       true,
		Message:       "",
		Data: &ProviderPricingData{
			Currency:   ProviderPricingCurrency,
			PriceUnit:  ProviderPricingUnit,
			SiteName:   siteName,
			SiteDomain: siteDomain,
			UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
			Models:     models,
		},
	}, nil
}

func (s *ProviderPricingService) supportedModelsForProviderPricing(models []SupportedModel, platform string) []SupportedModel {
	if hasSupportedModelForPlatform(models, platform) {
		return models
	}
	return s.defaultSupportedModelsForPlatform(platform)
}

func hasSupportedModelForPlatform(models []SupportedModel, platform string) bool {
	for _, model := range models {
		if model.Platform == platform {
			return true
		}
	}
	return false
}

func (s *ProviderPricingService) defaultSupportedModelsForPlatform(platform string) []SupportedModel {
	if s == nil || s.channelService == nil || s.channelService.pricingService == nil {
		return nil
	}

	modelIDs := defaultProviderPricingModelIDs(platform)
	if len(modelIDs) == 0 {
		return nil
	}

	out := make([]SupportedModel, 0, len(modelIDs))
	seen := make(map[string]struct{}, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		lower := strings.ToLower(modelID)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}

		lp := s.channelService.pricingService.GetModelPricing(modelID)
		if lp == nil {
			continue
		}
		pricing := synthesizePricingFromLiteLLM(lp)
		if pricing == nil {
			continue
		}
		out = append(out, SupportedModel{
			Name:          modelID,
			Platform:      platform,
			Pricing:       pricing,
			PricingSource: SupportedModelPricingSourceGlobalFallback,
		})
	}
	return out
}

func defaultProviderPricingModelIDs(platform string) []string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformOpenAI:
		return openai.DefaultModelIDs()
	case PlatformAnthropic:
		return claude.DefaultModelIDs()
	case PlatformGemini:
		ids := make([]string, len(geminicli.DefaultModels))
		for i, model := range geminicli.DefaultModels {
			ids[i] = model.ID
		}
		return ids
	case PlatformAntigravity:
		models := antigravity.DefaultModels()
		ids := make([]string, len(models))
		for i, model := range models {
			ids[i] = model.ID
		}
		return ids
	default:
		return nil
	}
}

func (s *ProviderPricingService) balanceRechargeMultiplier(ctx context.Context) float64 {
	if s.paymentConfig == nil {
		return defaultBalanceRechargeMultiplier
	}
	cfg, err := s.paymentConfig.GetPaymentConfig(ctx)
	if err != nil || cfg == nil {
		return defaultBalanceRechargeMultiplier
	}
	return normalizeBalanceRechargeMultiplier(cfg.BalanceRechargeMultiplier)
}

func (s *ProviderPricingService) siteMetadata(ctx context.Context) (siteName, siteDomain string) {
	siteName = "Sub2API"
	if s.settingService == nil {
		return siteName, ""
	}

	settings, err := s.settingService.GetPublicSettings(ctx)
	if err != nil || settings == nil {
		return s.settingService.GetSiteName(ctx), ""
	}
	siteName = strings.TrimSpace(settings.SiteName)
	if siteName == "" {
		siteName = "Sub2API"
	}
	siteDomain = extractProviderPricingDomain(settings.APIBaseURL)
	if siteDomain == "" {
		siteDomain = extractProviderPricingDomain(s.frontendURL(ctx))
	}
	return siteName, siteDomain
}

func (s *ProviderPricingService) frontendURL(ctx context.Context) string {
	if s == nil || s.settingService == nil {
		return ""
	}
	if s.settingService.settingRepo != nil {
		if val, err := s.settingService.settingRepo.GetValue(ctx, SettingKeyFrontendURL); err == nil && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	if s.settingService.cfg == nil {
		return ""
	}
	return s.settingService.cfg.Server.FrontendURL
}

func providerPricingFromModel(
	modelName string,
	groupName string,
	pricing *ChannelModelPricing,
	rateMultiplier float64,
	rechargeMultiplier float64,
	enabled bool,
) *ProviderPricingModel {
	if pricing == nil || pricing.BillingMode == BillingModePerRequest || pricing.BillingMode == BillingModeImage {
		return nil
	}
	input := cnyPerMTok(pricing.InputPrice, rateMultiplier, rechargeMultiplier)
	if input == nil {
		return nil
	}
	return &ProviderPricingModel{
		ModelName:          modelName,
		GroupName:          groupName,
		InputPrice:         *input,
		OutputPrice:        cnyPerMTok(pricing.OutputPrice, rateMultiplier, rechargeMultiplier),
		CacheInputPrice:    cnyPerMTok(pricing.CacheReadPrice, rateMultiplier, rechargeMultiplier),
		CacheCreatePrice:   cnyPerMTok(pricing.CacheWritePrice, rateMultiplier, rechargeMultiplier),
		CacheCreatePrice1h: nil,
		Enabled:            enabled,
		Note:               "",
	}
}

func cnyPerMTok(pricePerToken *float64, rateMultiplier float64, rechargeMultiplier float64) *float64 {
	if pricePerToken == nil {
		return nil
	}
	if rechargeMultiplier <= 0 {
		rechargeMultiplier = defaultBalanceRechargeMultiplier
	}
	v := *pricePerToken * rateMultiplier / rechargeMultiplier * tokensPerMillion
	return &v
}

func extractProviderPricingDomain(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(host), "www.")
}
