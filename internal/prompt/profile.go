package prompt

const DefaultRunProfileName = "implementer"

type RunProfile struct {
	Name        string
	Description string
}

func DefaultRunProfile() RunProfile {
	return RunProfile{
		Name: DefaultRunProfileName,
		Description: "You are the implementer for this Revolvr pass.\n\n" +
			"Focus on the selected task, make small reviewable changes, preserve existing repository state, verify the work, and write the required receipt before stopping.",
	}
}
