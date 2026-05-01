// Package testutil provides shared test helpers to eliminate boilerplate
// across test files. All helpers use t.Helper() and t.TempDir() for
// proper error attribution and automatic cleanup.
package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
)

// PricingJSON is a minimal pricing table covering the models used in tests.
const PricingJSON = `{
  "models": {
    "claude-opus-4-7": {
      "input_price_per_m": 15,
      "cached_price_per_m": 1.5,
      "cache_creation_price_per_m": 22.5,
      "output_price_per_m": 75
    },
    "gemini-2.5-pro": {
      "input_price_per_m": 1.25,
      "cached_price_per_m": 0.31,
      "cache_creation_price_per_m": 4.5,
      "output_price_per_m": 5
    }
  }
}`

// NewTestDB creates a temporary SQLite database for testing.
// The database is automatically cleaned up when the test finishes.
func NewTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// NewTestCalc creates a Calculator initialized with a standard test pricing table.
func NewTestCalc(t *testing.T) *calculator.Calculator {
	t.Helper()
	pricingPath := filepath.Join(t.TempDir(), "pricing.json")
	if err := os.WriteFile(pricingPath, []byte(PricingJSON), 0644); err != nil {
		t.Fatalf("failed to write test pricing: %v", err)
	}
	calc, err := calculator.New(pricingPath)
	if err != nil {
		t.Fatalf("failed to create test calculator: %v", err)
	}
	return calc
}

// NewTestDBAndCalc is a convenience function that creates both.
func NewTestDBAndCalc(t *testing.T) (*db.DB, *calculator.Calculator) {
	t.Helper()
	return NewTestDB(t), NewTestCalc(t)
}
