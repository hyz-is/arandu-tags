package unit_test

import (
	"go/ast"
	"slices"
	"testing"

	tags "github.com/hyz-is/arandu-tags"
)

// TestEveryViewThisPackagePublishesIsRenderedByAHandler is the check the
// publication itself cannot make.
//
// Boot refuses to serve until every published view is registered, so an
// application that installs this package is made to publish, compile and import
// all of them. That is a cost, and a view no handler ever renders makes somebody
// pay it for a page nobody can reach -- a file in their repository, a package in
// their imports, and a refusal at start-up if they delete either.
//
// It reads the source rather than the running module because a render happens
// on a request and only for the branch that took it: a screen reachable from one
// handler under one condition would need that request to be made before anything
// noticed, which is exactly the state this test exists to prevent.
func TestEveryViewThisPackagePublishesIsRenderedByAHandler(t *testing.T) {
	t.Parallel()

	names := tags.ViewNames()
	if len(names) == 0 {
		t.Fatal("the package publishes no view, so this test would pass by having nothing to read")
	}

	// The constants a handler renders by, resolved to what they hold, so the
	// check is against the name a page is registered under and not against the
	// identifier that happens to carry it.
	rendered := map[string]bool{}
	for _, source := range productionGoFiles(t, packageRoot(t)) {
		ast.Inspect(source.file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || calledName(call) != "View" || len(call.Args) == 0 {
				return true
			}
			if name, ok := call.Args[0].(*ast.Ident); ok {
				rendered[name.Name] = true
			}
			return true
		})
	}

	// Each constant, by the name it holds.
	holding := map[string]string{
		"ViewIndex":  tags.ViewIndex,
		"ViewEdit":   tags.ViewEdit,
		"ViewOrder":  tags.ViewOrder,
		"ViewPicker": tags.ViewPicker,
	}
	drawn := map[string]bool{}
	for identifier, name := range holding {
		if rendered[identifier] {
			drawn[name] = true
		}
	}

	fragments := tags.Fragments()
	for _, name := range names {
		if slices.Contains(fragments, name) {
			// The application draws this one; see Fragments.
			continue
		}
		if !drawn[name] {
			t.Errorf("%s is published and required at boot, and no handler renders it: whoever installs this pays for a page nobody can reach", name)
		}
	}
	for identifier, name := range holding {
		if !slices.Contains(names, name) {
			t.Errorf("%s is %q, which is not a view this package publishes, so rendering it is a 500", identifier, name)
		}
	}
}
