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

func TestThePickerTellsNoLabelsApartFromNoneCarried(t *testing.T) {
	t.Parallel()

	held := tags.Row{ID: "label-1", Held: true}
	free := tags.Row{ID: "label-2"}

	for _, c := range []struct {
		why             string
		data            tags.PickerData
		nothingToChoose bool
		noneCarried     bool
	}{
		{"a taxonomy with no label yet", tags.PickerData{}, true, false},
		{"labels, none of them carried", tags.PickerData{Available: []tags.Row{free}}, false, true},
		{"a label carried, marked by PickerRows", tags.PickerData{Available: []tags.Row{held, free}}, false, false},
		{"a label carried, listed in Carried", tags.PickerData{Available: []tags.Row{free}, Carried: []tags.Row{{ID: "label-2"}}}, false, false},
		{"labels carried and none left to offer", tags.PickerData{Carried: []tags.Row{{ID: "label-2"}}}, false, false},
	} {
		if got := c.data.NothingToChoose(); got != c.nothingToChoose {
			t.Errorf("%s: NothingToChoose is %v, want %v", c.why, got, c.nothingToChoose)
		}
		if got := c.data.NoneCarried(); got != c.noneCarried {
			t.Errorf("%s: NoneCarried is %v, want %v", c.why, got, c.noneCarried)
		}
	}
}

func TestThePickerSaysWhichEmptinessItIs(t *testing.T) {
	t.Parallel()

	m := module(t)
	for _, locale := range tags.Locales() {
		labels := m.Labels(locale)
		none, empty := labels.T("screen.picker_nothing_to_choose"), labels.T("screen.picker_empty")
		if none == "screen.picker_nothing_to_choose" || empty == "screen.picker_empty" {
			t.Fatalf("%s draws a key instead of a sentence: %q, %q", locale, none, empty)
		}
		if none == empty {
			t.Errorf("%s says %q both when there is no label and when none is carried", locale, none)
		}
	}

	body, err := os.ReadFile(filepath.Join(packageRoot(t), "resources", "publish", "picker.kyse.go"))
	if err != nil {
		t.Fatalf("reading the picker: %v", err)
	}
	markup := string(body)
	for _, pair := range [][2]string{
		{"@if(.NothingToChoose())", `.Labels.T("screen.picker_nothing_to_choose")`},
		{"@if(.NoneCarried())", `.Labels.T("screen.picker_empty")`},
	} {
		at := strings.Index(markup, pair[0])
		if at < 0 {
			t.Fatalf("the picker does not ask %s", pair[0])
		}
		next := strings.Index(markup[at:], "@endif")
		if next < 0 || !strings.Contains(markup[at:at+next], pair[1]) {
			t.Errorf("the picker does not draw %s under %s", pair[1], pair[0])
		}
	}
}
