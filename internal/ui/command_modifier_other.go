//go:build !darwin || !cgo

package ui

func platformCommandModifierPressed() bool {
	return false
}
