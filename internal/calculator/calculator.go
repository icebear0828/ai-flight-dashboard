package calculator

import (
	"encoding/json"
	"fmt"
	"os"
)

type ModelPrice struct {
	InputPricePerM          float64 `json:"input_price_per_m"`
	CachedPricePerM         float64 `json:"cached_price_per_m"`
	CacheCreationPricePerM  float64 `json:"cache_creation_price_per_m"`
	OutputPricePerM         float64 `json:"output_price_per_m"`
}

type PricingTable struct {
	Models map[string]ModelPrice `json:"models"`
}

type Calculator struct {
	prices PricingTable
}

// NewFromBytes creates a Calculator from raw JSON pricing data.
func NewFromBytes(data []byte) (*Calculator, error) {
	var pt PricingTable
	if err := json.Unmarshal(data, &pt); err != nil {
		return nil, fmt.Errorf("unmarshal pricing table: %w", err)
	}
	return &Calculator{prices: pt}, nil
}

// New creates a Calculator by reading a pricing JSON file from disk.
func New(pricingFilePath string) (*Calculator, error) {
	data, err := os.ReadFile(pricingFilePath)
	if err != nil {
		return nil, fmt.Errorf("read pricing file: %w", err)
	}
	return NewFromBytes(data)
}

// CalculateCost takes the specific token metrics and returns the estimated USD cost.
func (c *Calculator) CalculateCost(model string, inputTokens, cachedTokens, cacheCreationTokens, outputTokens int) (float64, error) {
	price, ok := c.prices.Models[model]
	if !ok {
		return 0, nil // unknown model, skip silently
	}
	
	baseInput := inputTokens - cachedTokens - cacheCreationTokens
	if baseInput < 0 {
		baseInput = 0
	}
	
	inputCost := (float64(baseInput) / 1_000_000.0) * price.InputPricePerM
	cachedCost := (float64(cachedTokens) / 1_000_000.0) * price.CachedPricePerM
	cacheCreationCost := (float64(cacheCreationTokens) / 1_000_000.0) * price.CacheCreationPricePerM
	outputCost := (float64(outputTokens) / 1_000_000.0) * price.OutputPricePerM
	
	return inputCost + cachedCost + cacheCreationCost + outputCost, nil
}

// GetModelPrice returns the price table for a specific model.
func (c *Calculator) GetModelPrice(model string) (ModelPrice, bool) {
	price, ok := c.prices.Models[model]
	return price, ok
}

// ListModels returns all model names in the pricing table.
func (c *Calculator) ListModels() []string {
	models := make([]string, 0, len(c.prices.Models))
	for m := range c.prices.Models {
		models = append(models, m)
	}
	return models
}

// CalculateCostNoCaching computes a hypothetical cost where cached tokens are
// charged at the full input price instead of the discounted cached price.
func (c *Calculator) CalculateCostNoCaching(model string, inputTokens, cachedTokens, cacheCreationTokens, outputTokens int) (float64, error) {
	price, ok := c.prices.Models[model]
	if !ok {
		return 0, nil
	}
	// Treat ALL input tokens (including cached and cache_creation) at full input price
	inputCost := (float64(inputTokens) / 1_000_000.0) * price.InputPricePerM
	outputCost := (float64(outputTokens) / 1_000_000.0) * price.OutputPricePerM
	return inputCost + outputCost, nil
}
