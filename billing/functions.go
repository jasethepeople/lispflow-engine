package billing

import (
	"fmt"
	"math"

	"github.com/glycerine/zygomys/zygo"
)

// disableDangerous strips all I/O and execution functions from the interpreter.
func disableDangerous(env *zygo.Glisp) {
	dangerous := []string{
		"open", "read", "write", "close", "load", "eval-string",
		"system", "shell", "exec", "os-exec", "spawn", "http-get",
		"http-post", "tcp-connect", "udp-connect", "file-exists",
		"delete-file", "rename-file", "mkdir", "rmdir", "chdir",
		"tcp-listen", "udp-listen", "socket", "net-dial", "net-listen",
		"go", "go-run", "go-channel", "go-receive", "go-send",
		"reflect", "unsafe", "unsafe-pointer", "cgo",
	}
	for _, name := range dangerous {
		env.RemoveFunction(name)
	}
}

// registerBillingFunctions binds robust billing primitives into the Lisp environment.
func registerBillingFunctions(env *zygo.Glisp) {
	// ── Core Usage Primitive ──────────────────────────────────────────────
	env.AddFunction("usage",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 1 {
				return zygo.SexpNull, fmt.Errorf("usage expects 1 argument, got %d", len(args))
			}
			key, err := zygo.DecodeString(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("usage key must be a string: %w", err)
			}
			usageMap, err := env.FindGlobal("*usage*")
			if err != nil {
				return zygo.NewSexpFloat(0.0), nil // Default to 0 if *usage* not bound
			}
			hash, ok := usageMap.(*zygo.SexpHash)
			if !ok {
				return zygo.NewSexpFloat(0.0), nil
			}
			val, err := hash.HashGet(key)
			if err != nil {
				return zygo.NewSexpFloat(0.0), nil // Default to 0 for missing keys
			}
			return val, nil
		})

	// ── Tiered Pricing ────────────────────────────────────────────────────
	env.AddFunction("tiered",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 2 {
				return zygo.SexpNull, fmt.Errorf("tiered expects 2 args, got %d", len(args))
			}
			usage, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("tiered usage must be numeric: %w", err)
			}
			tierList, ok := args[1].(*zygo.SexpArray)
			if !ok {
				return zygo.SexpNull, fmt.Errorf("tiers must be a list/array")
			}

			total := 0.0
			remaining := usage

			for _, elem := range tierList.Val {
				tier, ok := elem.(*zygo.SexpArray)
				if !ok || len(tier.Val) != 3 {
					return zygo.SexpNull, fmt.Errorf("each tier must be (low high price)")
				}

				low, err := zygo.DecodeFloat64(tier.Val[0])
				if err != nil {
					return zygo.SexpNull, fmt.Errorf("tier low bound must be numeric: %w", err)
				}

				high := -1.0
				if tier.Val[1] != zygo.SexpNull {
					high, err = zygo.DecodeFloat64(tier.Val[1])
					if err != nil {
						return zygo.SexpNull, fmt.Errorf("tier high bound must be numeric or nil: %w", err)
					}
				}

				price, err := zygo.DecodeFloat64(tier.Val[2])
				if err != nil {
					return zygo.SexpNull, fmt.Errorf("tier price must be numeric: %w", err)
				}

				if remaining <= 0 {
					break
				}
				if high > 0 && usage < low {
					continue
				}

				bracketSize := remaining
				if high > 0 && remaining > (high-low) {
					bracketSize = high - low
					if usage < high {
						bracketSize = usage - low
					}
				}
				if bracketSize < 0 {
					bracketSize = 0
				}

				total += bracketSize * price
				remaining -= bracketSize
			}
			return zygo.NewSexpFloat(total), nil
		})

	// ── Volume Pricing ────────────────────────────────────────────────────
	env.AddFunction("volume",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 2 {
				return zygo.SexpNull, fmt.Errorf("volume expects 2 args, got %d", len(args))
			}
			usage, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("volume usage must be numeric: %w", err)
			}
			rate, err := zygo.DecodeFloat64(args[1])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("volume rate must be numeric: %w", err)
			}
			return zygo.NewSexpFloat(usage * rate), nil
		})

	// ── Overage Pricing ────────────────────────────────────────────────────
	env.AddFunction("overage",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 3 {
				return zygo.SexpNull, fmt.Errorf("overage expects 3 args, got %d", len(args))
			}
			usage, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("overage usage must be numeric: %w", err)
			}
			included, err := zygo.DecodeFloat64(args[1])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("overage included must be numeric: %w", err)
			}
			rate, err := zygo.DecodeFloat64(args[2])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("overage rate must be numeric: %w", err)
			}
			if usage <= included {
				return zygo.NewSexpFloat(0.0), nil
			}
			return zygo.NewSexpFloat((usage - included) * rate), nil
		})

	// ── Minimum Charge ────────────────────────────────────────────────────
	env.AddFunction("min-charge",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 2 {
				return zygo.SexpNull, fmt.Errorf("min-charge expects 2 args, got %d", len(args))
			}
			amount, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("min-charge amount must be numeric: %w", err)
			}
			floor, err := zygo.DecodeFloat64(args[1])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("min-charge floor must be numeric: %w", err)
			}
			return zygo.NewSexpFloat(math.Max(amount, floor)), nil
		})

	// ── Maximum Cap ───────────────────────────────────────────────────────
	env.AddFunction("max-cap",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 2 {
				return zygo.SexpNull, fmt.Errorf("max-cap expects 2 args, got %d", len(args))
			}
			amount, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("max-cap amount must be numeric: %w", err)
			}
			ceiling, err := zygo.DecodeFloat64(args[1])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("max-cap ceiling must be numeric: %w", err)
			}
			return zygo.NewSexpFloat(math.Min(amount, ceiling)), nil
		})

	// ── Discount ──────────────────────────────────────────────────────────
	env.AddFunction("discount",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 2 {
				return zygo.SexpNull, fmt.Errorf("discount expects 2 args, got %d", len(args))
			}
			amount, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("discount amount must be numeric: %w", err)
			}
			percent, err := zygo.DecodeFloat64(args[1])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("discount percent must be numeric: %w", err)
			}
			return zygo.NewSexpFloat(amount * (1 - percent/100)), nil
		})

	// ── Conditional Pricing ──────────────────────────────────────────────
	env.AddFunction("when-usage",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 3 {
				return zygo.SexpNull, fmt.Errorf("when-usage expects 3 args, got %d", len(args))
			}
			condition, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("when-usage condition must be numeric: %w", err)
			}
			if condition > 0 {
				return args[1], nil
			}
			return args[2], nil
		})

	// ── Bundle Pricing ────────────────────────────────────────────────────
	env.AddFunction("bundle",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 3 {
				return zygo.SexpNull, fmt.Errorf("bundle expects 3 args, got %d", len(args))
			}
			included, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("bundle included must be numeric: %w", err)
			}
			unitPrice, err := zygo.DecodeFloat64(args[1])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("bundle unit-price must be numeric: %w", err)
			}
			usage, err := zygo.DecodeFloat64(args[2])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("bundle usage must be numeric: %w", err)
			}
			units := math.Min(usage, included)
			return zygo.NewSexpFloat(units * unitPrice), nil
		})

	// ── Per-Unit Pricing ──────────────────────────────────────────────────
	env.AddFunction("per-unit",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 2 {
				return zygo.SexpNull, fmt.Errorf("per-unit expects 2 args, got %d", len(args))
			}
			usage, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("per-unit usage must be numeric: %w", err)
			}
			price, err := zygo.DecodeFloat64(args[1])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("per-unit price must be numeric: %w", err)
			}
			return zygo.NewSexpFloat(math.Ceil(usage) * price), nil
		})

	// ── Rounding ──────────────────────────────────────────────────────────
	env.AddFunction("round2",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 1 {
				return zygo.SexpNull, fmt.Errorf("round2 expects 1 arg, got %d", len(args))
			}
			val, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("round2 value must be numeric: %w", err)
			}
			return zygo.NewSexpFloat(math.Round(val*100) / 100), nil
		})

	// ── Tax Calculation ───────────────────────────────────────────────────
	env.AddFunction("tax",
		func(env *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 2 {
				return zygo.SexpNull, fmt.Errorf("tax expects 2 args, got %d", len(args))
			}
			amount, err := zygo.DecodeFloat64(args[0])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("tax amount must be numeric: %w", err)
			}
			rate, err := zygo.DecodeFloat64(args[1])
			if err != nil {
				return zygo.SexpNull, fmt.Errorf("tax rate must be numeric: %w", err)
			}
			return zygo.NewSexpFloat(amount * (1 + rate/100)), nil
		})
}
