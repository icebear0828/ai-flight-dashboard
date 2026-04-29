package calculator

import (
	"encoding/json"
	"fmt"
	"os"
)

type ModelPrice struct {
	InputPricePerM  float64 `json:"input_price_per_m"`
	CachedPricePerM float64 `json:"cached_price_per_m"`
	OutputPricePerM float64 `json:"output_price_per_m"`
}

type PricingTable struct {
	Models map[string]ModelPrice `json:"models"`
}

type Calculator struct {
	prices PricingTable
}

func New(pricingFilePath string) (*Calculator, error) {
	data, err := os.ReadFile(pricingFilePath)
	if err != nil {
		return nil, fmt.Errorf("read pricing file: %w", err)
	}
	var pt PricingTable
	if err := json.Unmarshal(data, &pt); err != nil {
		return nil, fmt.Errorf("unmarshal pricing table: %w", err)
	}
	return &Calculator{prices: pt}, nil
}

// CalculateCost takes the specific token metrics and returns the estimated USD cost.
func (c *Calculator) CalculateCost(model string, inputTokens, cachedTokens, outputTokens int) (float64, error) {
	price, ok := c.prices.Models[model]
	if !ok {
		return 0, nil // unknown model, skip silently
	}
	
	baseInput := inputTokens - cachedTokens
	if baseInput < 0 {
		baseInput = 0
	}
	
	inputCost := (float64(baseInput) / 1_000_000.0) * price.InputPricePerM
	cachedCost := (float64(cachedTokens) / 1_000_000.0) * price.CachedPricePerM
	outputCost := (float64(outputTokens) / 1_000_000.0) * price.OutputPricePerM
	
	return inputCost + cachedCost + outputCost, nil
}

// GetModelPrice returns the price table for a specific model.
func (c *Calculator) GetModelPrice(model string) (ModelPrice, bool) {
	price, ok := c.prices.Models[model]
	return price, ok
}
