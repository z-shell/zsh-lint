package workflowcontract

import (
	"os"
	"testing"
)

func isFailedTagReleaseProbe(repository, refType, refName string) bool {
	return repository == "z-shell/zsh-lint" &&
		refType == "tag" &&
		refName == "v0.0.0"
}

func TestIsFailedTagReleaseProbe(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		refType    string
		refName    string
		want       bool
	}{
		{
			name:       "exact sentinel",
			repository: "z-shell/zsh-lint",
			refType:    "tag",
			refName:    "v0.0.0",
			want:       true,
		},
		{name: "local environment"},
		{
			name:       "another repository",
			repository: "fork/zsh-lint",
			refType:    "tag",
			refName:    "v0.0.0",
		},
		{
			name:       "branch ref",
			repository: "z-shell/zsh-lint",
			refType:    "branch",
			refName:    "v0.0.0",
		},
		{
			name:       "future release tag",
			repository: "z-shell/zsh-lint",
			refType:    "tag",
			refName:    "v1.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFailedTagReleaseProbe(tt.repository, tt.refType, tt.refName)
			if got != tt.want {
				t.Fatalf("isFailedTagReleaseProbe() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestFailedTagReleaseProbe intentionally fails only for the v0.0.0 exercise
// tracked in https://github.com/z-shell/zsh-lint/issues/90.
func TestFailedTagReleaseProbe(t *testing.T) {
	if !isFailedTagReleaseProbe(
		os.Getenv("GITHUB_REPOSITORY"),
		os.Getenv("GITHUB_REF_TYPE"),
		os.Getenv("GITHUB_REF_NAME"),
	) {
		return
	}

	t.Fatal("intentional z-shell/zsh-lint#90 release-gate probe: v0.0.0 must not publish")
}
