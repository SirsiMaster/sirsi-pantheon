//go:build !darwin

package main

import "fmt"

func kinfoAnchorProcess(pid int) (anchorProcess, error) {
	return anchorProcess{}, fmt.Errorf("inspect pid %d: no ps-free lookup on this platform", pid)
}
