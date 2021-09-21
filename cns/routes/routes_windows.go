// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build windows

package routes

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/Azure/azure-container-networking/cns/logger"
)

const (
	ipv4RoutingTableStart = "IPv4 Route Table"
	activeRoutesStart     = "Active Routes:"
)

func getInterfaceByAddress(address string) (int, error) {
	logger.Printf("[Azure CNS] getInterfaceByAddress")

	var ifaces []net.Interface
	logger.Printf("[Azure CNS] Going to obtain interface for address %s", address)
	ifaces, err := net.Interfaces()
	if err != nil {
		return -1, err
	}

	for i := 0; i < len(ifaces); i++ {
		logger.Debugf("[Azure CNS] Going to check interface %v", ifaces[i].Name)
		addrs, _ := ifaces[i].Addrs()
		for _, addr := range addrs {
			logger.Debugf("[Azure CNS] ipAddress being compared input=%v %v\n",
				address, addr.String())
			ip := strings.Split(addr.String(), "/")
			if len(ip) != 2 {
				return -1, fmt.Errorf("Malformed ip: %v", addr.String())
			}
			if ip[0] == address {
				return ifaces[i].Index, nil
			}
		}
	}

	return -1, fmt.Errorf(
		"[Azure CNS] Unable to determine interface index for address %s",
		address)
}

func getRoutes() ([]Route, error) {
	logger.Printf("[Azure CNS] getRoutes")

	c := exec.Command("cmd", "/C", "route", "print")
	var routePrintOutput string
	var routeCount int
	bytes, err := c.Output()
	if err == nil {
		routePrintOutput = string(bytes)
		logger.Debugf("[Azure CNS] Printing Routing table \n %v\n", routePrintOutput)
	} else {
		logger.Printf("Received error in printing routing table %v", err.Error())
		return nil, err
	}

	routePrint := strings.Split(routePrintOutput, ipv4RoutingTableStart)
	routeTable := strings.Split(routePrint[1], activeRoutesStart)
	tokens := strings.Split(
		strings.Split(routeTable[1], "Metric")[1],
		"=")
	table := tokens[0]
	routes := strings.Split(table, "\r")
	routeCount = len(routes)
	logger.Debugf("[Azure CNS] Recevied route count: %d", routeCount)
	if routeCount == 0 {
		return nil, nil
	}

	localRoutes := make([]Route, routeCount)
	cntr := 0
	truncated := 0
	for _, route := range routes {
		if route != "" {
			tokens := strings.Fields(route)
			if len(tokens) != 5 {
				logger.Printf("[Azure CNS] Ignoring route %s", route)
				truncated++
			} else {
				logger.Debugf("[Azure CNS] Parsing route: %s %s %s %s %s\n",
					tokens[0], tokens[1], tokens[2], tokens[3], tokens[4])
				rt := Route{
					destination: tokens[0],
					mask:        tokens[1],
					gateway:     tokens[2],
					metric:      tokens[4],
				}

				if rt.gateway == "On-link" {
					rt.gateway = "0.0.0.0"
				}

				index, err := getInterfaceByAddress(tokens[3])
				if err == nil {
					rt.ifaceIndex = index
					localRoutes[cntr] = rt
					cntr++
				} else {
					logger.Printf("[Azure CNS] Error encountered while obtaining index. %v\n", err.Error())
					truncated++
				}
			}
		}
	}

	if truncated == routeCount {
		localRoutes = nil
	} else {
		localRoutes = localRoutes[0 : routeCount-truncated-1]
	}

	return localRoutes, nil
}

func containsRoute(routes []Route, route Route) (bool, error) {
	logger.Printf("[Azure CNS] containsRoute")
	if routes == nil {
		return false, nil
	}
	for _, existingRoute := range routes {
		if existingRoute.destination == route.destination &&
			existingRoute.gateway == route.gateway &&
			existingRoute.ifaceIndex == route.ifaceIndex &&
			existingRoute.mask == route.mask {
			return true, nil
		}
	}
	return false, nil
}

func putRoutes(routes []Route) error {
	logger.Printf("[Azure CNS] putRoutes")

	var err error
	logger.Printf("[Azure CNS] Going to get current routes")
	currentRoutes, err := getRoutes()
	if err != nil {
		return err
	}

	for _, route := range routes {
		exists, err := containsRoute(currentRoutes, route)
		if err == nil && !exists {
			args := []string{
				"/C", "route", "ADD",
				route.destination,
				"MASK",
				route.mask,
				route.gateway,
				"METRIC",
				route.metric,
				"IF",
				fmt.Sprintf("%d", route.ifaceIndex),
			}
			logger.Printf("[Azure CNS] Adding missing route: %v", args)

			c := exec.Command("cmd", args...)
			bytes, err := c.Output()
			if err == nil {
				logger.Printf("[Azure CNS] Successfully executed add route: %v\n%v", args, string(bytes))
			} else {
				logger.Errorf("[Azure CNS] Failed to execute add route: %v\n%v", args, string(bytes))
			}
		} else {
			logger.Printf("[Azure CNS] Route already exists. skipping %+v", route)
		}
	}

	return err
}
