package scoring

import (
	"math"
	"testing"
)

func TestScoreThreshold_AllLegit(t *testing.T) {
	labels := []string{"legit", "legit", "legit", "legit", "legit"}
	fraudScore, approved := ComputeScore(labels)

	if fraudScore != 0.0 {
		t.Errorf("fraudScore = %f, want 0.0", fraudScore)
	}
	if !approved {
		t.Errorf("approved = false, want true (score 0.0 < 0.6)")
	}
}

func TestScoreThreshold_AllFraud(t *testing.T) {
	labels := []string{"fraud", "fraud", "fraud", "fraud", "fraud"}
	fraudScore, approved := ComputeScore(labels)

	if fraudScore != 1.0 {
		t.Errorf("fraudScore = %f, want 1.0", fraudScore)
	}
	if approved {
		t.Errorf("approved = true, want false (score 1.0 >= 0.6)")
	}
}

func TestScoreThreshold_Mixed(t *testing.T) {
	// 2 frauds out of 5 = 0.4 → approved
	labels := []string{"fraud", "legit", "fraud", "legit", "legit"}
	fraudScore, approved := ComputeScore(labels)

	wantScore := 0.4
	if math.Abs(fraudScore-wantScore) > 0.0001 {
		t.Errorf("fraudScore = %f, want %f", fraudScore, wantScore)
	}
	if !approved {
		t.Errorf("approved = false, want true (score 0.4 < 0.6)")
	}
}

func TestScoreThreshold_BoundaryBelow(t *testing.T) {
	// Exactly 3 frauds out of 5 = 0.6 → NOT approved (threshold is < 0.6, not <= 0.6)
	labels := []string{"fraud", "fraud", "fraud", "legit", "legit"}
	fraudScore, approved := ComputeScore(labels)

	wantScore := 0.6
	if math.Abs(fraudScore-wantScore) > 0.0001 {
		t.Errorf("fraudScore = %f, want %f", fraudScore, wantScore)
	}
	if approved {
		t.Errorf("approved = true, want false (score 0.6 is NOT < 0.6)")
	}
}

func TestScoreThreshold_BoundaryAbove(t *testing.T) {
	// 4 frauds out of 5 = 0.8 → NOT approved
	labels := []string{"fraud", "fraud", "fraud", "fraud", "legit"}
	fraudScore, approved := ComputeScore(labels)

	wantScore := 0.8
	if math.Abs(fraudScore-wantScore) > 0.0001 {
		t.Errorf("fraudScore = %f, want %f", fraudScore, wantScore)
	}
	if approved {
		t.Errorf("approved = true, want false (score 0.8 >= 0.6)")
	}
}

func TestScoreThreshold_EmptyLabels(t *testing.T) {
	// Edge case: no labels → score 0, approved (safe default)
	labels := []string{}
	fraudScore, approved := ComputeScore(labels)

	if fraudScore != 0.0 {
		t.Errorf("fraudScore = %f, want 0.0 (empty labels)", fraudScore)
	}
	if !approved {
		t.Errorf("approved = true, want true (empty labels → safe default)")
	}
}

func TestScoreThreshold_VariableK(t *testing.T) {
	// Test with non-5 labels to verify the function uses actual length
	labels := []string{"fraud", "fraud", "legit"} // 2/3 = 0.666...
	fraudScore, approved := ComputeScore(labels)

	wantScore := 2.0 / 3.0
	if math.Abs(fraudScore-wantScore) > 0.0001 {
		t.Errorf("fraudScore = %f, want %f", fraudScore, wantScore)
	}
	if approved {
		t.Errorf("approved = true, want false (score %.4f >= 0.6)", fraudScore)
	}
}
