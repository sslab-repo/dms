package search

import "testing"

func TestPassesSemanticThreshold(t *testing.T) {
	const threshold = 0.6

	if passesSemanticThreshold(threshold-0.001, threshold) {
		t.Fatalf("expected score below threshold to be rejected")
	}
	if !passesSemanticThreshold(threshold, threshold) {
		t.Fatalf("expected score at threshold to be accepted")
	}
	if !passesSemanticThreshold(threshold+0.001, threshold) {
		t.Fatalf("expected score above threshold to be accepted")
	}
}
