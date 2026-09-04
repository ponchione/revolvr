# Bubble Tea v1.3.4 local patch

This directory contains the production source, module files, and MIT license
from `github.com/charmbracelet/bubbletea` v1.3.4. Tests, examples, and upstream
repository metadata are omitted.

- Upstream tag: `v1.3.4`
- Upstream commit: `bf1216dfaf642b73b639262ab91a7e7c86095d34`
- Go module sum: `h1:kCg7B+jSCFPLYRA52SDZjr51kG/fMUEoPoZrkaDHyoI=`
- Downloaded module ZIP SHA-256:
  `17ad36a3daad753e2c0b0a77e4e74da811cbc4e53b35c95560967df822a9905c`

The only source change is `tea_init.go`: Revolvr removes Bubble Tea v1's
unconditional `lipgloss.HasDarkBackground()` package initializer. That
initializer emits terminal queries before `main`, which makes it impossible
for Revolvr's stdin-first/stdout-second TTY gate to reject redirected launches
without prior terminal output.

Remove this replacement when Revolvr migrates to a Bubble Tea version that
does not query the terminal during package initialization.
