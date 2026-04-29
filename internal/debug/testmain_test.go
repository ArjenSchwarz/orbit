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

	if err := os.Setenv("HOME", tmp); err != nil {
		panic("failed to set HOME for tests: " + err.Error())
	}

	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}
