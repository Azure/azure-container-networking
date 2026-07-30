// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

// ibReadyEventName is the single global named object CNI uses to signal external
// components (e.g. the CRI proxy service) that CNI's side of the IB backend NIC
// work is done. It lives in the Global\ namespace so a service running in a
// different session can open/wait on it.
const ibReadyEventName = `Global\AzureCNI-IB-Ready`

// signalIBWorkDone signals external components that CNI has finished its side of
// the IB backend NIC work. It creates (or opens) an auto-reset Windows global
// named event and sets it, releasing any waiter (the CRI proxy service).
//
// This is best-effort: a failure to signal is logged but does not fail endpoint
// creation, since the CRI proxy can also poll/retry.
func signalIBWorkDone() error {
	return signalNamedEvent(ibReadyEventName)
}

// signalNamedEvent creates (or opens) an auto-reset Windows named event and sets
// it. Split out from signalIBWorkDone so it can be unit tested with a
// non-elevated (session-local) event name.
func signalNamedEvent(eventName string) error {
	name, err := windows.UTF16PtrFromString(eventName)
	if err != nil {
		logger.Error("Failed to encode IB ready event name", zap.String("event", eventName), zap.Error(err))
		return errors.Wrap(err, "failed to encode IB ready event name")
	}

	// manualReset=0 (auto-reset), initialState=0 (non-signaled). Creates the event
	// if it does not exist, otherwise opens the existing one.
	handle, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil {
		// CreateEvent returns ERROR_ALREADY_EXISTS as err even on success (handle is valid).
		if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			logger.Error("Failed to create IB ready event", zap.String("event", eventName), zap.Error(err))
			return errors.Wrap(err, "failed to create IB ready event")
		}
	}
	defer windows.CloseHandle(handle) //nolint:errcheck // best-effort cleanup

	if err := windows.SetEvent(handle); err != nil {
		logger.Error("Failed to set IB ready event", zap.String("event", eventName), zap.Error(err))
		return errors.Wrap(err, "failed to set IB ready event")
	}

	logger.Info("Signaled IB work done to external components", zap.String("event", eventName))
	return nil
}
