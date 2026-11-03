//go:build !darwin

package main

import "errors"

func execute(request) error { return errors.New("installer supports macOS only") }
