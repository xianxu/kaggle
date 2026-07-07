package submit

import (
	"testing"
	"time"
)

func TestEnvInt(t *testing.T) {
	if got := EnvInt("KAGGLE_TEST_UNSET_INT", 7); got != 7 {
		t.Errorf("unset → %d, want default 7", got)
	}
	t.Setenv("KAGGLE_TEST_INT", "")
	if got := EnvInt("KAGGLE_TEST_INT", 7); got != 7 {
		t.Errorf("empty → %d, want default 7", got)
	}
	t.Setenv("KAGGLE_TEST_INT", "12")
	if got := EnvInt("KAGGLE_TEST_INT", 7); got != 12 {
		t.Errorf("valid → %d, want 12", got)
	}
	t.Setenv("KAGGLE_TEST_INT", "notanint")
	if got := EnvInt("KAGGLE_TEST_INT", 7); got != 7 {
		t.Errorf("malformed → %d, want default 7 (warns to stderr)", got)
	}
}

func TestEnvDuration(t *testing.T) {
	def := 5 * time.Second
	cases := []struct {
		val  string // "" via a distinct unset key
		set  bool
		want time.Duration
	}{
		{set: false, want: def},                                 // unset → default
		{val: "0", set: true, want: 0},                          // Go duration "0"
		{val: "250ms", set: true, want: 250 * time.Millisecond}, // Go duration
		{val: "3", set: true, want: 3 * time.Second},            // bare integer → seconds
		{val: "garbage", set: true, want: def},                  // malformed → default (warns)
	}
	for _, c := range cases {
		if c.set {
			t.Setenv("KAGGLE_TEST_DUR", c.val)
			if got := EnvDuration("KAGGLE_TEST_DUR", def); got != c.want {
				t.Errorf("EnvDuration(%q) = %v; want %v", c.val, got, c.want)
			}
		} else {
			if got := EnvDuration("KAGGLE_TEST_DUR_UNSET", def); got != c.want {
				t.Errorf("EnvDuration(unset) = %v; want %v", got, c.want)
			}
		}
	}
}
