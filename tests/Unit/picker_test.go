package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tags "github.com/hyz-is/arandu-tags"
)

// The picker is the one fragment an application draws inside a page of its
// own, and a page can hold more than one of them: the labels of a case beside
// the labels of each of its parties. These tests hold what keeps two of them
// apart on one page.

func TestTwoPickersOnOnePageDrawDifferentIDs(t *testing.T) {
	t.Parallel()

	row := tags.Row{ID: "label-1"}

	// Empty is the id this fragment has always drawn, so a page that never set
	// a prefix draws what it drew before.
	if got := (tags.PickerData{}).CheckboxID(row); got != "tag-label-1" {
		t.Fatalf("with no prefix the checkbox is %q, want the id it always had, tag-label-1", got)
	}

	caseID := (tags.PickerData{IDPrefix: "case-"}).CheckboxID(row)
	partyID := (tags.PickerData{IDPrefix: "party-"}).CheckboxID(row)
	if caseID == partyID {
		t.Fatalf("two pickers with two prefixes drew one id, %q", caseID)
	}
	if caseID != "case-tag-label-1" || partyID != "party-tag-label-1" {
		t.Fatalf("the prefixed ids are %q and %q, want case-tag-label-1 and party-tag-label-1", caseID, partyID)
	}
}

func TestThePublishedPickerDrawsItsIDsThroughThePrefix(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(packageRoot(t), "resources", "publish", "picker.kyse.go"))
	if err != nil {
		t.Fatalf("reading the picker: %v", err)
	}
	// The id is composed in Go, where the test above holds it, and the markup
	// writes the result. A second composition in the markup would be one the
	// prefix does not reach.
	if !strings.Contains(string(body), "ID:      .CheckboxID(row),") {
		t.Fatal("the picker does not draw its checkbox ids with PickerData.CheckboxID")
	}
	if strings.Contains(string(body), `"tag-" + row.ID`) {
		t.Fatal("the picker composes a checkbox id the prefix does not reach")
	}
}
