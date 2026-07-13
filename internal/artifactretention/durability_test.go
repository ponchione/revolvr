package artifactretention

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type applyFailureObservation struct {
	point ApplyFailurePoint
	path  string
}

func TestPruneDurabilityBoundariesAreOrderedAndRecoverable(t *testing.T) {
	root, ledgerPath, logical, plan := prunePlan(t, "durability-order")
	var observed []applyFailureObservation
	result, err := ApplyGC(context.Background(), ApplyInput{
		RepositoryRoot: root,
		LedgerPath:     ledgerPath,
		Plan:           plan,
		Clock:          func() time.Time { return plan.FrozenAt },
		FailureInjector: func(point ApplyFailurePoint, path string) error {
			observed = append(observed, applyFailureObservation{point: point, path: path})
			return nil
		},
	})
	if err != nil || result.Journal.Stage != stageCleaned {
		t.Fatalf("ApplyGC() = %+v, %v", result, err)
	}
	want := expectedPruneDurabilityOrder(root, plan.OperationID, logical)
	if !reflect.DeepEqual(observed, want) {
		t.Fatalf("durability order =\n%+v\nwant\n%+v", observed, want)
	}

	for crashIndex, boundary := range want {
		t.Run(fmt.Sprintf("%02d-%s", crashIndex, boundary.point), func(t *testing.T) {
			root, ledgerPath, logical, plan := prunePlan(t, fmt.Sprintf("durability-crash-%02d", crashIndex))
			crash := errors.New("simulated crash")
			calls := 0
			_, err := ApplyGC(context.Background(), ApplyInput{
				RepositoryRoot: root,
				LedgerPath:     ledgerPath,
				Plan:           plan,
				Clock:          func() time.Time { return plan.FrozenAt },
				FailureInjector: func(ApplyFailurePoint, string) error {
					if calls == crashIndex {
						calls++
						return crash
					}
					calls++
					return nil
				},
			})
			if !errors.Is(err, crash) || calls != crashIndex+1 {
				t.Fatalf("crash result err=%v calls=%d, want injected boundary %d", err, calls, crashIndex)
			}

			var recovery []applyFailureObservation
			recovered, err := ApplyGC(context.Background(), ApplyInput{
				RepositoryRoot: root,
				LedgerPath:     ledgerPath,
				Plan:           plan,
				Clock:          func() time.Time { return plan.FrozenAt },
				FailureInjector: func(point ApplyFailurePoint, path string) error {
					recovery = append(recovery, applyFailureObservation{point: point, path: path})
					return nil
				},
			})
			if err != nil || recovered.Journal.Stage != stageCleaned {
				t.Fatalf("recovery = %+v, %v; observations=%+v", recovered, err, recovery)
			}
			if crashIndex == 1 && (len(recovery) == 0 || recovery[0].point != FailureAfterPruneSourceSync) {
				t.Fatalf("filesystem-ahead retry did not resync source first: %+v", recovery)
			}
			assertPruneCleaned(t, root, plan.OperationID, logical)
		})
	}
}

func expectedPruneDurabilityOrder(root, operationID, logical string) []applyFailureObservation {
	operationDir := filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(operationID))
	base := filepath.Join(operationDir, "quarantine", filepath.FromSlash(logical))
	abs := filepath.Join(root, filepath.FromSlash(logical))
	result := []applyFailureObservation{
		{point: FailureAfterPruneRename, path: base + ".gz"},
		{point: FailureAfterPruneRename, path: base + ".gz.manifest.json"},
		{point: FailureAfterPruneSourceSync, path: filepath.Dir(abs)},
	}
	for dir := filepath.Dir(base); ; dir = filepath.Dir(dir) {
		result = append(result, applyFailureObservation{point: FailureAfterPruneDestinationSync, path: dir})
		if dir == operationDir {
			break
		}
	}
	result = append(result,
		applyFailureObservation{point: FailureAfterActionJournal, path: logical},
		applyFailureObservation{point: FailureAfterCompletedJournal, path: operationID},
		applyFailureObservation{point: FailureAfterQuarantineRemoval, path: filepath.Join(operationDir, "quarantine")},
		applyFailureObservation{point: FailureAfterQuarantineParentSync, path: operationDir},
		applyFailureObservation{point: FailureAfterCleanedJournal, path: operationID},
	)
	return result
}

func assertPruneCleaned(t *testing.T, root, operationID, logical string) {
	t.Helper()
	for _, suffix := range []string{"", ".gz", ".gz.manifest.json"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(logical)) + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("source representation %q remains: %v", suffix, err)
		}
	}
	quarantine := filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(operationID), "quarantine")
	if _, err := os.Lstat(quarantine); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantine remains after cleanup: %v", err)
	}
	journal, found, err := InspectGC(root, operationID)
	if err != nil || !found || journal.Stage != stageCleaned {
		t.Fatalf("InspectGC() = %+v, %t, %v", journal, found, err)
	}
}
