package ui

import "github.com/gdamore/tcell/v2"

// keyRune is the character event types, 0 for any other key: Ctrl+D is
// KeyCtrlD and carries the rune 'd', which must not pass for a typed d
func keyRune(event *tcell.EventKey) rune {
	if event.Key() != tcell.KeyRune {
		return 0
	}
	return event.Rune()
}

// isAltEnter reports whether event is Option+Enter (Alt+Enter)
func isAltEnter(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter && event.Modifiers()&tcell.ModAlt != 0 {
		return true
	}
	// A terminal that sends Esc and Enter for it: read as Alt+Ctrl+M
	return event.Key() == tcell.KeyRune && event.Rune() == 'm' && event.Modifiers() == tcell.ModAlt|tcell.ModCtrl
}

// isNewlineKey reports whether event asks for a new line in a text area
// that Enter sends: Shift+Enter, where the terminal tells it from Enter, or
// Option+Enter anywhere
func isNewlineKey(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter && event.Modifiers()&tcell.ModShift != 0 {
		return true
	}
	// A terminal set up to send a line feed for Shift+Enter, as Claude
	// Code's terminal setup does in iTerm2: the same as Ctrl+J
	if event.Key() == tcell.KeyCtrlJ {
		return true
	}
	return isAltEnter(event)
}
