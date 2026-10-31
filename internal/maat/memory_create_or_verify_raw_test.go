package maat

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCreateOrVerifyRawCloseFailureIsNotRetried(t *testing.T) {
	parent := t.TempDir()
	parentFD, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parentFD)

	want := []byte("durable create-only record")
	closeCalls := 0
	closeWithReportedError := func(fd int) error {
		closeCalls++
		if closeCalls > 1 {
			// Deliberately do not close again: the descriptor may have been reused.
			return errors.New("unexpected close retry")
		}
		if err := unix.Close(fd); err != nil {
			return err
		}
		return unix.EIO
	}

	err = createOrVerifyRawWithClose(parentFD, "record.json", want, closeWithReportedError)
	if !errors.Is(err, unix.EIO) {
		t.Fatalf("createOrVerifyRawWithClose error = %v, want EIO cause", err)
	}
	if closeCalls != 1 {
		t.Fatalf("close calls = %d, want exactly one", closeCalls)
	}
	got, err := os.ReadFile(filepath.Join(parent, "record.json"))
	if err != nil {
		t.Fatalf("read created record after close error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("created record = %q, want %q", got, want)
	}
}

func TestCreateOrVerifyRawNormalCloseReadback(t *testing.T) {
	parent := t.TempDir()
	parentFD, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parentFD)

	want := []byte("normal create-only readback")
	if err := createOrVerifyRaw(parentFD, "record.json", want); err != nil {
		t.Fatalf("createOrVerifyRaw: %v", err)
	}
	got, err := readExact(parentFD, "record.json")
	if err != nil {
		t.Fatalf("read created record: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("created record = %q, want %q", got, want)
	}
}
