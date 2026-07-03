package kaggle

import "testing"

func TestCompetitionValidate(t *testing.T) {
	if err := (Competition{Slug: ""}).Validate(); err == nil {
		t.Fatal("empty slug: want error, got nil")
	}
	if err := (Competition{Slug: "titanic"}).Validate(); err != nil {
		t.Fatalf("valid competition: want nil, got %v", err)
	}
}
