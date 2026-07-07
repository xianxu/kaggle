package main

import "testing"

func TestSlugFromRecordJSON(t *testing.T) {
	cases := []struct {
		name string
		json string
		slug string
		ok   bool
	}{
		{
			name: "download step carries the slug",
			json: `{"steps":[{"step_id":"download","with":{"competition":{"slug":"titanic","metric":"accuracy"}}},{"step_id":"train","with":{"model":"logreg"}}]}`,
			slug: "titanic",
			ok:   true,
		},
		{
			name: "first kaggle step wins",
			json: `{"steps":[{"with":{"model":"x"}},{"with":{"competition":{"slug":"spaceship"}}}]}`,
			slug: "spaceship",
			ok:   true,
		},
		{name: "no kaggle step", json: `{"steps":[{"with":{"model":"logreg"}}]}`, ok: false},
		{name: "no steps", json: `{"steps":[]}`, ok: false},
		{name: "malformed json", json: `{not json`, ok: false},
		{name: "empty slug", json: `{"steps":[{"with":{"competition":{"slug":""}}}]}`, ok: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			slug, ok := slugFromRecordJSON([]byte(c.json))
			if ok != c.ok || slug != c.slug {
				t.Errorf("slugFromRecordJSON = (%q, %v); want (%q, %v)", slug, ok, c.slug, c.ok)
			}
		})
	}
}
