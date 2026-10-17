package client

import (
	"encoding/json"
	"strings"
	"testing"

	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
)

// A garment is scoped by a config level id, and an order carries the pupil's
// level id and name — the school's own levels, not a fixed SEP list.
func TestUniformLevelsTravelAsCatalogIDs(t *testing.T) {
	item := toUniformItem(&uniformsv1.Item{Id: "it-1", LevelId: "lvl-9"})
	if item.LevelID != "lvl-9" {
		t.Errorf("item levelId = %q", item.LevelID)
	}
	order := toUniformOrder(&uniformsv1.Order{Id: "o-1", StudentLevelId: "lvl-9", StudentLevelName: "Preescolar"})
	if order.StudentLevelID != "lvl-9" || order.StudentLevel != "Preescolar" {
		t.Errorf("order level = (%q, %q)", order.StudentLevelID, order.StudentLevel)
	}
}

// T14-019: the seller reaches the receipt builder, and never the JSON.
func TestTheSellerIsReadButNotServed(t *testing.T) {
	order := toUniformOrder(&uniformsv1.Order{Id: "o-1", CreatedByUserId: "usr-7"})
	if order.CreatedByUserID != "usr-7" {
		t.Errorf("created by = %q, want usr-7", order.CreatedByUserID)
	}
	body, err := json.Marshal(order)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "usr-7") {
		t.Errorf("a staff id reached the wire: %s", body)
	}
}
