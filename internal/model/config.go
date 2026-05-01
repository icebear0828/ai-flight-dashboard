package model

import "fmt"

// BillingMode determines how usage is tracked and what alerts are shown.
type BillingMode string

const (
	BillingAuto         BillingMode = "auto"
	BillingSubscription BillingMode = "subscription"
	BillingAPI          BillingMode = "api"
)

// ParseBillingMode converts a string to a validated BillingMode.
func ParseBillingMode(s string) (BillingMode, error) {
	switch s {
	case "", "auto":
		return BillingAuto, nil
	case "subscription":
		return BillingSubscription, nil
	case "api":
		return BillingAPI, nil
	default:
		return "", fmt.Errorf("invalid billing mode %q: must be auto, subscription, or api", s)
	}
}

// PlanLimit defines the token and cost caps for a subscription plan.
type PlanLimit struct {
	TokenLimit int
	CostLimit  float64
}

// PlanLimits maps subscription plan names to their limits.
var PlanLimits = map[string]PlanLimit{
	"pro":   {TokenLimit: 19000, CostLimit: 18.00},
	"max5":  {TokenLimit: 88000, CostLimit: 35.00},
	"max20": {TokenLimit: 220000, CostLimit: 140.00},
}

// DashboardConfig holds all runtime configuration parsed from CLI flags.
type DashboardConfig struct {
	BillingMode BillingMode
	Plan        string  // pro, max5, max20 (subscription mode only)
	BudgetDaily float64 // USD, 0 = disabled (api mode only)
	DeviceID    string
}
