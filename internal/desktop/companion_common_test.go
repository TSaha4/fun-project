package desktop

import (
	"strings"
	"testing"

	"example.com/local-capture/internal/domain"
)

func TestSuggestedTitle(t *testing.T) {
	if got := SuggestedTitle("  A title  \nsecond line"); got != "A title" {
		t.Fatalf("SuggestedTitle() = %q", got)
	}
	if got := SuggestedTitle(" \nsecond line"); got != "Quick capture" {
		t.Fatalf("empty SuggestedTitle() = %q", got)
	}
	long := strings.Repeat("界", 100)
	if got := SuggestedTitle(long); len([]rune(got)) != 80 {
		t.Fatalf("long title rune length = %d", len([]rune(got)))
	}
}

func TestNoteTypeForIndex(t *testing.T) {
	if got := NoteTypeForIndex(3); got != domain.NoteTypeLink {
		t.Fatalf("NoteTypeForIndex(3) = %q", got)
	}
	if got := NoteTypeForIndex(-1); got != domain.NoteTypeThought {
		t.Fatalf("invalid index fallback = %q", got)
	}
}

func TestValidateDashboardURL(t *testing.T) {
	for _, value := range []string{"http://127.0.0.1:3000", "https://localhost/dashboard"} {
		if err := ValidateDashboardURL(value); err != nil {
			t.Errorf("valid URL %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "file:///tmp/page", "javascript:alert(1)", "localhost:3000"} {
		if err := ValidateDashboardURL(value); err == nil {
			t.Errorf("invalid URL %q accepted", value)
		}
	}
}

func TestSubmissionGatePreventsDuplicateInFlightSaves(t *testing.T) {
	var gate submissionGate
	if !gate.Acquire() {
		t.Fatal("first save did not acquire gate")
	}
	if gate.Acquire() {
		t.Fatal("duplicate save acquired gate")
	}
	if !gate.IsBusy() {
		t.Fatal("gate did not report an in-flight save")
	}
	gate.Release()
	if gate.IsBusy() {
		t.Fatal("gate stayed busy after release")
	}
	if !gate.Acquire() {
		t.Fatal("save could not be retried after release")
	}
	gate.Release()
}
