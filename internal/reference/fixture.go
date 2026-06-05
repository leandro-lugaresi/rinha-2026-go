// Package reference provides types and loading for reference vectors used
// in KNN search validation and benchmark correctness checks.
package reference

import (
	"encoding/json"
	"fmt"
	"os"
)

// Reference holds one labeled 14-dimensional vector from the dataset.
// Dimensions follow the order defined in DETECTION_RULES.md.
type Reference struct {
	Vector [14]float64 `json:"vector"`
	Label  string      `json:"label"`
}

// rawRef is the wire format matching the JSON shape exactly.
// It uses a slice so json.Unmarshal can populate it; validation
// converts it to the fixed-size array.
type rawRef struct {
	Vector []float64 `json:"vector"`
	Label  string    `json:"label"`
}

// LoadExampleReferences reads a JSON file containing labeled reference
// vectors and returns them as a slice of Reference. Each entry must
// contain exactly 14 float64 values in its vector field.
func LoadExampleReferences(path string) ([]Reference, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read example references: %w", err)
	}

	var raw []rawRef
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse example references: %w", err)
	}

	refs := make([]Reference, 0, len(raw))
	for i, r := range raw {
		if len(r.Vector) != 14 {
			return nil, fmt.Errorf("reference %d: expected 14 dimensions, got %d", i, len(r.Vector))
		}
		var arr [14]float64
		copy(arr[:], r.Vector)
		refs = append(refs, Reference{Vector: arr, Label: r.Label})
	}

	if len(refs) == 0 {
		return nil, fmt.Errorf("no references found in %s", path)
	}

	return refs, nil
}
