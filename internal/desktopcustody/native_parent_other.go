//go:build !darwin

package desktopcustody

func nativeParentStart(int) string { return "" }
