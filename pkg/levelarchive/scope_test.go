package levelarchive

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

const (
	lvl = "aaaaaaaa-0000-0000-0000-000000000001"
	grp = "bbbbbbbb-0000-0000-0000-000000000001"
)

func TestScopeNormalize(t *testing.T) {
	cases := []struct {
		name    string
		in      Scope
		wantErr bool
		want    Scope
	}{
		{"canonical passes", Scope{LevelID: lvl, GroupIDs: []string{grp}}, false, Scope{LevelID: lvl, GroupIDs: []string{grp}}},
		{"uppercase is lowered", Scope{LevelID: " AAAAAAAA-0000-0000-0000-000000000001 "}, false, Scope{LevelID: lvl}},
		{"missing level", Scope{GroupIDs: []string{grp}}, true, Scope{}},
		{"malformed level", Scope{LevelID: "kinder"}, true, Scope{}},
		{"malformed member", Scope{LevelID: lvl, EnrollmentIDs: []string{"x"}}, true, Scope{}},
		{"duplicates collapse", Scope{LevelID: lvl, TermIDs: []string{grp, grp}}, false, Scope{LevelID: lvl, TermIDs: []string{grp}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.Normalize()
			if tc.wantErr {
				if glerrors.KindOf(err) != glerrors.KindInvalid {
					t.Fatalf("Normalize() err = %v, want KindInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize() = %v", err)
			}
			if got.LevelID != tc.want.LevelID || len(got.GroupIDs) != len(tc.want.GroupIDs) || len(got.TermIDs) != len(tc.want.TermIDs) {
				t.Fatalf("Normalize() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBatchIDMustBeAUUID(t *testing.T) {
	if _, err := NormalizeBatchID("nope"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("NormalizeBatchID(nope) err = %v, want KindInvalid", err)
	}
	got, err := NormalizeBatchID(" " + "AAAAAAAA-0000-0000-0000-000000000001")
	if err != nil || got != lvl {
		t.Fatalf("NormalizeBatchID = %q, %v", got, err)
	}
}
