//go:build !darwin

package agent

import "errors"

var errMacOnly = errors.New("the drive agent needs macOS (launchd)")

// Install is only available on macOS.
func Install(bin string, env map[string]string) (string, error) { return "", errMacOnly }

// Uninstall is only available on macOS.
func Uninstall() error { return errMacOnly }

// Status reports that there is no agent off macOS.
func Status() string { return "not available: the drive agent needs macOS" }

// Notify does nothing off macOS.
func Notify(title, msg string) {}
