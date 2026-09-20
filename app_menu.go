package main

import (
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type menuLabels struct {
	file, newNote, newNotebook, exportPDF, quit string
	view, fullscreen                            string
	sync, syncNow, fullSync                     string
	help, about                                 string
}

func labelsFor(language string) menuLabels {
	if language == "zh-CN" {
		return menuLabels{
			file: "文件", newNote: "新建笔记", newNotebook: "新建笔记本", exportPDF: "导出 PDF", quit: "退出",
			view: "视图", fullscreen: "切换全屏",
			sync: "同步", syncNow: "立即同步", fullSync: "完全同步",
			help: "帮助", about: "关于",
		}
	}
	return menuLabels{
		file: "File", newNote: "New Note", newNotebook: "New Notebook", exportPDF: "Export PDF", quit: "Quit",
		view: "View", fullscreen: "Toggle Full Screen",
		sync: "Sync", syncNow: "Sync Now", fullSync: "Full Sync",
		help: "Help", about: "About",
	}
}

func buildMenu(app *App, language string) *menu.Menu {
	label := labelsFor(language)
	appMenu := menu.NewMenu()

	fileMenu := appMenu.AddSubmenu(label.file)
	fileMenu.AddText(label.newNote, keys.CmdOrCtrl("n"), func(_ *menu.CallbackData) {})
	fileMenu.AddText(label.newNotebook, keys.CmdOrCtrl("shift+n"), func(_ *menu.CallbackData) {})
	fileMenu.AddSeparator()
	fileMenu.AddText(label.exportPDF, keys.CmdOrCtrl("shift+e"), func(_ *menu.CallbackData) {})
	fileMenu.AddSeparator()
	fileMenu.AddText(label.quit, keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) { os.Exit(0) })

	viewMenu := appMenu.AddSubmenu(label.view)
	viewMenu.AddText(label.fullscreen, keys.Key("F11"), func(_ *menu.CallbackData) {})

	syncMenu := appMenu.AddSubmenu(label.sync)
	syncMenu.AddText(label.syncNow, keys.CmdOrCtrl("s"), func(_ *menu.CallbackData) { go app.IncrSync() })
	syncMenu.AddText(label.fullSync, keys.CmdOrCtrl("shift+s"), func(_ *menu.CallbackData) { go app.FullSyncForce() })

	helpMenu := appMenu.AddSubmenu(label.help)
	helpMenu.AddText(label.about, nil, func(_ *menu.CallbackData) {})
	return appMenu
}

func (a *App) MenuLanguage() string {
	if saved, _ := a.db.GetConfig("ui_language"); saved == "zh-CN" || saved == "en-US" {
		return saved
	}
	if strings.HasPrefix(strings.ToLower(os.Getenv("LANG")), "zh") {
		return "zh-CN"
	}
	return "en-US"
}

// SetLanguage keeps the native application menu in step with the SPA.
func (a *App) SetLanguage(language string) {
	if language != "zh-CN" && language != "en-US" {
		return
	}
	a.db.SetConfig("ui_language", language)
	if a.ctx != nil {
		runtime.MenuSetApplicationMenu(a.ctx, buildMenu(a, language))
	}
}
