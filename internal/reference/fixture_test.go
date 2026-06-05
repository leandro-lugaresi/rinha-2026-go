package reference_test

import (
	"os"
	"testing"

	"github.com/leandro-lugaresi/rinha-2026-go/internal/reference"
)

const fixturePath = "../../.context/rinha-de-backend-2026/resources/example-references.json"

func TestExampleReferencesLoad(t *testing.T) {
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skipf("fixture file not found: %s", fixturePath)
	}

	refs, err := reference.LoadExampleReferences(fixturePath)
	if err != nil {
		t.Fatalf("LoadExampleReferences: %v", err)
	}

	t.Run("returns_all_entries", func(t *testing.T) {
		if len(refs) == 0 {
			t.Fatal("expected non-zero references")
		}
		if len(refs) != 100 {
			t.Errorf("expected 100 references, got %d", len(refs))
		}
	})

	t.Run("first_entry_is_correct", func(t *testing.T) {
		first := refs[0]
		if first.Label != "legit" {
			t.Errorf("first label: want legit, got %s", first.Label)
		}
		if first.Vector[0] != 0.01 {
			t.Errorf("first vector[0]: want 0.01, got %v", first.Vector[0])
		}
	})

	t.Run("all_vectors_have_14_dims", func(t *testing.T) {
		for i, ref := range refs {
			allZero := true
			for _, v := range ref.Vector {
				if v != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				t.Errorf("reference %d has all-zero vector", i)
			}
		}
	})

	t.Run("both_labels_present", func(t *testing.T) {
		var legit, fraud int
		for _, ref := range refs {
			switch ref.Label {
			case "legit":
				legit++
			case "fraud":
				fraud++
			default:
				t.Errorf("unexpected label: %s", ref.Label)
			}
		}
		if legit == 0 {
			t.Error("no legit references found")
		}
		if fraud == 0 {
			t.Error("no fraud references found")
		}
		t.Logf("legit=%d fraud=%d", legit, fraud)
	})
}

func TestLoadExampleReferencesErrors(t *testing.T) {
	t.Run("nonexistent_file", func(t *testing.T) {
		_, err := reference.LoadExampleReferences("/nonexistent/path.json")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})
}
