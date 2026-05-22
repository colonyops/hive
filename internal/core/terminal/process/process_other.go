//go:build !linux && !darwin

package process

import "fmt"

func tpgidFromPID(_ int) (int, error) {
	return 0, fmt.Errorf("process identification not supported on this platform")
}

func commForPID(_ int) string { return "" }

func cmdlineForPID(_ int) ([]string, error) {
	return nil, fmt.Errorf("process identification not supported on this platform")
}

func environForPID(_ int) map[string]string { return nil }

func childrenForPID(_ int) ([]int, error) { return nil, nil }
