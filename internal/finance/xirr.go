// Package finance computes portfolio holdings and returns (XIRR) from
// transaction history.
package finance

import (
	"math"
	"sort"
	"time"
)

// CashFlow is one signed cash flow on a date, from the investor's point
// of view: negative = money out of pocket (a purchase), positive = money
// received (a redemption, or the current value of what's still held).
type CashFlow struct {
	Date   time.Time
	Amount float64
}

// XIRR solves for the annualised internal rate of return implied by a set
// of irregularly-dated cash flows, using Newton-Raphson with a bisection
// fallback if Newton-Raphson fails to converge (e.g. a bad initial
// guess for an unusual cash-flow shape). Returns (rate, ok) - ok is false
// if no root could be found (e.g. all cash flows have the same sign,
// which has no solution).
func XIRR(flows []CashFlow) (float64, bool) {
	if len(flows) < 2 {
		return 0, false
	}
	sorted := make([]CashFlow, len(flows))
	copy(sorted, flows)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	hasPositive, hasNegative := false, false
	for _, f := range sorted {
		if f.Amount > 0 {
			hasPositive = true
		}
		if f.Amount < 0 {
			hasNegative = true
		}
	}
	if !hasPositive || !hasNegative {
		return 0, false
	}

	t0 := sorted[0].Date
	years := make([]float64, len(sorted))
	for i, f := range sorted {
		years[i] = f.Date.Sub(t0).Hours() / 24 / 365.0
	}

	npv := func(rate float64) float64 {
		sum := 0.0
		for i, f := range sorted {
			sum += f.Amount / math.Pow(1+rate, years[i])
		}
		return sum
	}
	dnpv := func(rate float64) float64 {
		sum := 0.0
		for i, f := range sorted {
			if years[i] == 0 {
				continue
			}
			sum += -years[i] * f.Amount / math.Pow(1+rate, years[i]+1)
		}
		return sum
	}

	// Newton-Raphson from a reasonable starting guess.
	rate := 0.1
	converged := false
	for i := 0; i < 200; i++ {
		f := npv(rate)
		if math.Abs(f) < 1e-9 {
			converged = true
			break
		}
		d := dnpv(rate)
		if d == 0 {
			break
		}
		next := rate - f/d
		if next <= -0.999999 {
			next = (rate - 1) / 2 // keep it inside the domain where (1+rate) > 0
		}
		rate = next
	}
	if converged && !math.IsNaN(rate) && !math.IsInf(rate, 0) {
		return rate, true
	}

	// Bisection fallback over a wide, bounded range.
	lo, hi := -0.9999, 10.0
	flo, fhi := npv(lo), npv(hi)
	if math.IsNaN(flo) || math.IsNaN(fhi) || flo*fhi > 0 {
		return 0, false
	}
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		fmid := npv(mid)
		if math.Abs(fmid) < 1e-7 {
			return mid, true
		}
		if flo*fmid < 0 {
			hi = mid
			fhi = fmid
		} else {
			lo = mid
			flo = fmid
		}
	}
	return (lo + hi) / 2, true
}
