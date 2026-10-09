package nativemenu

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNativeMenuLanguageAndActions(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "menu-test")
	cmd := exec.Command("clang", "-fobjc-arc", "-fblocks", "-framework", "Cocoa", "testdata/menu_test.m", "-o", binary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile native fixture: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("native menu contract: %v\n%s", err, out)
	}
}
