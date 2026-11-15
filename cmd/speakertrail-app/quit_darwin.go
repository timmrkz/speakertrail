//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// quitAsked says whether the quit being handled is the Mac's own: the
// Apple event that asks the app to quit, sent with a reason, which
// loginwindow gives for a logout, a restart and a shutdown. Cmd+Q and
// Quit in the menu call terminate: with no Apple event, and the Dock's
// Quit sends one with no reason.
static bool quitAsked(void) {
	NSAppleEventDescriptor *ev = [[NSAppleEventManager sharedAppleEventManager] currentAppleEvent];
	if (ev == nil || [ev eventClass] != kCoreEventClass || [ev eventID] != kAEQuitApplication) {
		return false;
	}
	return [ev attributeDescriptorForKeyword:keyAEQuitReason] != nil;
}
*/
import "C"

// quitBySystem says whether macOS asks the app to quit for a logout, a
// restart or a shutdown. Wails asks ShouldQuit inside
// applicationShouldTerminate, on the main thread, while that Apple event
// is the one being handled, so it can be read there. An answer of no
// there is NSTerminateCancel, which cancels the logout.
func quitBySystem() bool { return bool(C.quitAsked()) }
