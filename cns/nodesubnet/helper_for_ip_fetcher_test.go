package nodesubnet

import "time"

// This method is in this file (_test.go) because it is a test helper method.
// The following method is built during tests, and is not part of the main code.
func (c *IPFetcher) SetSecondaryIPQueryInterval(interval time.Duration) {
	c.secondaryIPQueryInterval = interval
}
