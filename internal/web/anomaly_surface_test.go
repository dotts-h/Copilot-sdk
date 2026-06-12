package web

import (
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// seedAnomalyRuns appends a quiet same-workflow cohort plus one runaway run, so
// DetectAnomalies has a baseline to flag the outlier against.
func seedAnomalyRuns(rs *telemetry.RunStore) {
	for _, id := range []string{"q1", "q2", "q3"} {
		_ = rs.Append(telemetry.RunRecord{
			ID: id, WorkflowID: "wf", Name: "Nightly", Outcome: "finished",
			Lanes: []telemetry.RunLane{{Index: 0, Credits: 1}},
		})
	}
	_ = rs.Append(telemetry.RunRecord{
		ID: "runaway", WorkflowID: "wf", Name: "Nightly", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, Credits: 9}}, // 9× the median
	})
}

// The Telemetry page lists a cost anomaly when a run is out of band, linking to the
// run inspector and ambering the factor.
func TestTelemetryPageListsAnomaly(t *testing.T) {
	s, _ := newTestServer()
	seedAnomalyRuns(withRunStore(s))

	html := s.telemetryPartial(defaultSpendWindow)

	if !strings.Contains(html, "Cost anomalies") {
		t.Fatal("the Telemetry page should carry a Cost anomalies section when a run is out of band")
	}
	if !strings.Contains(html, "/page/runs/runaway") {
		t.Error("the anomaly row should link to the offending run's inspector")
	}
	if !strings.Contains(html, "9.0×") {
		t.Errorf("the anomaly row should amber the factor (9.0×): %s", html)
	}
}

// A quiet history (no outlier) shows no anomalies section — the signal is silent
// until a run is genuinely out of band.
func TestTelemetryPageNoAnomalySectionWhenQuiet(t *testing.T) {
	s, _ := newTestServer()
	rs := withRunStore(s)
	for _, id := range []string{"a", "b", "c"} {
		_ = rs.Append(telemetry.RunRecord{
			ID: id, WorkflowID: "wf", Name: "Nightly",
			Lanes: []telemetry.RunLane{{Index: 0, Credits: 1}},
		})
	}
	if html := s.telemetryPartial(defaultSpendWindow); strings.Contains(html, "Cost anomalies") {
		t.Error("a quiet cohort should not show the anomalies section")
	}
}

// The run inspector ambers the run the anomaly points at, and leaves a normal run
// un-flagged.
func TestRunInspectorAmbersAnomaly(t *testing.T) {
	s, _ := newTestServer()
	rs := withRunStore(s)
	seedAnomalyRuns(rs)

	var runaway, quiet telemetry.RunRecord
	for _, r := range rs.Records() {
		switch r.ID {
		case "runaway":
			runaway = r
		case "q1":
			quiet = r
		}
	}

	if got := s.runDetailPartial(runaway, defaultSpendWindow, ""); !strings.Contains(got, "Cost anomaly") {
		t.Error("the inspector should amber the anomalous run")
	}
	if got := s.runDetailPartial(quiet, defaultSpendWindow, ""); strings.Contains(got, "Cost anomaly") {
		t.Error("a normal run should not be flagged as an anomaly")
	}
}
