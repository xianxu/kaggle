package kagglecli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xianxu/kaggle/pkg/kaggle"
)

// buildFake compiles cmd/fake-kaggle into a temp dir and returns its path.
func buildFake(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fake-kaggle")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/xianxu/kaggle/cmd/fake-kaggle")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build fake: %v", err)
	}
	return bin
}

// TestClientAgainstFake is the M1 proof: the injectable CLI + the pure parsers +
// the process-level fake compose into a working submit → poll → scored flow, and
// the poll loop ACTUALLY ITERATES through the fake's pending→scored transition
// (scored no earlier than poll #2). This is the whole reason the fake models the
// transition rather than the terminal state.
func TestClientAgainstFake(t *testing.T) {
	fake := buildFake(t)
	t.Setenv("KAGGLE_CLI", fake)
	t.Setenv("KAGGLE_FAKE", "1")
	t.Setenv("KAGGLE_FAKE_STATE", t.TempDir())
	t.Setenv("KAGGLE_FAKE_SCORE_AFTER", "1") // poll #1 pending, poll #2 scored

	c := New()
	if err := c.Submit("titanic", "submission.csv", "baseline"); err != nil {
		t.Fatalf("submit: %v", err)
	}

	var best kaggle.Submission
	polls, scored := 0, false
	for polls = 1; polls <= 5; polls++ {
		out, err := c.Submissions("titanic")
		if err != nil {
			t.Fatalf("submissions poll %d: %v", polls, err)
		}
		subs, err := kaggle.ParseSubmissions(out)
		if err != nil {
			t.Fatalf("parse poll %d: %v", polls, err)
		}
		if b, ok := kaggle.LatestScored(subs); ok {
			best, scored = b, true
			break
		}
	}
	if !scored {
		t.Fatal("never scored after 5 polls")
	}
	if polls < 2 {
		t.Fatalf("poll loop did not iterate through pending (scored on poll %d) — fake not modeling the async transition", polls)
	}
	if best.File != "submission.csv" {
		t.Errorf("scored file = %q, want submission.csv", best.File)
	}
	if best.PublicScore == nil || *best.PublicScore != 0.775 {
		t.Fatalf("public score = %v, want 0.775", best.PublicScore)
	}
	t.Logf("scored on poll %d: %s public=%.3f", polls, best.File, *best.PublicScore)
}
