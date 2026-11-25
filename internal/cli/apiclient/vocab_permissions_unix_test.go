//go:build !windows

package apiclient_test

import (
	"os"
	"testing"
)

func assertVocabStorePermissions(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("vocab store written %o, want 0600 (it holds a capability token on a possibly shared path)", perm)
	}
}
