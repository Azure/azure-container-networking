// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

type Controller interface {
	// Initialize the controller
	Run(stopCh <-chan struct{}) error
}
