package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDMGPythonBootstrap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS Bash helper")
	}
	for _, mode := range []string{"automatic", "install-failure", "explicit-invalid"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			scriptDir := filepath.Join(root, "scripts")
			bin := filepath.Join(root, "bin")
			for _, dir := range []string{scriptDir, bin} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"prepare-dmg-python.sh", "dmg-requirements.txt"} {
				if err := os.WriteFile(filepath.Join(scriptDir, name), []byte(read(t, name)), 0644); err != nil {
					t.Fatal(err)
				}
			}
			fake := `#!/bin/bash
set -eu
case "$1" in
 -c) exit 0 ;;
 -) test -f "$TEST_ROOT/installed" ;;
 -m)
  if [[ "$2" == venv ]]; then
   echo venv >> "$TEST_ROOT/calls"
   mkdir -p "$3/bin"
   cp "$0" "$3/bin/python"
  else
   echo pip >> "$TEST_ROOT/calls"
   echo "pip progress on stdout"
   [[ "$TEST_MODE" != install-failure ]] || exit 1
   touch "$TEST_ROOT/installed"
  fi ;;
 *) exit 9 ;;
esac
`
			python := filepath.Join(bin, "python3")
			if err := os.WriteFile(python, []byte(fake), 0755); err != nil {
				t.Fatal(err)
			}
			run := func() ([]byte, error) {
				cmd := exec.Command("bash", filepath.Join(scriptDir, "prepare-dmg-python.sh"))
				override := ""
				if mode == "explicit-invalid" {
					override = python
				}
				cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_ROOT="+root, "TEST_MODE="+mode, "DMG_PYTHON="+override)
				return cmd.Output()
			}
			out, err := run()
			if mode != "automatic" {
				if err == nil {
					t.Fatal("failed setup must fail closed")
				}
				if len(out) != 0 {
					t.Fatalf("failed setup returned interpreter: %s", out)
				}
				if mode == "explicit-invalid" {
					if _, err := os.Stat(filepath.Join(root, "calls")); !os.IsNotExist(err) {
						t.Fatal("explicit environment must not be modified")
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Clean(strings.TrimSpace(string(out))) != filepath.Join(root, "build", "dmg-venv", "bin", "python") {
				t.Fatalf("unexpected stdout: %q", out)
			}
			if _, err := run(); err != nil {
				t.Fatal(err)
			}
			if calls := read(t, filepath.Join(root, "calls")); calls != "venv\npip\n" {
				t.Fatalf("must reuse ready environment without pip: %q", calls)
			}
		})
	}
}
