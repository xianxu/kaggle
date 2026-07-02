package kaggle

import (
	"errors"
	"strings"
	"testing"
)

// CredentialSource is PURE: no t.Setenv, no temp HOME, no filesystem here. The
// env-read + os.Stat that feed it live in internal/kagglecli and are tested there.
func TestCredentialSource(t *testing.T) {
	cases := []struct {
		name          string
		username, key string
		fileExists    bool
		wantPresent   bool
	}{
		{"env pair", "u", "k", false, true},
		{"file only", "", "", true, true},
		{"nothing", "", "", false, false},
		{"partial env (user only)", "u", "", false, false},
		{"partial env + file", "u", "", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			present, err := CredentialSource(tc.username, tc.key, tc.fileExists)
			if present != tc.wantPresent {
				t.Fatalf("present = %v, want %v", present, tc.wantPresent)
			}
			if tc.wantPresent {
				if err != nil {
					t.Fatalf("present case returned err: %v", err)
				}
			} else if !errors.Is(err, ErrNoCredentials) {
				t.Fatalf("absent case: want ErrNoCredentials, got %v", err)
			}
		})
	}

	// The error names BOTH auth mechanisms so a stuck user knows their options.
	msg := ErrNoCredentials.Error()
	for _, want := range []string{"KAGGLE_USERNAME", "KAGGLE_KEY", "kaggle.json"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrNoCredentials message missing %q: %s", want, msg)
		}
	}
}
