package github

import (
	"errors"
	"testing"
)

func TestSplitRepository(t *testing.T) {
	cases := []struct {
		in          string
		owner, repo string
		wantErr     bool
	}{
		{in: "ColdCrabby/presets", owner: "ColdCrabby", repo: "presets"},
		{in: "  owner/name  ", owner: "owner", repo: "name"},
		{in: "noslash", wantErr: true},
		{in: "/name", wantErr: true},
		{in: "owner/", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, c := range cases {
		owner, repo, err := splitRepository(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("splitRepository(%q) = nil error, want error", c.in)
			}
			if !errors.Is(err, ErrInvalidConfig) && err != nil {
				t.Errorf("splitRepository(%q) error not ErrInvalidConfig: %v", c.in, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("splitRepository(%q) unexpected error: %v", c.in, err)
			continue
		}
		if owner != c.owner || repo != c.repo {
			t.Errorf("splitRepository(%q) = %q,%q; want %q,%q", c.in, owner, repo, c.owner, c.repo)
		}
	}
}
