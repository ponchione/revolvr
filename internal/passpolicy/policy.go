package passpolicy

import (
	"fmt"
	"strings"

	"revolvr/internal/taskfile"
)

const (
	ProfileImplementer = "implementer"
	ProfileAuditor     = "auditor"
	ProfileDocumentor  = "documentor"
	ProfileSimplifier  = "simplifier"
)

type Policy struct {
	Workflow             string
	Phase                string
	ProfileName          string
	AllowNoChangeSuccess bool
	NextPhase            string
	CompletesTask        bool
}

func Lookup(workflow string, phase string) (Policy, error) {
	workflow = strings.TrimSpace(workflow)
	phase = strings.TrimSpace(phase)

	if workflow != taskfile.WorkflowMixedPassV1 {
		return Policy{}, fmt.Errorf("lookup pass policy: unsupported workflow %q", workflow)
	}

	switch phase {
	case taskfile.PhaseImplement:
		return Policy{
			Workflow:             workflow,
			Phase:                phase,
			ProfileName:          ProfileImplementer,
			AllowNoChangeSuccess: false,
			NextPhase:            taskfile.PhaseAudit,
		}, nil
	case taskfile.PhaseAudit:
		return Policy{
			Workflow:             workflow,
			Phase:                phase,
			ProfileName:          ProfileAuditor,
			AllowNoChangeSuccess: true,
			NextPhase:            taskfile.PhaseDocument,
		}, nil
	case taskfile.PhaseDocument:
		return Policy{
			Workflow:             workflow,
			Phase:                phase,
			ProfileName:          ProfileDocumentor,
			AllowNoChangeSuccess: true,
			NextPhase:            taskfile.PhaseSimplify,
		}, nil
	case taskfile.PhaseSimplify:
		return Policy{
			Workflow:             workflow,
			Phase:                phase,
			ProfileName:          ProfileSimplifier,
			AllowNoChangeSuccess: true,
			CompletesTask:        true,
		}, nil
	default:
		return Policy{}, fmt.Errorf("lookup pass policy: unsupported phase %q for workflow %q", phase, workflow)
	}
}
