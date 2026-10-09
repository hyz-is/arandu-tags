package unit_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tags "github.com/hyz-is/arandu-tags"
)

// The releases the published views are compiled against.
//
// Both are pinned rather than resolved as the latest, so that a red run names a
// change in this repository and not one somebody published elsewhere in the
// meantime. Raising either is a one-line change, and it is the change that
// tells whether the views still compile with what an installer gets today.
const (
	// aruRelease is the CLI whose view compiler turns the markup into Go. It is
	// the compiler, not the Go toolchain, that refuses an address composed
	// behind text, so an older one passes views a current one refuses.
	aruRelease = "v0.69.3"
	// skeletonRelease is the project the views are published into: its layout
	// is the one every screen extends, and its go.mod is what an installer
	// compiles against.
	skeletonRelease = "v0.34.1"
)

// TestEveryPublishedViewCompiles does what an installer does with the views
// this package publishes, and fails where the installer would.
//
// Nothing else in this repository reads the markup. The sources carry a build
// tag that keeps the Go toolchain out of them, so `go build`, `go vet` and
// `go test` pass over a view that `aru view:build` refuses -- and the refusal
// arrives in somebody else's project, after they published.
//
// So the test builds that project: a copy of the skeleton release, the files
// Publishes declares written where it declares them, this module required from
// the working tree, the pinned aru's view:build, and then the Go compiler over
// every package ViewPackages names. The paths come from the module itself, so a
// view that moves cannot leave this test looking at the old address.
func TestEveryPublishedViewCompiles(t *testing.T) {
	root := packageRoot(t)
	scratch := t.TempDir()

	// The CLI, installed into the scratch directory rather than looked up on
	// the PATH: whatever aru the machine happens to have is not the one this
	// test names.
	bin := filepath.Join(scratch, "bin")
	run(t, scratch, []string{"GOBIN=" + bin}, "go", "install", "github.com/arandu-io/aru@"+aruRelease)
	aru := filepath.Join(bin, "aru")

	// The skeleton release, through the module proxy: the tagged tree `aru new`
	// clones with git, and a download the Go toolchain already knows how to
	// make and verify against the checksum database.
	out := run(t, scratch, nil, "go", "mod", "download", "-json", "github.com/arandu-io/arandu@"+skeletonRelease)
	var download struct{ Dir, Error string }
	if err := json.Unmarshal([]byte(out), &download); err != nil || download.Dir == "" {
		t.Fatalf("downloading the skeleton %s: %v %s", skeletonRelease, err, download.Error)
	}
	app := filepath.Join(scratch, "app")
	copyTree(t, download.Dir, app)

	// No stylesheet: view:build would download Tailwind to compile one, and
	// what this test asks about is the markup, which is compiled before it.
	if err := os.Remove(filepath.Join(app, "resources", "css", "app.css")); err != nil {
		t.Fatalf("removing the skeleton's stylesheet: %v", err)
	}

	// What vendor:publish writes: every publication, at the path it declares.
	publications := new(tags.Module).Publishes()
	if len(publications) == 0 {
		t.Fatal("the module publishes nothing, so this test would compile nothing")
	}
	written := 0
	for _, pub := range publications {
		err := fs.WalkDir(pub.Files, pub.From, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			body, err := fs.ReadFile(pub.Files, path)
			if err != nil {
				return err
			}
			target := filepath.Join(app, filepath.FromSlash(pub.To), filepath.FromSlash(strings.TrimPrefix(path, pub.From)))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			written++
			return os.WriteFile(target, body, 0o644)
		})
		if err != nil {
			t.Fatalf("writing the publication %s: %v", pub.Tag, err)
		}
	}
	if written != len(tags.PublishedPaths()) {
		t.Fatalf("the publications wrote %d files and PublishedPaths names %d", written, len(tags.PublishedPaths()))
	}

	// This module, from the working tree: the views being checked are the ones
	// here, and the types they alias are the ones beside them.
	gomod := filepath.Join(app, "go.mod")
	body, err := os.ReadFile(gomod)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("\nrequire github.com/hyz-is/arandu-tags v0.0.0\n\nreplace github.com/hyz-is/arandu-tags => "+root+"\n")...)
	if err := os.WriteFile(gomod, body, 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := command(app, nil, aru, "view:build").CombinedOutput(); err != nil {
		t.Fatalf("aru %s refuses the published views, and so does every project that publishes them:\n%s", aruRelease, out)
	}

	packages := tags.ViewPackages()
	if len(packages) == 0 {
		t.Fatal("ViewPackages names no package, so nothing would be compiled")
	}
	targets := make([]string, 0, len(packages))
	for _, pkg := range packages {
		matches, _ := filepath.Glob(filepath.Join(app, filepath.FromSlash(pkg), "*.go"))
		if len(matches) == 0 {
			t.Fatalf("aru view:build wrote no Go into %s, which is where ViewPackages says the views land", pkg)
		}
		targets = append(targets, "./"+pkg)
	}
	if out, err := command(app, nil, "go", append([]string{"build"}, targets...)...).CombinedOutput(); err != nil {
		t.Fatalf("the compiled views do not build, and no other gate of this repository reaches them:\n%s", out)
	}
}

// command is a process in dir with the module settings the scratch project
// needs: no workspace, and a go.sum completed as the build goes.
func command(dir string, env []string, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	cmd.Env = append(cmd.Env, env...)
	return cmd
}

// run is command, failing the test when it fails, and answering what it wrote to
// stdout.
func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := command(dir, env, name, args...)
	// A go command given a module version ignores the current module, and it
	// refuses a -mod flag outright.
	cmd.Env = append(cmd.Env, "GOFLAGS=")
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exit, ok := err.(*exec.ExitError); ok {
			stderr = string(exit.Stderr)
		}
		t.Fatalf("%s %s: %v\n%s%s", name, strings.Join(args, " "), err, out, stderr)
	}
	return string(out)
}

// copyTree copies a read-only module directory into a writable one.
func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
	if err != nil {
		t.Fatalf("copying %s: %v", from, err)
	}
}
