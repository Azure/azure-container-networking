// Copyright Microsoft. All rights reserved.
// MIT License

package nodesetup

import "go.uber.org/zap"

// Run performs one-time node-level setup.
// On Windows, no special node setup is currently required.
func Run(_ *zap.Logger) error {
	return nil
}
