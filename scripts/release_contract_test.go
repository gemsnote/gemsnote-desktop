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
