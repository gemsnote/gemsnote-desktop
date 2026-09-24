package webapi

import "runtime"

// DesktopAbout describes the running desktop build, independently of the
// connected server and of the availability of generated Wails JS bindings.
func DesktopAbout(version string) map[string]string {
	return map[string]string{
		"Name": "Gemsnote", "Version": version,
		"Platform": runtime.GOOS, "Arch": runtime.GOARCH, "Runtime": runtime.Version(),
	}
}
