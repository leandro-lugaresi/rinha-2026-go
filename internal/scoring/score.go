package scoring

const (
	fraudThreshold = 0.6
	fraudLabel     = "fraud"
)

// ComputeScore computes the fraud score as the fraction of fraud labels
// among the nearest neighbors, and returns whether the transaction is approved.
// approved is true when fraudScore < 0.6.
func ComputeScore(nearestLabels []string) (fraudScore float64, approved bool) {
	if len(nearestLabels) == 0 {
		return 0.0, true
	}

	fraudCount := 0
	for _, label := range nearestLabels {
		if label == fraudLabel {
			fraudCount++
		}
	}

	fraudScore = float64(fraudCount) / float64(len(nearestLabels))
	approved = fraudScore < fraudThreshold
	return fraudScore, approved
}
