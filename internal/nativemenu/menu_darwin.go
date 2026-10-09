package nativemenu

/*
#cgo LDFLAGS: -framework Cocoa
void GemsnoteLocalizeMenu(void);
*/
import "C"

// Localize updates Wails' native labels on the AppKit main thread.
func Localize() { C.GemsnoteLocalizeMenu() }
