//go:build kgwctl

/*
Copyright kgateway Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	KgatewayGroupName = "gateway.kgateway.dev"
	// TODO replace with constant from kgateway api library when available https://github.com/kgateway-dev/kgateway/blob/main/api/labels/delegation.go
	KgatewayDelegationGroupName = "delegation.kgateway.dev"
)

var (
	DirectResponseGK    schema.GroupKind = schema.GroupKind{Group: KgatewayGroupName, Kind: "DirectResponse"}
	BackendGK           schema.GroupKind = schema.GroupKind{Group: KgatewayGroupName, Kind: "Backend"}
	TrafficPolicyGK     schema.GroupKind = schema.GroupKind{Group: KgatewayGroupName, Kind: "TrafficPolicy"}
	GatewayParametersGK schema.GroupKind = schema.GroupKind{Group: KgatewayGroupName, Kind: "GatewayParameters"}
	// LabelFauxGK respresents a type of objectRef that is not a real resource in the cluster.
	// It is used to in memory to create a relation between parent and child HTTPRoutes for delegation.
	// This should not create a node of type Label in the graph.
	LabelFauxGK schema.GroupKind = schema.GroupKind{Group: KgatewayDelegationGroupName, Kind: "label"}
)
