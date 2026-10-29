package billing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/glycerine/zygomys/zygo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	evalCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "lispflow_evaluations_total",
		Help: "Total number of pricing evaluations",
	}, []string{"status", "customer_id"})

	evalDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "lispflow_evaluation_duration_seconds",
		Help:    "Pricing evaluation duration",
		Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15),
	}, []string{"status"})

	planSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lispflow_plan_size_bytes",
		Help:    "Size of pricing plans in bytes",
		Buckets: prometheus.ExponentialBuckets(128, 2, 10),
	})
)

// BillingEngine holds a pool of interpreters for thread-safe evaluation.
type BillingEngine struct {
	mu              sync.RWMutex
	env             *zygo.Glisp
	plans           map[string]string
	sandbox         bool
	maxPlanSize     int
	maxEvalTime     time.Duration
	defaultCurrency string
}

// EngineOption configures the billing engine.
type EngineOption func(*BillingEngine)

// WithSandbox enables/disables sandbox mode.
func WithSandbox(enabled bool) EngineOption {
	return func(be *BillingEngine) { be.sandbox = enabled }
}

// WithMaxPlanSize sets the maximum plan size in bytes.
func WithMaxPlanSize(size int) EngineOption {
	return func(be *BillingEngine) { be.maxPlanSize = size }
}

// WithMaxEvalTime sets the maximum evaluation time.
func WithMaxEvalTime(d time.Duration) EngineOption {
	return func(be *BillingEngine) { be.maxEvalTime = d }
}

// WithDefaultCurrency sets the default currency.
func WithDefaultCurrency(currency string) EngineOption {
	return func(be *BillingEngine) { be.defaultCurrency = currency }
}

// NewBillingEngine initialises the interpreter sandbox.
func NewBillingEngine(opts ...EngineOption) *BillingEngine {
	env := zygo.NewGlisp()
	env.StandardSetup()

	be := &BillingEngine{
		env:             env,
		plans:           make(map[string]string),
		sandbox:         true,
		maxPlanSize:     65536,
		maxEvalTime:     100 * time.Millisecond,
		defaultCurrency: "USD",
	}

	for _, opt := range opts {
		opt(be)
	}

	if be.sandbox {
		disableDangerous(env)
	}
	registerBillingFunctions(env)

	return be
}

// LoadPlan stores a new Lisp pricing expression for a customer.
func (be *BillingEngine) LoadPlan(customerID, planSexp string) error {
	if len(planSexp) > be.maxPlanSize {
		return fmt.Errorf("plan exceeds maximum size of %d bytes", be.maxPlanSize)
	}

	// Validate by parsing
	testEnv := be.env.Duplicate()
	_, err := testEnv.Parse(planSexp)
	if err != nil {
		return fmt.Errorf("invalid plan expression: %w", err)
	}

	be.mu.Lock()
	defer be.mu.Unlock()
	be.plans[customerID] = planSexp
	planSize.Observe(float64(len(planSexp)))
	return nil
}

// GetPlan retrieves the current pricing expression for a customer.
func (be *BillingEngine) GetPlan(customerID string) (string, bool) {
	be.mu.RLock()
	defer be.mu.RUnlock()
	plan, ok := be.plans[customerID]
	return plan, ok
}

// DeletePlan removes a customer's pricing plan.
func (be *BillingEngine) DeletePlan(customerID string) {
	be.mu.Lock()
	defer be.mu.Unlock()
	delete(be.plans, customerID)
}

// EvaluatePricing evaluates a usage map against a customer's plan with timeout.
func (be *BillingEngine) EvaluatePricing(ctx context.Context, customerID string, usage map[string]float64) (float64, string, error) {
	start := time.Now()
	status := "success"
	defer func() {
		evalDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
		evalCounter.WithLabelValues(status, customerID).Inc()
	}()

	be.mu.RLock()
	plan, ok := be.plans[customerID]
	be.mu.RUnlock()
	if !ok {
		status = "no_plan"
		return 0, be.defaultCurrency, fmt.Errorf("no plan for customer %s", customerID)
	}

	// Create evaluation context with timeout
	ctx, cancel := context.WithTimeout(ctx, be.maxEvalTime)
	defer cancel()

	// Run evaluation in goroutine with timeout
	type result struct {
		cost float64
		err  error
	}
	resCh := make(chan result, 1)

	go func() {
		cost, err := be.evaluate(plan, usage)
		resCh <- result{cost, err}
	}()

	select {
	case <-ctx.Done():
		status = "timeout"
		return 0, be.defaultCurrency, fmt.Errorf("evaluation timeout after %v", be.maxEvalTime)
	case res := <-resCh:
		if res.err != nil {
			status = "error"
			return 0, be.defaultCurrency, res.err
		}
		return res.cost, be.defaultCurrency, nil
	}
}

// evaluate performs the actual Lisp evaluation.
func (be *BillingEngine) evaluate(plan string, usage map[string]float64) (float64, error) {
	env := be.env.Duplicate()

	usageHash := zygo.NewSexpHash()
	for k, v := range usage {
		usageHash.HashSet(k, zygo.NewSexpFloat(v))
	}
	env.SetGlobal("*usage*", usageHash)

	expr, err := env.Parse(plan)
	if err != nil {
		return 0, fmt.Errorf("plan parse error: %w", err)
	}
	res, err := env.Eval(expr)
	if err != nil {
		return 0, fmt.Errorf("plan eval error: %w", err)
	}

	cost, err := zygo.DecodeFloat64(res)
	if err != nil {
		return 0, fmt.Errorf("plan did not return a number: %v", res)
	}
	return cost, nil
}

// SimulatePlan runs historical usage through a proposed plan.
func (be *BillingEngine) SimulatePlan(ctx context.Context, proposedPlan string, history []map[string]float64) (*SimulationResult, error) {
	if len(proposedPlan) > be.maxPlanSize {
		return nil, fmt.Errorf("proposed plan exceeds maximum size of %d bytes", be.maxPlanSize)
	}

	// Validate proposed plan
	testEnv := be.env.Duplicate()
	_, err := testEnv.Parse(proposedPlan)
	if err != nil {
		return nil, fmt.Errorf("invalid proposed plan: %w", err)
	}

	be.mu.Lock()
	be.plans["__sim__"] = proposedPlan
	be.mu.Unlock()
	defer func() {
		be.mu.Lock()
		delete(be.plans, "__sim__")
		be.mu.Unlock()
	}()

	start := time.Now()
	result := &SimulationResult{
		PlanExpr:      proposedPlan,
		Periods:       len(history),
		CostBreakdown: make([]PeriodCost, 0, len(history)),
	}

	for i, usage := range history {
		c, err := be.EvaluatePricing(ctx, "__sim__", usage)
		if err != nil {
			return nil, fmt.Errorf("simulation at point %d: %w", i, err)
		}
		result.CostBreakdown = append(result.CostBreakdown, PeriodCost{
			Period: i + 1,
			Usage:  usage,
			Cost:   c,
		})
		result.TotalCost += c
		if i == 0 || c < result.MinCost {
			result.MinCost = c
		}
		if c > result.MaxCost {
			result.MaxCost = c
		}
	}

	if result.Periods > 0 {
		result.AverageCost = result.TotalCost / float64(result.Periods)
	}
	result.DurationMs = time.Since(start).Milliseconds()

	return result, nil
}

// ValidatePlan checks if a plan expression is syntactically valid.
func (be *BillingEngine) ValidatePlan(planSexp string) error {
	if len(planSexp) > be.maxPlanSize {
		return fmt.Errorf("plan exceeds maximum size of %d bytes", be.maxPlanSize)
	}
	env := be.env.Duplicate()
	_, err := env.Parse(planSexp)
	return err
}
