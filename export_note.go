package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func noteExportOptions(filename, language string) runtime.SaveDialogOptions {
	title, display := "Export note", "Markdown notes"
	pattern := "*.md"
	if strings.HasSuffix(filename, ".html") {
		pattern = "*.html"
		display = "HTML notes"
	}
	if strings.HasPrefix(strings.ToLower(language), "zh") {
		title = "导出笔记"
		display = "Markdown 笔记"
		if pattern == "*.html" {
			display = "HTML 笔记"
		}
	}
	if strings.HasSuffix(filename, ".rtf") {
		pattern = "*.rtf"
		display = "Rich Text Format"
		if strings.HasPrefix(strings.ToLower(language), "zh") {
			display = "RTF 富文本"
		}
	}
	if strings.HasSuffix(filename, ".zip") {
		pattern = "*.zip"
		title, display = "Export notebook", "ZIP archive"
		if strings.HasPrefix(strings.ToLower(language), "zh") {
			title, display = "导出笔记本", "ZIP 压缩包"
		}
	}
	return runtime.SaveDialogOptions{Title: title, DefaultFilename: filename, CanCreateDirectories: true, Filters: []runtime.FileFilter{{DisplayName: display, Pattern: pattern}}}
}

func (a *App) saveNoteExport(filename, content, language string) (bool, error) {
	if a.ctx == nil {
		return false, fmt.Errorf("native window unavailable")
	}
	path, err := runtime.SaveFileDialog(a.ctx, noteExportOptions(filename, language))
	if err != nil || path == "" {
		return false, err
	}
	if filepath.Ext(path) == "" {
		path += filepath.Ext(filename)
	}
	if err := writeNoteExport(path, content); err != nil {
		return false, err
	}
	return true, nil
}

func writeNoteExport(path, content string) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".gemsnote-export-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	defer file.Close()
	if _, err = file.WriteString(content); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
