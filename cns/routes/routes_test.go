// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows && pipeline
// +build windows,pipeline

package routes

import (
	"os"
	"os/exec"
	"testing"

	"github.com/Azure/azure-container-networking/log"
)

const (
	testDest    = "159.254.169.254"
	testMask    = "255.255.255.255"
	testGateway = "0.0.0.0"
	testMetric  = "12"
)

// Wraps the test run.
func TestMain(m *testing.M) {
	// Run tests.
	exitCode := m.Run()
	os.Exit(exitCode)
}

func addTestRoute() error {
	arg := []string{
		"/C", "route", "ADD", testDest,
		"MASK", testMask, testGateway, "METRIC", testMetric,
	}
	log.Printf("[Azure CNS] Adding missing route: %v", arg)

	c := exec.Command("cmd", arg...)
	bytes, err := c.Output()
	if err == nil {
		log.Printf("[Azure CNS] Successfully executed add route: %v\n%v",
			arg, string(bytes))
	} else {
		log.Printf("[Azure CNS] Failed to execute add route: %v\n%v\n%v",
			arg, string(bytes), err.Error())
		return err
	}

	return nil
}

func deleteTestRoute() error {
	args := []string{
		"/C", "route", "DELETE", testDest, "MASK", testMask,
		testGateway, "METRIC", testMetric,
	}
	log.Printf("[Azure CNS] Deleting route: %v", args)

	c := exec.Command("cmd", args...)
	bytes, err := c.Output()
	if err == nil {
		log.Printf("[Azure CNS] Successfully executed delete route: %v\n%v",
			args, string(bytes))
	} else {
		log.Printf("[Azure CNS] Failed to execute delete route: %v\n%v\n%v",
			args, string(bytes), err.Error())
		return err
	}

	return nil
}

// TestPutRoutes tests if a missing route is properly restored or not.
func TestRestoreMissingRoute(t *testing.T) {
	log.Printf("Test: PutMissingRoutes")

	err := addTestRoute()
	if err != nil {
		t.Errorf("add route failed %+v", err.Error())
	}

	rt := &RoutingTable{}

	// save routing table.
	rt.GetRoutingTable()
	cntr := 0
	for _, rt := range rt.Routes {
		log.Printf("[]: Route[%d]: %+v", cntr, rt)
		cntr++
	}

	// now delete the route so it goes missing.
	err = deleteTestRoute()
	if err != nil {
		t.Errorf("delete route failed %+v", err.Error())
	}

	// now restore the deleted route.
	rt.RestoreRoutingTable()

	// test if route was resotred or not.
	rt.GetRoutingTable()
	restored := false
	for _, existingRoute := range rt.Routes {
		log.Printf("Comapring %+v\n", existingRoute)
		if existingRoute.destination == testDest &&
			existingRoute.gateway == testGateway &&
			existingRoute.mask == testMask {
			restored = true
		}
	}

	if !restored {
		t.Errorf("unable to restore missing route")
	} else {
		err = deleteTestRoute()
		if err != nil {
			t.Errorf("delete route failed %+v", err.Error())
		}
	}
}
