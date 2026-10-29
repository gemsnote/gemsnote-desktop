package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteNoteExportPreservesUTF8AndReplacesOnlyOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "珠玑.md")
	for _, content := range []string{"# 标题\r\n\n**正文**\n", ""} {
		if err := writeNoteExport(path, content); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("export contents %q, %v", got, err)
		}
	}
	if err := writeNoteExport(dir, "must not overwrite a directory"); err == nil {
		t.Fatal("directory accepted as export destination")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "珠玑.md" {
		t.Fatalf("export left temporary files: %+v %v", entries, err)
	}
}

func TestNoteExportDialogUsesSelectedLanguageAndFormat(t *testing.T) {
	zh := noteExportOptions("珠玑.md", "zh-CN")
	en := noteExportOptions("note.html", "en-US")
	rtf := noteExportOptions("rich.rtf", "en-US")
	if rtf.Filters[0].Pattern != "*.rtf" {
		t.Fatalf("RTF dialog: %+v", rtf)
	}
	zip := noteExportOptions("笔记本.zip", "zh-CN")
	if zip.Title != "导出笔记本" || zip.Filters[0].Pattern != "*.zip" {
		t.Fatalf("ZIP dialog: %+v", zip)
	}
	if zh.Title != "导出笔记" || zh.DefaultFilename != "珠玑.md" || zh.Filters[0].Pattern != "*.md" {
		t.Fatalf("Chinese Markdown dialog: %+v", zh)
	}
	if en.Title != "Export note" || en.Filters[0].Pattern != "*.html" {
		t.Fatalf("English HTML dialog: %+v", en)
	}
}
