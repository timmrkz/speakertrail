package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// appMenu is the menu Wails gives every app, with one change: Undo and
// Redo in Edit belong to the page. Ported from FrameFairy's menu.go.
//
// The stock ones hand the keys to the web view, whose undo only knows
// about text being typed, and on macOS the menu takes Cmd+Z before the
// page ever sees it. Report a Problem keeps its own undo, since a
// dictation writes into its field and Clear empties it, see
// web/src/screens/Report.svelte. These send the window in front an event
// called undo, and its page decides: in a field of its own it undoes the
// field the way the stock one did.
func appMenu(app *application.App) *application.Menu {
	menu := application.NewMenu()
	if runtime.GOOS != "windows" {
		menu.AddRole(application.AppMenu)
	}
	menu.AddRole(application.FileMenu)
	edit := menu.AddSubmenu("Edit")
	tell := func(how string) {
		if w := app.Window.Current(); w != nil {
			w.EmitEvent("undo", how)
		}
	}
	edit.Add("Undo").SetAccelerator("CmdOrCtrl+z").OnClick(func(*application.Context) { tell("undo") })
	// Redo is Shift+Cmd+Z, as in every text field on a Mac.
	edit.Add("Redo").SetAccelerator("CmdOrCtrl+Shift+z").OnClick(func(*application.Context) { tell("redo") })
	edit.AddSeparator()
	edit.AddRole(application.Cut)
	edit.AddRole(application.Copy)
	edit.AddRole(application.Paste)
	// The rest as the stock Edit menu has it.
	if runtime.GOOS == "darwin" {
		edit.AddRole(application.PasteAndMatchStyle)
		edit.AddRole(application.Delete)
		edit.AddRole(application.SelectAll)
		edit.AddSeparator()
		edit.AddRole(application.SpeechMenu)
	} else {
		edit.AddRole(application.Delete)
		edit.AddSeparator()
		edit.AddRole(application.SelectAll)
	}
	menu.AddRole(application.ViewMenu)
	menu.AddRole(application.WindowMenu)
	menu.AddRole(application.HelpMenu)
	return menu
}
