//go:build !darwin

package library

// canonicalCase is the identity outside macOS, where paths are case-sensitive.
func canonicalCase(p string) string { return p }
