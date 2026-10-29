package billing

import (
	"time"

	"github.com/google/uuid"
)

// Plan represents a customer pricing plan (versioned).
type Plan struct {
	ID          uuid.UUID `json:"id" db:"id"`
	CustomerID  string    `json:"customer_id" db:"customer_id"`
	PlanExpr    string    `json:"plan_expr" db:"plan_expr"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	ActivatedAt *time.Time `json:"activated_at,omitempty" db:"activated_at"`
	Metadata    map[string]string `json:"metadata,omitempty" db:"metadata"`
}

// UsageEvent represents a single usage dimension event.
type UsageEvent struct {
	ID         uuid.UUID         `json:"id"`
	CustomerID string            `json:"customer_id"`
	Dimensions map[string]float64 `json:"dimensions"`
	Timestamp  time.Time         `json:"timestamp"`
	Source     string            `json:"source"` // e.g., "api", "kafka", "batch"
}

// LedgerEntry represents a computed billing transaction.
type LedgerEntry struct {
	ID          uuid.UUID         `json:"id" db:"id"`
	CustomerID  string            `json:"customer_id" db:"customer_id"`
	PlanID      uuid.UUID         `json:"plan_id" db:"plan_id"`
	PlanExpr    string            `json:"plan_expr" db:"plan_expr"`
	UsageData   map[string]float64 `json:"usage_data" db:"usage_data"`
	Cost        float64           `json:"cost" db:"cost"`
	Currency    string            `json:"currency" db:"currency"`
	PeriodStart time.Time         `json:"period_start" db:"period_start"`
	PeriodEnd   time.Time         `json:"period_end" db:"period_end"`
	CreatedAt   time.Time         `json:"created_at" db:"created_at"`
	EvaluatedAt time.Time         `json:"evaluated_at" db:"evaluated_at"`
	EvalDurationMs int64          `json:"eval_duration_ms" db:"eval_duration_ms"`
}

// SimulationResult holds the output of a time-travel simulation.
type SimulationResult struct {
	PlanExpr      string    `json:"plan_expr"`
	Periods       int       `json:"periods"`
	TotalCost     float64   `json:"total_cost"`
	AverageCost   float64   `json:"average_cost"`
	MinCost       float64   `json:"min_cost"`
	MaxCost       float64   `json:"max_cost"`
	CostBreakdown []PeriodCost `json:"cost_breakdown"`
	DurationMs    int64     `json:"duration_ms"`
}

// PeriodCost represents a single period in a simulation.
type PeriodCost struct {
	Period    int                `json:"period"`
	Usage     map[string]float64 `json:"usage"`
	Cost      float64            `json:"cost"`
	Timestamp time.Time          `json:"timestamp"`
}

// WebhookPayload represents the data sent to customer webhooks.
type WebhookPayload struct {
	EventType   string      `json:"event_type"`
	CustomerID  string      `json:"customer_id"`
	Timestamp   time.Time   `json:"timestamp"`
	Data        interface{} `json:"data"`
	Signature   string      `json:"signature"`
}

// BillingPeriod represents a billing cycle.
type BillingPeriod struct {
	CustomerID  string    `json:"customer_id"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Status      string    `json:"status"` // "open", "closing", "closed"
	TotalUsage  map[string]float64 `json:"total_usage"`
	ComputedCost float64  `json:"computed_cost"`
}
