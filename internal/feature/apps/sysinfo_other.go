//go:build !linux

package apps

func collectSystem() *System { return nil }

func packageOrigins([]string) map[string]string { return nil }
