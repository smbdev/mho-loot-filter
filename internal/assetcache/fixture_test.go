package assetcache

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// testdata reads a real game file copied by tools/make_fixtures.py. Game files are not part of the repository,
// so tests that need them are skipped until the fixtures have been created from a local install.
func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("testdata/%s not found: run python tools/make_fixtures.py \"<game folder>\" first", name)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b
}
