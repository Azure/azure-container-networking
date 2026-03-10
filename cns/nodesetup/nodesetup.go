// Copyright Microsoft. All rights reserved.
// MIT License

// Package nodesetup performs one-time node-level preparation before CNS starts
// serving. What "getting the node ready" means is an implementation detail that
// varies by platform — for example, programming IP rules on Linux or configuring
// HNS on Windows.
package nodesetup
