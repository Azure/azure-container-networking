// Copyright 2019 Microsoft. All rights reserved.
// MIT License

package network

import (
	"testing"
)

func TestGetPodName(t *testing.T) {
	testData := map[string]string{
		"nginx-deployment-5c689d88bb":       "nginx",
		"nginx-deployment-5c689d88bb-qwq47": "nginx-deployment",
		"nginx": "nginx",
	}

	for testValue, expectedPodName := range testData {
		podName := GetPodNameWithoutSuffix(testValue)

		if podName != expectedPodName {
			t.Error("Expected:", expectedPodName, ", Got: ", podName, ", For Test Value:", testValue)
		}
	}
}
