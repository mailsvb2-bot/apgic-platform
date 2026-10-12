package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// This reference adapter stores signed provider outbox sequences and role
// grants in atomic local files. A second process sharing that directory
// would fork the authoritative provider stream. Fail closed instead.
// A horizontally scalable CommunicationProvider must replace this reference
// with a transactional shared-state owner and equivalent stream leases.
func lockProviderState(directory string) (func(), error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create durable reference state: %w", err)
	}
	path := filepath.Join(directory, ".communication-provider.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open communication provider lease: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("another provider owns the durable stream: %w", errors.New("reference singleton lease unavailable"))
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
