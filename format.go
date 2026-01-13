package spectra

import "fmt"

// formatArgs formats variadic arguments into a string.
func formatArgs(args ...any) string {
	return fmt.Sprint(args...)
}

// formatf formats a string with variadic arguments.
func formatf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
