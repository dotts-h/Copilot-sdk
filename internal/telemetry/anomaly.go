package telemetry

import "sort"

// This file is the cost-anomaly reader (issue 0097, roadmap v16 P2): the "noticed
// but not stopped" dual of the per-run budget cap (ADR-0053). The cap stops a run
// crossing a hard ceiling; the anomaly signal *notices* a run that merely cost far
// more than its workflow normally does — the slow burn a binary cap threshold
// misses. The converged minimum-viable form for a single-user tool is a
// deterministic ratio threshold, not a statistical/ML model (the roadmap-v16
// research found anomaly-detection frequency/thresholds are unstandardized): a run
// is anomalous when its total credits reach a multiple of its same-workflow
// cohort's MEDIAN cost. The median (not the mean) is the robust baseline — one
// runaway run barely moves it, so the runaway doesn't mask itself. Keyed on
// WorkflowID, the same per-workflow discipline as the run-comparison reader
// (rundelta.go). A pure reader over a record slice — no IO, no schema change.

// Default anomaly-detection parameters for a single-user tool: flag a run that
// cost at least 3× its workflow's typical run, once there are at least 3 prior
// runs of that workflow to form a baseline. Tuned to be quiet — it should fire on
// a genuine runaway, not normal run-to-run variance.
const (
	DefaultAnomalyFactor    = 3.0
	DefaultAnomalyMinCohort = 3
)

// Anomaly flags a run whose total cost is out of band versus its workflow's own
// baseline. It carries the figures a surface needs to explain the flag: the run's
// credits, the cohort baseline it exceeded, and by how many times.
type Anomaly struct {
	RunID      string
	WorkflowID string
	Name       string
	Credits    float64 // the run's total metered credits (RunRecord.Credits)
	Baseline   float64 // the median credits of the run's same-workflow cohort
	Factor     float64 // Credits / Baseline — how many× the typical run cost
}

// AnomalyOpts configures DetectAnomalies. Factor is the multiple of the
// per-workflow baseline at or above which a run is flagged; MinCohort is the
// minimum number of same-workflow runs needed before any of them can be judged
// (below it there is too little history to call a run unusual).
type AnomalyOpts struct {
	Factor    float64
	MinCohort int
}

// DefaultAnomalyOpts returns the sane single-user defaults so callers needn't
// re-derive them.
func DefaultAnomalyOpts() AnomalyOpts {
	return AnomalyOpts{Factor: DefaultAnomalyFactor, MinCohort: DefaultAnomalyMinCohort}
}

// DetectAnomalies flags runs whose total credits reach opts.Factor× the median
// cost of their same-workflow cohort. Pure and deterministic: same records → same
// result, sorted worst (highest factor) first, ties broken by RunID. A workflow
// with fewer than opts.MinCohort runs — or a non-positive baseline (a free cohort)
// — yields no anomaly: too little history, or no cost to compare against.
func DetectAnomalies(records []RunRecord, opts AnomalyOpts) []Anomaly {
	if opts.Factor <= 0 || opts.MinCohort < 1 {
		return nil
	}
	// Group each run's credits (and the record) by workflow, preserving order.
	byWorkflow := map[string][]RunRecord{}
	order := []string{}
	for _, r := range records {
		if _, seen := byWorkflow[r.WorkflowID]; !seen {
			order = append(order, r.WorkflowID)
		}
		byWorkflow[r.WorkflowID] = append(byWorkflow[r.WorkflowID], r)
	}

	var out []Anomaly
	for _, wf := range order {
		cohort := byWorkflow[wf]
		if len(cohort) < opts.MinCohort {
			continue // too little history to judge this workflow
		}
		credits := make([]float64, len(cohort))
		for i, r := range cohort {
			credits[i] = r.Credits()
		}
		baseline := median(credits)
		if baseline <= 0 {
			continue // a free (or unmetered) cohort — no ratio to compute
		}
		for _, r := range cohort {
			c := r.Credits()
			factor := c / baseline
			if factor >= opts.Factor {
				out = append(out, Anomaly{
					RunID: r.ID, WorkflowID: r.WorkflowID, Name: r.Name,
					Credits: c, Baseline: baseline, Factor: factor,
				})
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Factor != out[j].Factor {
			return out[i].Factor > out[j].Factor // worst first
		}
		return out[i].RunID < out[j].RunID
	})
	return out
}

// median returns the middle value of vs (the mean of the two middle values for an
// even count). It does not mutate vs. An empty slice returns 0.
func median(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	s := make([]float64, len(vs))
	copy(s, vs)
	sort.Float64s(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}
