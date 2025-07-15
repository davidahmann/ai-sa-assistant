// Copyright 2024 AI SA Assistant Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"testing"
)

func TestExtractQueryParameters(t *testing.T) {
	tests := []struct {
		name            string
		query           string
		expectedVmCount int
		expectedTechs   []string
		expectedClouds  []string
		expectedRTO     string
		expectedRPO     string
	}{
		{
			name:            "AWS migration with VM count",
			query:           "Generate a high-level lift-and-shift plan for migrating 120 on-prem Windows and Linux VMs to AWS",
			expectedVmCount: 120,
			expectedTechs:   []string{"Windows", "Linux"},
			expectedClouds:  []string{"AWS"},
			expectedRTO:     "",
			expectedRPO:     "",
		},
		{
			name:            "Azure DR with RTO/RPO",
			query:           "Design a DR solution in Azure for critical workloads with RTO = 2 hours and RPO = 15 minutes",
			expectedVmCount: 0,
			expectedTechs:   []string{},
			expectedClouds:  []string{"Azure"},
			expectedRTO:     "2 hours",
			expectedRPO:     "15 minutes",
		},
		{
			name:            "Complex query with multiple parameters",
			query:           "Outline a hybrid architecture connecting VMware environment to AWS with 50 VMs, RTO of 4 hours",
			expectedVmCount: 50,
			expectedTechs:   []string{"VMware"},
			expectedClouds:  []string{"AWS"},
			expectedRTO:     "4 hours",
			expectedRPO:     "",
		},
		{
			name:            "Simple query with no specific parameters",
			query:           "What are cloud migration best practices?",
			expectedVmCount: 0,
			expectedTechs:   []string{},
			expectedClouds:  []string{},
			expectedRTO:     "",
			expectedRPO:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := extractQueryParameters(tt.query)

			if params.VmCount != tt.expectedVmCount {
				t.Errorf("Expected VmCount %d, got %d", tt.expectedVmCount, params.VmCount)
			}

			if len(params.Technologies) != len(tt.expectedTechs) {
				t.Errorf("Expected %d technologies, got %d", len(tt.expectedTechs), len(params.Technologies))
			}

			if len(params.CloudProviders) != len(tt.expectedClouds) {
				t.Errorf("Expected %d cloud providers, got %d", len(tt.expectedClouds), len(params.CloudProviders))
			}

			if params.RTORequirement != tt.expectedRTO {
				t.Errorf("Expected RTO '%s', got '%s'", tt.expectedRTO, params.RTORequirement)
			}

			if params.RPORequirement != tt.expectedRPO {
				t.Errorf("Expected RPO '%s', got '%s'", tt.expectedRPO, params.RPORequirement)
			}

			// Check that technologies are correctly extracted
			for _, expected := range tt.expectedTechs {
				found := false
				for _, actual := range params.Technologies {
					if actual == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected technology '%s' not found in %v", expected, params.Technologies)
				}
			}

			// Check that cloud providers are correctly extracted
			for _, expected := range tt.expectedClouds {
				found := false
				for _, actual := range params.CloudProviders {
					if actual == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected cloud provider '%s' not found in %v", expected, params.CloudProviders)
				}
			}
		})
	}
}
