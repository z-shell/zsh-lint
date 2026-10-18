package workflowcontract

import (
	"regexp"
	"strings"
)

var immutableActionSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// wikiActionPins keeps the allowed action identities without duplicating
// Renovate-managed digests. The workflow remains the pin's single owner.
func wikiActionPins(workflow string) (map[string]string, []string) {
	allowed := map[string]bool{
		"actions/create-github-app-token": true,
		"actions/checkout":                true,
		"actions/setup-go":                true,
		"peter-evans/create-pull-request": true,
	}
	pins := make(map[string]string)
	var violations []string
	for _, line := range strings.Split(workflow, "\n") {
		text := strings.TrimSpace(line)
		if !strings.HasPrefix(text, "uses: ") {
			continue
		}
		pin := strings.TrimPrefix(text, "uses: ")
		ref := actionRef(pin)
		action, revision, ok := strings.Cut(ref, "@")
		if !ok || !allowed[action] || !immutableActionSHA.MatchString(revision) {
			violations = append(violations, "wiki sync action must retain its allowed identity and immutable SHA: "+ref)
			continue
		}
		if previous, found := pins[action]; found && previous != pin {
			violations = append(violations, "wiki sync repeated action must use one pin: "+action)
		} else {
			pins[action] = pin
		}
	}
	for action := range allowed {
		if _, ok := pins[action]; !ok {
			violations = append(violations, "wiki sync is missing pinned action: "+action)
		}
	}
	return pins, violations
}
