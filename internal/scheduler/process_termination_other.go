//go:build !darwin

package scheduler

func findRunningAppProcesses([]string) []string { return nil }

func terminateBlockedProcesses([]string) {}
