package debug

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "orbit-debug-test-*")
	if err != nil {
		panic("failed to create temp home for tests: " + err.Error())
	}
	defer os.RemoveAll(tmp)

	os.Setenv("HOME", tmp)

	os.Exit(m.Run())
}
