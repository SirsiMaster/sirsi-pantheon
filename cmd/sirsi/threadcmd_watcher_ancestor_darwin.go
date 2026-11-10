//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Monitor directory identities without fsnotify's kqueue directory scan.
// The root is immutable by rename and is not opened; the private custody
// directory and every named ancestor are watched directly as vnodes.
type routerWatcherAncestorMonitor struct {
	changed      chan struct{}
	errors       chan error
	wakeR, wakeW *os.File
	queue        int
	bindings     []routerWatcherAncestorBinding
	done         chan struct{}
}

func startRouterWatcherAncestors(paths []string) (*routerWatcherAncestorMonitor, error) {
	return startRouterWatcherAncestorsWith(paths, nil, nil)
}

func startRouterWatcherAncestorsWith(paths []string, beforeOpen, afterOpen func(string) error) (*routerWatcherAncestorMonitor, error) {
	m := &routerWatcherAncestorMonitor{queue: -1, changed: make(chan struct{}, 1), errors: make(chan error, 1), done: make(chan struct{})}
	fail := func(err error) (*routerWatcherAncestorMonitor, error) {
		for _, binding := range m.bindings {
			_ = binding.file.Close()
		}
		if m.queue >= 0 {
			_ = unix.Close(m.queue)
		}
		if m.wakeR != nil {
			_ = m.wakeR.Close()
		}
		if m.wakeW != nil {
			_ = m.wakeW.Close()
		}
		return nil, err
	}
	var err error
	m.queue, err = unix.Kqueue()
	if err != nil {
		return fail(err)
	}
	unix.CloseOnExec(m.queue)
	m.wakeR, m.wakeW, err = os.Pipe()
	if err != nil {
		return fail(err)
	}
	changes := []unix.Kevent_t{{Ident: uint64(m.wakeR.Fd()), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD | unix.EV_CLEAR}}
	for i, path := range paths[:len(paths)-1] {
		if beforeOpen != nil {
			if err := beforeOpen(path); err != nil {
				return fail(err)
			}
		}
		fd, err := unix.Open(path, unix.O_EVTONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return fail(fmt.Errorf("watch-router: open custody ancestor %s: %w", path, err))
		}
		m.bindings = append(m.bindings, routerWatcherAncestorBinding{path: path, file: os.NewFile(uintptr(fd), path)})
		if afterOpen != nil {
			if err := afterOpen(path); err != nil {
				return fail(err)
			}
		}
		flags := uint32(unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_ATTRIB | unix.NOTE_REVOKE)
		if i == 0 {
			flags |= unix.NOTE_WRITE // only the private directory; no broad directory scans
		}
		changes = append(changes, unix.Kevent_t{Ident: uint64(fd), Filter: unix.EVFILT_VNODE, Flags: unix.EV_ADD | unix.EV_CLEAR, Fflags: flags})
	}
	if _, err := unix.Kevent(m.queue, changes, nil, nil); err != nil {
		return fail(fmt.Errorf("watch-router: subscribe custody ancestors: %w", err))
	}
	if err := m.revalidate(); err != nil {
		return fail(err)
	}
	go m.run()
	return m, nil
}

func (m *routerWatcherAncestorMonitor) revalidate() error {
	return revalidateRouterWatcherAncestors(m.bindings)
}

func (m *routerWatcherAncestorMonitor) run() {
	defer close(m.done)
	for {
		var events [16]unix.Kevent_t
		n, err := unix.Kevent(m.queue, nil, events[:], nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			m.errors <- err
			return
		}
		for _, event := range events[:n] {
			if event.Ident == uint64(m.wakeR.Fd()) {
				return
			}
			if event.Flags&unix.EV_ERROR != 0 {
				m.errors <- fmt.Errorf("custody vnode event error: %d", event.Data)
				return
			}
			select {
			case m.changed <- struct{}{}:
			default:
			}
		}
	}
}

func (m *routerWatcherAncestorMonitor) Close() error {
	// Closing the private pipe writer delivers EOF to the registered reader.
	wakeErr := m.wakeW.Close()
	if wakeErr == nil {
		<-m.done
	}
	var err error
	for _, binding := range m.bindings {
		err = errors.Join(err, binding.file.Close())
	}
	return errors.Join(err, unix.Close(m.queue), m.wakeR.Close(), wakeErr)
}
