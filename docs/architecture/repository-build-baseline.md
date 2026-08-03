# Repository and Build Baseline

Revolvr will evolve in this repository in place. This preserves the existing
Git history, working CLI, and package layout while later bounded tasks replace
implementation that conflicts with the accepted architecture decisions. The
suggested structure in Section 8 of the canonical architecture specification
is a direction for that work, not a description of the repository today.

## Go and CLI

- Module: `revolvr`
- Go version: Go 1.26.5
- CLI entry point: `cmd/revolvr/main.go`

The canonical repository-root commands are:

```bash
go build ./...
go test ./...
go run ./cmd/revolvr --help
```

The CLI remains the complete operational surface as required by ADR-020. The
accepted Go persistence direction in ADR-005 will be introduced by its own
bounded tasks; this baseline does not imply that the current persistence code
already implements it.

## Continuous Integration

`.github/workflows/ci.yml` is the current CI authority. It uses the Go version
declared in `go.mod`, runs the full, focused autonomous, and race test suites,
checks modules and `go vet`, exercises the fake-Codex smoke paths, and builds
the supported Unix targets plus the unsupported-platform diagnostic stub. The
workflow itself remains the source of truth for the exact job matrix.

## Repository Structure

Preserve working directories and package boundaries until a bounded task owns
their replacement. Add directories from the suggested Section 8 structure
only when implementing the contents they hold. Do not create empty scaffolding
or a Makefile that merely wraps the canonical Go commands.
