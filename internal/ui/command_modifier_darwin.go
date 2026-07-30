//go:build darwin && cgo

package ui

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>

static bool xlentCommandModifierPressed(void) {
	CGEventFlags flags = CGEventSourceFlagsState(kCGEventSourceStateCombinedSessionState);
	return (flags & kCGEventFlagMaskCommand) != 0;
}
*/
import "C"

// platformCommandModifierPressed bridges the Command key state into terminal
// mouse handling. SGR mouse reports Shift, Option, and Control, but has no bit
// for Command; querying the current macOS session state preserves Command-click
// even when the event passes through tmux.
func platformCommandModifierPressed() bool {
	return bool(C.xlentCommandModifierPressed())
}
