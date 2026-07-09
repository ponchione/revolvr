package taskimport

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseReturnsOrderedTaskSpecsWithPreservedNotes(t *testing.T) {
	input := `# Import

## Task: Markdown parser
Create a small parser.

Keep multiline task text readable.

### Acceptance
- returns ordered task specs
- preserves notes

### Verification
- go test ./internal/taskimport

### Design Notes
Unknown sections stay in the task body.

## Task
Second task first line.
Second task second line.

### Summary
Second summary
`

	specs, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	want := []TaskSpec{
		{
			Summary: "Markdown parser",
			Task: strings.Join([]string{
				"Create a small parser.\n\nKeep multiline task text readable.",
				"### Acceptance\n- returns ordered task specs\n- preserves notes",
				"### Verification\n- go test ./internal/taskimport",
				"### Design Notes\nUnknown sections stay in the task body.",
			}, "\n\n"),
		},
		{
			Summary: "Second summary",
			Task:    "Second task first line.\nSecond task second line.",
		},
	}
	if !reflect.DeepEqual(specs, want) {
		t.Fatalf("specs = %#v, want %#v", specs, want)
	}
}

func TestParseSupportsExplicitTaskBodySection(t *testing.T) {
	input := `## Task
### Summary
Import dry run

### Task Body
Add an app-level dry-run import operation.

### Acceptance Criteria
- dry-run does not write tasks

### Verification Notes
- go test ./internal/app
`

	specs, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := len(specs), 1; got != want {
		t.Fatalf("len(specs) = %d, want %d", got, want)
	}
	if got, want := specs[0].Summary, "Import dry run"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	wantTask := strings.Join([]string{
		"Add an app-level dry-run import operation.",
		"### Acceptance Criteria\n- dry-run does not write tasks",
		"### Verification Notes\n- go test ./internal/app",
	}, "\n\n")
	if got := specs[0].Task; got != wantTask {
		t.Fatalf("task = %q, want %q", got, wantTask)
	}
}

func TestParseEmptyTaskTextReportsTaskLine(t *testing.T) {
	_, err := ParseString(`## Task: Empty

### Acceptance
- must have a real task body
`)
	if err == nil {
		t.Fatal("parse err = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "line 1") || !strings.Contains(got, "empty task text") {
		t.Fatalf("error = %q, want line context and empty task text", got)
	}
}

func TestParseMalformedSectionBeforeFirstTaskReportsLine(t *testing.T) {
	_, err := ParseString(`# Import

## Summary
No task section yet.
`)
	if err == nil {
		t.Fatal("parse err = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "line 3") || !strings.Contains(got, "expected a task section") {
		t.Fatalf("error = %q, want line context and expected task section", got)
	}
}

func TestParseDuplicateKnownSectionReportsLine(t *testing.T) {
	_, err := ParseString(`## Task
Do the work.

### Acceptance
- first

### Acceptance
- second
`)
	if err == nil {
		t.Fatal("parse err = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "line 7") || !strings.Contains(got, "duplicate Acceptance") {
		t.Fatalf("error = %q, want duplicate Acceptance line context", got)
	}
}

func TestParseEmptyInputReportsExpectedTask(t *testing.T) {
	_, err := ParseString(" \n\t\n")
	if err == nil {
		t.Fatal("parse err = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "line 1") || !strings.Contains(got, "no task sections found") {
		t.Fatalf("error = %q, want no task sections line context", got)
	}
}
