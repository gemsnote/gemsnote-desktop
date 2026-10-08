package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrepareFrontendOnCleanCheckout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper fixture; PowerShell order is checked separately")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "build-failure"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write := func(path, data string, mode os.FileMode) {
				t.Helper()
				path = filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), mode); err != nil {
					t.Fatal(err)
				}
			}
			write("desktop-app/build-frontend.sh", read(t, "../build-frontend.sh"), 0755)
			write("frontend/dist/index.html", "fixture frontend", 0644)
			write("public/tinymce/tinymce.min.js", "fixture editor", 0644)
			stub := "#!/bin/sh\nexit 0\n"
			if fail {
				stub = "#!/bin/sh\nexit 13\n"
			}
			write("bin/npm", stub, 0755)
			cmd := exec.Command(bash, filepath.Join(root, "desktop-app/build-frontend.sh"))
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			dist := filepath.Join(root, "desktop-app/frontend/dist")
			if fail {
				if err == nil {
					t.Fatal("failed npm build must stop resource preparation")
				}
				if _, err := os.Stat(dist); !os.IsNotExist(err) {
					t.Fatal("failed build unexpectedly created embedded resources")
				}
				return
			}
			if err != nil {
				t.Fatalf("prepare: %v\n%s", err, out)
			}
			for _, file := range []string{"index.html", "tinymce/tinymce.min.js"} {
				if _, err := os.Stat(filepath.Join(dist, file)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReleasePreparesEmbeddedFrontendBeforeTests(t *testing.T) {
	for _, name := range []string{"build-release.sh", "build-release.ps1"} {
		t.Run(name, func(t *testing.T) {
			s := read(t, name)
			prepare := strings.Index(s, "build-frontend.sh")
			tests := strings.Index(s, "go test ./...")
			if prepare < 0 || tests < 0 || prepare >= tests {
				t.Fatal("frontend preparation must precede Go tests on a clean checkout")
			}
			if name == "build-release.ps1" && !strings.Contains(s[prepare:tests], "$LASTEXITCODE -ne 0") {
				t.Fatal("PowerShell must stop when frontend preparation fails")
			}
		})
	}
}

func TestReleaseTagsMatchServerConvention(t *testing.T) {
	s := read(t, "../.github/workflows/release.yml")
	for _, required := range []string{`"[0-9]*.[0-9]*.[0-9]*"`, `^[0-9]+\.[0-9]+\.[0-9]+$`, `echo "version=${tag}"`} {
		if !strings.Contains(s, required) {
			t.Errorf("missing bare-version tag contract: %s", required)
		}
	}
	if strings.Contains(s, "${tag#v}") || strings.Contains(s, "v*.*.*") {
		t.Fatal("legacy v-prefix release convention remains")
	}
}

func TestStartupDoesNotAdoptLegacyCache(t *testing.T) {
	s := read(t, "../main.go")
	if strings.Contains(s, "migrateLegacyData") || strings.Contains(s, "leanote.db") {
		t.Fatal("startup must not adopt legacy cache")
	}
}

func TestMacOSPreparationSealsFinalResourcesAndFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS packaging uses Bash")
	}
	for _, failure := range []string{"", "sign", "verify"} {
		t.Run("failure="+failure, func(t *testing.T) {
			root := t.TempDir()
			app := filepath.Join(root, "Gemsnote with spaces.app")
			if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("fixture"), 0644); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0755); err != nil {
				t.Fatal(err)
			}
			stub := `#!/usr/bin/env bash
set -eu
app="${@: -1}"
for locale in en zh-Hans zh-Hant; do
  test -s "$app/Contents/Resources/$locale.lproj/InfoPlist.strings"
done
if [[ "$1" == --force ]]; then
  [[ "$2" == --sign && "$3" == - && "$4" == --timestamp=none ]]
  echo sign >> "$TEST_LOG"
  [[ "$TEST_FAILURE" != sign ]] || exit 13
else
  [[ "$1" == --verify && "$2" == --deep && "$3" == --strict ]]
  grep -q '^sign$' "$TEST_LOG"
  echo verify >> "$TEST_LOG"
  [[ "$TEST_FAILURE" != verify ]] || exit 14
fi
`
			if err := os.WriteFile(filepath.Join(bin, "codesign"), []byte(stub), 0755); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(root, "calls")
			// Model an old release checkout with no scripts directory. Only the
			// separately checked-out workflow tooling provides the signing helper.
			toolDir := filepath.Join(root, "release-tools", "scripts")
			if err := os.MkdirAll(toolDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(toolDir, "prepare-macos-app.sh"), []byte(read(t, "prepare-macos-app.sh")), 0644); err != nil {
				t.Fatal(err)
			}
			sourceDir := filepath.Join(root, "gemsnote", "desktop-app")
			if err := os.MkdirAll(filepath.Join(sourceDir, "build", "bin"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(app, filepath.Join(sourceDir, "build", "bin", "gemsnote.app")); err != nil {
				t.Fatal(err)
			}
			workflow := read(t, "../.github/workflows/release.yml")
			var invocation string
			for _, line := range strings.Split(workflow, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "bash ") && strings.Contains(line, "prepare-macos-app.sh") {
					invocation = strings.TrimSpace(line)
				}
			}
			if invocation == "" {
				t.Fatal("missing workflow helper invocation")
			}
			cmd := exec.Command("bash", "-euc", invocation)
			cmd.Dir = sourceDir
			cmd.Env = append(os.Environ(), "GITHUB_WORKSPACE="+root, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_LOG="+log, "TEST_FAILURE="+failure)
			out, err := cmd.CombinedOutput()
			if (err != nil) != (failure != "") {
				t.Fatalf("unexpected result: %v\n%s", err, out)
			}
			calls := read(t, log)
			want := "sign\nverify\n"
			if failure == "sign" {
				want = "sign\n"
			}
			if calls != want {
				t.Fatalf("calls = %q, want %q", calls, want)
			}
		})
	}
}

func TestMacOSReleasePreparesSignatureBeforeDiskImage(t *testing.T) {
	for _, path := range []string{"build-release.sh", "../.github/workflows/release.yml"} {
		s := read(t, path)
		prepare := strings.Index(s, "prepare-macos-app.sh")
		dmg := strings.Index(s, "hdiutil create -volname")
		if prepare < 0 || dmg < 0 || prepare >= dmg {
			t.Fatalf("%s must verify the final app before creating a DMG", path)
		}
		if strings.Contains(s, "InfoPlist.strings") {
			t.Fatalf("%s must not mutate localized resources outside the signing helper", path)
		}
	}
}

func TestMacOSToolingUsesWorkflowRevisionInsteadOfReleaseTag(t *testing.T) {
	s := read(t, "../.github/workflows/release.yml")
	start := strings.Index(s, "      - name: Check out macOS release tooling\n")
	if start < 0 {
		t.Fatal("missing separate tooling checkout")
	}
	end := strings.Index(s[start+1:], "      - name:")
	if end < 0 {
		t.Fatal("missing following step")
	}
	step := s[start : start+1+end]
	for _, required := range []string{
		"if: matrix.archive == 'darwin'",
		"uses: actions/checkout@v4",
		"repository: ${{ github.repository }}",
		"ref: ${{ github.workflow_sha }}",
		"path: release-tools",
		"sparse-checkout: scripts/prepare-macos-app.sh",
		"sparse-checkout-cone-mode: false",
	} {
		if !strings.Contains(step, required) {
			t.Errorf("tooling checkout missing %s", required)
		}
	}
	if strings.Contains(step, "outputs.tag") {
		t.Fatal("tooling must not come from release tag")
	}
	if !strings.Contains(s, "ref: ${{ needs.validate.outputs.tag }}") {
		t.Fatal("application must still use release tag")
	}
}
