/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Based on code from https://github.com/yevgeny-shnaidman/amd-gpu-operator

Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
SPDX-License-Identifier: BSD-3-Clause-Clear
Not a contribution.
*/

package v1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	QTIPCIVendorID = "17cb"
)

// AICSpec defines the desired state of AIC
type AICSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// defines image that includes driver source and firmware
	SourceImage string `json:"sourceImage"`

	// defines image that includes drivers and firmware blobs
	DriversImage string `json:"driversImage"`

	// version of the drivers source code, can be used as part of image of dockerfile source image
	DriversVersion string `json:"driversVersion"`

	// device plugin image
	DevicePluginImage string `json:"devicePluginImage"`

	// version of the device plugin image
	DevPluginVersion string `json:"devPluginVersion"`

	// QAIC Device Agent image
	DeviceAgentImage string `json:"deviceAgentImage"`

	// version of the Device Agent image
	DeviceAgentVersion string `json:"deviceAgentVersion"`

	// pull secrets used for pull/setting images used by operator
	// +optional
	ImageRepoSecret *v1.LocalObjectReference `json:"imageRepoSecret,omitempty"`

	// Selector describes on which nodes the AIC Operator should enable the AIC device.
	// +optional
	Selector map[string]string `json:"selector,omitempty"`

	// UseInTreeModules used to prefer in-tree modules, if not available still tries to build
	// +optional
	UseInTreeModules bool `json:"useInTreeModules,omitempty"`
}

type State string

const (
	Ignored        State = "Ignored"
	Operational    State = "Ready"
	NotOperational State = "NotReady"
)

// AICStatus defines the observed state of AIC
type AICStatus struct {
	// +kubebuilder:validation:Enum=Ignored;Ready;NotReady
	State State `json:"state"`

	// ObservedGeneration is the most recent generation evaluated by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describe the current readiness and prerequisite state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Readiness summarizes selected nodes without exposing node metadata beyond names.
	// +optional
	Readiness AICReadinessSummary `json:"readiness,omitempty"`

	// Nodes contains a bounded list of selected node observations.
	// +optional
	Nodes []AICNodeReadiness `json:"nodes,omitempty"`

	// NextAction is a concise administrator-facing troubleshooting action.
	// +optional
	NextAction string `json:"nextAction,omitempty"`
}

type AICReadinessSummary struct {
	Selected    int32 `json:"selected"`
	Ready       int32 `json:"ready"`
	Degraded    int32 `json:"degraded"`
	Unavailable int32 `json:"unavailable"`
}

type AICNodeState string

const (
	AICNodeReady       AICNodeState = "Ready"
	AICNodeDegraded    AICNodeState = "Degraded"
	AICNodeUnavailable AICNodeState = "Unavailable"
	AICNodeMismatched  AICNodeState = "Mismatched"
)

type AICNodeReadiness struct {
	Name   string       `json:"name"`
	State  AICNodeState `json:"state"`
	Reason string       `json:"reason"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AIC is the Schema for the aics API
type AIC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AICSpec   `json:"spec,omitempty"`
	Status AICStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AICList contains a list of AIC
type AICList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIC `json:"items"`
}

type LoadedModules int

const (
	Qaic_loaded LoadedModules = iota
	Mhi_loaded  LoadedModules = iota
	None_loaded LoadedModules = iota
)

func init() {
	SchemeBuilder.Register(&AIC{}, &AICList{})
}
