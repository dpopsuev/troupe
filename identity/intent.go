package identity

import "strings"

// DomainFromKeywords returns the best-matching domain for a natural language
// intent string. Returns empty string if no strong match.
func DomainFromKeywords(intent string) string {
	lower := strings.ToLower(intent)
	keywords := map[string]string{
		"code":        Coding,
		"coding":      Coding,
		"implement":   Coding,
		"refactor":    Coding,
		"debug":       Coding,
		"fix":         Coding,
		"build":       Coding,
		"test":        Coding,
		"review":      Coding,
		"reason":      Reasoning,
		"analyze":     Reasoning,
		"investigate": Reasoning,
		"root cause":  Reasoning,
		"explain":     Reasoning,
		"research":    Knowledge,
		"learn":       Knowledge,
		"summarize":   Knowledge,
		"document":    Knowledge,
		"math":        Math,
		"calculate":   Math,
		"compute":     Math,
		"formula":     Math,
		"tool":        Agentic,
		"agentic":     Agentic,
		"autonomous":  Agentic,
		"shell":       Agentic,
		"execute":     Agentic,
	}

	for kw, domain := range keywords {
		if strings.Contains(lower, kw) {
			return domain
		}
	}
	return ""
}
