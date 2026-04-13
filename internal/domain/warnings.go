package domain

func AppendWarning(existing []Warning, candidate Warning) []Warning {
	for _, warning := range existing {
		if warning.Code == candidate.Code && warning.Message == candidate.Message {
			return existing
		}
	}
	return append(existing, candidate)
}

func AppendWarnings(existing []Warning, candidates ...Warning) []Warning {
	for _, candidate := range candidates {
		existing = AppendWarning(existing, candidate)
	}
	return existing
}

func DedupeWarnings(warnings []Warning) []Warning {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		out = AppendWarning(out, warning)
	}
	return out
}
