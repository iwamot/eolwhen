package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDir(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".python-version", "2.7.18\n")
	write(t, dir, ".nvmrc", "lts/hydrogen\n")
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.16\n")
	write(t, dir, "Dockerfile", "FROM python:3.7-slim\n")
	// Below the directory, which is where a monorepo and a docker/ folder
	// keep theirs.
	write(t, dir, "docker/Dockerfile.ci", "FROM ubuntu:18.04\n")
	write(t, dir, "packages/web/.nvmrc", "14.19.0\n")
	write(t, dir, ".github/workflows/ci.yml", "jobs:\n  a:\n    runs-on: macos-13\n")
	write(t, dir, "compose.yml", "services:\n  db:\n    image: postgres:11\n")
	write(t, dir, "mise.toml", "[tools]\nbun = \"1.2\"\n")
	// A YAML file is a workflow because of where it sits, so this one is not.
	write(t, dir, ".github/dependabot.yml", "jobs:\n  a:\n    runs-on: macos-12\n")
	// Somebody else's declarations, which this directory is not answering
	// for.
	write(t, dir, "node_modules/left-pad/.nvmrc", "8.0.0\n")
	write(t, dir, "vendor/foo/Dockerfile", "FROM centos:6\n")

	ds, us, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	got := map[string]string{}
	for _, d := range ds {
		got[d.Source.String()] = d.Product + " " + d.Version
	}
	want := map[string]string{
		".python-version:1":          "python 2.7.18",
		"go.mod:3":                   "go 1.16",
		"Dockerfile:1":               "python 3.7",
		"docker/Dockerfile.ci:1":     "ubuntu 18.04",
		"packages/web/.nvmrc:1":      "nodejs 14.19.0",
		".github/workflows/ci.yml:3": "github-actions-runner-images macos-13",
		"compose.yml:3":              "postgres 11",
		"mise.toml:2":                "bun 1.2",
	}
	if len(got) != len(want) {
		t.Fatalf("declarations = %v; want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("at %s got %q; want %q", k, got[k], v)
		}
	}
	if len(us) != 1 || us[0].Text != "lts/hydrogen" {
		t.Errorf("unreadable = %+v; want the .nvmrc line", us)
	}
}

// TestDirEmpty is the common case for a directory that declares nothing, and
// it is not an error.
func TestDirEmpty(t *testing.T) {
	ds, us, err := Dir(t.TempDir())
	if err != nil || len(ds) != 0 || len(us) != 0 {
		t.Errorf("Dir = %+v, %+v, %v; want nothing and no error", ds, us, err)
	}
}

// TestDirSkipsItselfByName guards the skip list against skipping the very
// directory it was pointed at: `eolwhen vendor` reads that vendor directory.
func TestDirSkipsItselfByName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vendor")
	write(t, dir, ".nvmrc", "14.19.0\n")
	ds, _, err := Dir(dir)
	if err != nil || len(ds) != 1 {
		t.Errorf("Dir = %+v, %v; want the one declaration", ds, err)
	}
}

// TestDirNameTakenByADirectory is a directory that happens to carry a
// supported file's name. It is walked like any other and read like none.
func TestDirNameTakenByADirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".nvmrc/notes.txt", "nothing here\n")
	ds, us, err := Dir(dir)
	if err != nil || len(ds) != 0 || len(us) != 0 {
		t.Errorf("Dir = %+v, %+v, %v; want nothing and no error", ds, us, err)
	}
}

func TestDirMissing(t *testing.T) {
	if _, _, err := Dir(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("want an error for a directory that is not there")
	}
}

// TestDirUnreadableFile: a name the walk offers but the read cannot open —
// here a symlink to nothing — is reported and the rest of the tree is still
// read, because one unreadable corner is not a reason to refuse an answer.
func TestDirUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, ".nvmrc")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	write(t, dir, ".python-version", "2.7.18\n")
	ds, us, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if len(ds) != 1 || ds[0].Product != "python" {
		t.Errorf("declarations = %+v; want the readable one", ds)
	}
	if len(us) != 1 || us[0].Source.File != ".nvmrc" || us[0].Text != "" {
		t.Errorf("unreadable = %+v; want the .nvmrc path alone", us)
	}
	if !strings.HasPrefix(us[0].Reason, "could not be read") {
		t.Errorf("reason = %q", us[0].Reason)
	}
}

// TestDirUnreadableSubdirectory: a corner of the tree the walk cannot enter
// costs whatever it held and nothing more. The run still answers for
// everything else, because refusing the whole question over one locked
// directory serves nobody.
func TestDirUnreadableSubdirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".python-version", "2.7.18\n")
	locked := filepath.Join(dir, "locked")
	write(t, dir, "locked/.nvmrc", "14.19.0\n")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Skipf("cannot lock a directory here: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	ds, us, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if len(ds) != 1 || ds[0].Product != "python" {
		t.Skipf("the directory was readable anyway (running as root?): %+v", ds)
	}
	if len(us) != 1 || us[0].Source.File != "locked" {
		t.Fatalf("unreadable = %+v; want the locked directory", us)
	}
	if us[0].Reason != "could not be read: permission denied" {
		t.Errorf("reason = %q; want it to name the permission", us[0].Reason)
	}
}

// TestDirFindsWorkflowsWhateverTheSeparator guards the one place a path's
// spelling decides what reads it. A workflow is a workflow because of the
// directory it sits in, and on Windows that path arrives with backslashes,
// so normalizing after the extractor was chosen would hide every workflow
// on that platform. This test runs everywhere; the normalization it checks
// is what makes the Windows case work, which has not been run here.
func TestDirFindsWorkflowsWhateverTheSeparator(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".github/workflows/ci.yml",
		"jobs:\n  a:\n    steps:\n      - uses: actions/setup-python@v5\n        with:\n          python-version: '2.7'\n")
	ds, _, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if len(ds) != 1 || ds[0].Product != "python" || ds[0].Version != "2.7" {
		t.Fatalf("declarations = %+v; want python 2.7", ds)
	}
	// Reported with forward slashes, so a row reads the same on every
	// platform.
	if ds[0].Source.File != ".github/workflows/ci.yml" {
		t.Errorf("Source.File = %q; want .github/workflows/ci.yml", ds[0].Source.File)
	}
}

// TestDirThroughASymlink: a checkout reached through a link is the
// checkout. WalkDir does not follow the root it is given, so without
// resolving it first the scan finds nothing and says so, which reads
// exactly like a directory that declares nothing.
func TestDirThroughASymlink(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	write(t, real, ".python-version", "2.7\n")
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	direct, _, err := Dir(real)
	if err != nil {
		t.Fatalf("Dir(real): %v", err)
	}
	through, _, err := Dir(link)
	if err != nil {
		t.Fatalf("Dir(link): %v", err)
	}
	if len(through) != len(direct) || len(through) != 1 {
		t.Fatalf("through the link = %+v; direct = %+v", through, direct)
	}
	if through[0] != direct[0] {
		t.Errorf("through the link = %+v; want the same as direct, %+v", through[0], direct[0])
	}
}

// TestDirBrokenSymlink is an error rather than an empty answer: a link to
// nothing is not a directory that declares nothing.
func TestDirBrokenSymlink(t *testing.T) {
	base := t.TempDir()
	link := filepath.Join(base, "link")
	if err := os.Symlink(filepath.Join(base, "gone"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, _, err := Dir(link); err == nil {
		t.Error("want an error for a link to nothing")
	}
}

// TestDirUnreadableRoot: the directory that was asked about has to be
// readable. A corner of the tree that is not costs what it held, but the
// root itself is the question, so failing to read it is an error.
func TestDirUnreadableRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "locked")
	write(t, root, ".python-version", "2.7\n")
	if err := os.Chmod(root, 0o000); err != nil {
		t.Skipf("cannot lock a directory here: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	ds, _, err := Dir(root)
	if err == nil {
		t.Skipf("the directory was readable anyway (running as root?): %+v", ds)
	}
}
