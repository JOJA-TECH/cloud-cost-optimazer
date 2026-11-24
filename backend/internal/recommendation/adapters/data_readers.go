package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
)

type DataReaders struct {
	costs   data.CostRepository
	configs data.ConfigurationRepository
	prices  data.PriceCatalogReader
	skus    data.SKUReader
	context data.ResourceContextReader
}

func NewDataReaders(costs data.CostRepository, configs data.ConfigurationRepository, prices data.PriceCatalogReader, skus data.SKUReader, resourceContext data.ResourceContextReader) *DataReaders {
	return &DataReaders{costs: costs, configs: configs, prices: prices, skus: skus, context: resourceContext}
}

func (r *DataReaders) Costs(ctx context.Context, resourceUID uuid.UUID, from, to time.Time, limit int) ([]recommendation.CostRecord, error) {
	rows, err := r.costs.CostDailyByResource(ctx, resourceUID, from, to, limit)
	if err != nil {
		return nil, err
	}
	out := make([]recommendation.CostRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, recommendation.CostRecord{
			ResourceUID: row.ResourceUID, ProviderCode: row.ProviderCode, ScopeType: row.ScopeType,
			ScopeName: row.ScopeName, ServiceName: row.ServiceName, RegionCode: row.RegionCode,
			UsageDate: row.UsageDate, Currency: row.Currency, MeterID: row.MeterID,
			PreTaxCost: row.PreTaxCost, UsageQuantity: row.UsageQuantity,
			UsageUnit: row.UsageUnit, PricingModel: row.PricingModel, ChargeCategory: row.ChargeCategory,
		})
	}
	return out, nil
}

func (r *DataReaders) LatestConfigSnapshot(ctx context.Context, resourceUID uuid.UUID) (*recommendation.ConfigurationSnapshot, error) {
	row, err := r.configs.LatestConfigSnapshot(ctx, resourceUID)
	if errors.Is(err, data.ErrNotFound) {
		return nil, recommendation.ErrResourceNotFound
	}
	if err != nil || row == nil {
		return nil, err
	}
	return &recommendation.ConfigurationSnapshot{
		ResourceUID: row.ResourceUID, SKUName: row.SKUName, Tier: row.Tier,
		Family: row.Family, CapacityVCores: row.CapacityVCores,
		MinCapacityVCores: row.MinCapacityVCores, ComputeModel: row.ComputeModel,
		MaxSizeBytes: row.MaxSizeBytes, AutoPauseDelayMin: row.AutoPauseDelayMin,
		Status: row.Status, ObservedAtUTC: row.ObservedAtUTC,
	}, nil
}

func (r *DataReaders) ListPrices(ctx context.Context, query recommendation.PriceQuery) ([]recommendation.PriceRecord, error) {
	rows, err := r.prices.ListPrices(ctx, data.PriceQuery{
		ProviderCode: query.ProviderCode, RegionCode: query.RegionCode,
		MeterID: query.MeterID, SKUName: query.SKUName, ARMSKUName: query.ARMSKUName,
		ComputeModel: query.ComputeModel, PriceType: query.PriceType,
		Currency: query.Currency, EffectiveAt: query.EffectiveAt,
	})
	if err != nil {
		return nil, err
	}
	out := make([]recommendation.PriceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, recommendation.PriceRecord{
			ProviderCode: row.ProviderCode, RegionCode: row.RegionCode, MeterID: row.MeterID,
			MeterName: row.MeterName, ProductName: row.ProductName, SKUName: row.SKUName,
			ARMSKUName: row.ARMSKUName, ServiceName: row.ServiceName, PriceType: row.PriceType,
			UnitOfMeasure: row.UnitOfMeasure, UnitPrice: row.UnitPrice, Currency: row.Currency,
			TierMinimum: row.TierMinimum, EffectiveStart: row.EffectiveStart, EffectiveEnd: row.EffectiveEnd,
		})
	}
	return out, nil
}

func (r *DataReaders) ListSKUs(ctx context.Context, current recommendation.ConfigurationSnapshot, includeServerless bool) ([]recommendation.SKU, error) {
	rows, err := r.skus.ListSKUs(ctx, stringValue(current.Tier), stringValue(current.Family), includeServerless)
	if err != nil {
		return nil, err
	}
	out := make([]recommendation.SKU, 0, len(rows))
	for _, row := range rows {
		out = append(out, recommendation.SKU{
			ID: row.ID, Name: row.Name, ARMSKUName: row.ARMSKUName, Tier: row.Tier,
			Family: row.Family, ComputeModel: row.ComputeModel, CapacityVCores: row.CapacityVCores,
			MinCapacityVCores: row.MinCapacityVCores, MaxSizeGB: row.MaxSizeGB, InScope: row.InScope,
		})
	}
	return out, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (r *DataReaders) PricingContext(ctx context.Context, resourceUID uuid.UUID) (recommendation.PricingContext, error) {
	row, err := r.context.PricingContext(ctx, resourceUID)
	if err != nil {
		if errors.Is(err, data.ErrNotFound) {
			return recommendation.PricingContext{}, recommendation.ErrResourceNotFound
		}
		return recommendation.PricingContext{}, err
	}
	return recommendation.PricingContext{ProviderCode: row.ProviderCode, RegionCode: row.RegionCode}, nil
}

var _ recommendation.CostReader = (*DataReaders)(nil)
var _ recommendation.ConfigurationReader = (*DataReaders)(nil)
var _ recommendation.PriceReader = (*DataReaders)(nil)
var _ recommendation.SKUReader = (*DataReaders)(nil)
var _ recommendation.ResourceContextReader = (*DataReaders)(nil)
