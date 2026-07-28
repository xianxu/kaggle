//go:build live

// Live conformance check (ARCH-MOCK): the scheduled comparison between the fake's
// modeled behavior and the REAL Kaggle CLI, so schema drift is DETECTED rather than
// re-discovered by a human months later.
//
// It exists because that gap was real: the pre-kaggle#8 schema was authored from CLI
// docs and turned out wrong in four ways (status vocabulary, a dropped `ref` column,
// the date shape, and COMPLETE-without-score), two of which silently disabled
// production code paths on live while every fake-backed test stayed green.
//
// Placement is deliberate:
//   - HERE, not in pkg/kaggle, because it performs IO (pkg/kaggle is IO-free, ARCH-PURE);
//   - it drives the PRODUCTION seam CLI.Submissions() rather than re-implementing
//     binary resolution and exec, so it validates the path we actually ship (ARCH-DRY);
//   - it asserts against the exported kaggle.SubmissionsCSVHeader(), the single schema
//     definition, rather than restating the column list.
//
// Run:
//
//	KAGGLE_LIVE_CONFORMANCE=1 KAGGLE_CONFORMANCE_SLUG=<competition> \
//	  go test -tags live ./internal/kagglecli/ -run Conformance -v
//
// It SKIPS (never fails) when the opt-in, the slug, credentials, or rows are absent —
// an absent live environment is not a regression. It REFUSES to run against the fake:
// a conformance check that can pass against our own fake tests nothing.
package kagglecli

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/xianxu/kaggle/pkg/kaggle"
)

// statusWireShape is the observed Python enum repr, built from the SINGLE source of the
// wire prefix (kaggle.StatusWirePrefix) rather than restating it — the prefix is the half
// of the schema that was actually wrong before kaggle#8.
var statusWireShape = regexp.MustCompile(`^` + regexp.QuoteMeta(kaggle.StatusWirePrefix) + `[A-Z]+$`)

func TestLiveConformance_SubmissionsSchema(t *testing.T) {
	if os.Getenv("KAGGLE_LIVE_CONFORMANCE") != "1" {
		t.Skip("live conformance is opt-in: set KAGGLE_LIVE_CONFORMANCE=1")
	}
	slug := os.Getenv("KAGGLE_CONFORMANCE_SLUG")
	if slug == "" {
		t.Skip("set KAGGLE_CONFORMANCE_SLUG to a competition you have submissions in")
	}
	// A conformance check that can be satisfied by our own fake is worthless.
	if bin := os.Getenv("KAGGLE_CLI"); bin != "" && strings.Contains(bin, "fake") {
		t.Fatalf("KAGGLE_CLI=%q looks like the fake — live conformance must run against the real CLI", bin)
	}

	// New() — not CLI{} — so the check runs through the same ${KAGGLE_CLI:-kaggle}
	// resolution production uses (ARCH-DRY: reuse the seam, don't reimplement it).
	out, err := New().Submissions(slug)
	if err != nil {
		t.Skipf("live CLI unavailable (credentials/network/competition): %v", err)
	}
	header, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	if header == "" {
		t.Skip("no output from the live CLI")
	}

	// 1. Column set matches the one schema definition.
	got := strings.Split(header, ",")
	want := kaggle.SubmissionsCSVHeader()
	gotSet := map[string]bool{}
	for _, c := range got {
		gotSet[strings.TrimSpace(c)] = true
	}
	for _, c := range want {
		if !gotSet[c] {
			t.Errorf("live header is missing %q (got %v) — the fixture and fake are now stale;"+
				" re-capture into workshop/captures/ and update pkg/kaggle/testdata", c, got)
		}
	}
	for _, c := range got {
		c = strings.TrimSpace(c)
		known := false
		for _, w := range want {
			if c == w {
				known = true
			}
		}
		if !known {
			t.Errorf("live header has an UNMODELED column %q (got %v) — Kaggle added a field", c, got)
		}
	}

	// 2. Status values still carry the enum-repr shape our normalization assumes.
	subs, err := kaggle.ParseSubmissions(out)
	if err != nil {
		t.Fatalf("live output no longer parses: %v", err)
	}
	if len(subs) == 0 {
		t.Skip("account has no submissions in this competition — nothing to assert on rows")
	}
	for _, s := range subs {
		if s.StatusRaw == "" {
			continue
		}
		if !statusWireShape.MatchString(s.StatusRaw) {
			t.Errorf("status %q no longer matches %s — NormalizeStatus's prefix assumption is stale",
				s.StatusRaw, statusWireShape)
		}
		if s.Status == "" {
			t.Errorf("status %q normalized to empty", s.StatusRaw)
		}
	}
	t.Logf("live conformance OK: %d rows, header %v", len(subs), got)
}
