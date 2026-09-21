package main

import (
	"os"
	"strings"
)

func (a *App) MenuLanguage() string {
	if saved, _ := a.db.GetConfig("ui_language"); saved == "zh-CN" || saved == "en-US" {
		return saved
	}
	if strings.HasPrefix(strings.ToLower(os.Getenv("LANG")), "zh") {
		return "zh-CN"
	}
	return "en-US"
}

// SetLanguage persists the SPA language. Gemsnote intentionally has no native
// application menu; all commands live in the application UI.
func (a *App) SetLanguage(language string) {
	if language != "zh-CN" && language != "en-US" {
		return
	}
	a.db.SetConfig("ui_language", language)
}
