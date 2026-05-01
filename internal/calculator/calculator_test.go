package calculator_test

import (
	"os"
	"path/filepath"
	"testing"
	"ai-flight-dashboard/internal/calculator"
)

func TestCalculateCost(t *testing.T) {
	// Create a temporary pricing table
	tempDir := t.TempDir()
	pricingPath := filepath.Join(tempDir, "pricing_table.json")
	content := `{
		"models": {
			"test-model": {
				"input_price_per_m": 2.00,
				"cached_price_per_m": 0.50,
				"cache_creation_price_per_m": 3.00,
				"output_price_per_m": 10.00
			}
		}
	}`
	err := os.WriteFile(pricingPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp pricing table: %v", err)
	}

	calc, err := calculator.New(pricingPath)
	if err != nil {
		t.Fatalf("Failed to initialize calculator: %v", err)
	}

	// 1,000,000 input tokens = $2.00
	// 1,000,000 output tokens = $10.00
	cost, err := calc.CalculateCost("test-model", 1000000, 0, 0, 1000000)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	expected := 12.00
	if cost != expected {
		t.Errorf("Expected cost %f, got %f", expected, cost)
	}

	// Test cached tokens
	// 500,000 input, 500,000 cached, 0 cache_creation, 1,000,000 output
	// baseInput: max(500000 - 500000 - 0, 0) = 0 → inputCost = $0.00
	// cached: 500,000 * 0.5 / 1000000 = $0.25
	// output: 1000000 * 10.0 / 1000000 = $10.00
	// total = 10.25
	cost, err = calc.CalculateCost("test-model", 500000, 500000, 0, 1000000)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	expected = 10.25
	if cost != expected {
		t.Errorf("Expected cost %f, got %f", expected, cost)
	}

	// Test cache creation tokens
	// 600,000 input, 400,000 cached, 100,000 cache creation, 1,000,000 output
	// baseInput: max(600000 - 400000 - 100000, 0) = 100,000 → inputCost = $0.20
	// cached: 400,000 * 0.5 / 1000000 = $0.20
	// cache_creation: 100,000 * 3.0 / 1000000 = $0.30
	// output: 1000000 * 10.0 / 1000000 = $10.00
	// total = 10.70
	cost, err = calc.CalculateCost("test-model", 600000, 400000, 100000, 1000000)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	expected = 10.70
	if cost != expected {
		t.Errorf("Expected cost %f, got %f", expected, cost)
	}

	// Unknown model → returns 0 cost, no error (graceful fallback)
	cost, err = calc.CalculateCost("unknown", 100, 0, 0, 100)
	if err != nil {
		t.Errorf("Unexpected error for unknown model: %v", err)
	}
	if cost != 0 {
		t.Errorf("Expected 0 cost for unknown model, got %f", cost)
	}
}
