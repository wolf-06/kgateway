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

package kgateway

import (
	"fmt"
	"maps"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwcommon "sigs.k8s.io/gwctl/pkg/common"
	topology "sigs.k8s.io/gwctl/pkg/topology"
	topologygw "sigs.k8s.io/gwctl/pkg/topology/gateway"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/common"
)

var AllRelations = append(
	[]*topology.Relation{
		HTTPRouteChildDirectResponseRefsRelation,
		HTTPRouteChildBackendRefsRelation,
		GatewayChildGatewayParametersRelation,
		HTTPRouteChildHTTPRouteRefsRelation,
		HTTPRouteParentRelation,
		HTTPRouteChildLabeledRelation,
	},
	[]*topology.Relation{
		topologygw.GatewayParentGatewayClassRelation,
		topologygw.HTTPRouteChildBackendRefsRelation,
		topologygw.GatewayNamespace,
		topologygw.HTTPRouteNamespace,
		topologygw.BackendNamespace,
	}...,
)

var (
	HTTPRouteChildDirectResponseRefsRelation = &topology.Relation{
		From: gwcommon.HTTPRouteGK,
		To:   common.DirectResponseGK,
		Name: "DirectResponse",
		NeighborFunc: func(u *unstructured.Unstructured) []gwcommon.GKNN {
			httpRoute := &gatewayv1.HTTPRoute{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), httpRoute); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured HTTPRoute to structured: %v", err))
			}
			// Aggregate all DirectResponses
			var directResponseRefs []gatewayv1.LocalObjectReference
			for _, backendRef := range httpRoute.Spec.Rules {
				for _, filter := range backendRef.Filters {
					if filter.Type != gatewayv1.HTTPRouteFilterExtensionRef {
						continue
					}
					if filter.ExtensionRef == nil {
						continue
					}
					if !strings.EqualFold(string(filter.ExtensionRef.Kind), common.DirectResponseGK.Kind) {
						continue
					}
					directResponseRefs = append(directResponseRefs, *filter.ExtensionRef)
				}
			}

			// Convert each BackendRef to GKNN. GNKK does not use pointers and
			// thus is easily comparable.
			resultSet := make(map[gwcommon.GKNN]bool)
			for _, directResponseRef := range directResponseRefs {
				objRef := gwcommon.GKNN{
					Name:      string(directResponseRef.Name),
					Namespace: httpRoute.GetNamespace(), // LocalObjectReference does not have a namespace
				}
				if directResponseRef.Group != "" {
					objRef.Group = string(directResponseRef.Group)
				}
				objRef.Kind = string(directResponseRef.Kind)
				resultSet[objRef] = true
			}

			// Return unique objRefs
			var result []gwcommon.GKNN
			for objRef := range resultSet {
				result = append(result, objRef)
			}
			return result
		},
	}

	// HTTPRouteChildBackendRefsRelation returns Backends which the HTTPRoute
	// references.
	HTTPRouteChildBackendRefsRelation = &topology.Relation{
		From: gwcommon.HTTPRouteGK,
		To:   common.BackendGK,
		Name: "BackendRef",
		NeighborFunc: func(u *unstructured.Unstructured) []gwcommon.GKNN {
			httpRoute := &gatewayv1.HTTPRoute{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), httpRoute); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured HTTPRoute to structured: %v", err))
			}
			// Aggregate all BackendRefs
			var backendRefs []gatewayv1.BackendObjectReference
			for _, rule := range httpRoute.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					if strings.EqualFold(string(*backendRef.Kind), common.BackendGK.Kind) && strings.EqualFold(string(*backendRef.Group), common.BackendGK.Group) {
						backendRefs = append(backendRefs, backendRef.BackendObjectReference)
					}
				}
				for _, filter := range rule.Filters {
					if filter.Type != gatewayv1.HTTPRouteFilterRequestMirror {
						continue
					}
					if filter.RequestMirror == nil {
						continue
					}
					if !strings.EqualFold(string(filter.ExtensionRef.Kind), common.BackendGK.Kind) {
						continue
					}
					backendRefs = append(backendRefs, filter.RequestMirror.BackendRef)
				}
			}

			// Convert each BackendRef to GKNN. GNKK does not use pointers and
			// thus is easily comparable.
			resultSet := make(map[gwcommon.GKNN]bool)
			for _, backendRef := range backendRefs {
				objRef := gwcommon.GKNN{
					Name: string(backendRef.Name),
					// Assume namespace is unspecified in the backendRef and
					// check later to override the default value.
					Namespace: httpRoute.GetNamespace(),
				}
				if backendRef.Group != nil {
					objRef.Group = string(*backendRef.Group)
				}
				objRef.Kind = string(*backendRef.Kind)
				if backendRef.Namespace != nil {
					objRef.Namespace = string(*backendRef.Namespace)
				}
				resultSet[objRef] = true
			}

			// Return unique objRefs
			var result []gwcommon.GKNN
			for objRef := range resultSet {
				result = append(result, objRef)
			}
			return result
		},
	}

	GatewayChildGatewayParametersRelation = &topology.Relation{
		From: gwcommon.GatewayGK,
		To:   common.GatewayParametersGK,
		Name: "GatewayParameters",
		NeighborFunc: func(u *unstructured.Unstructured) []gwcommon.GKNN {
			gateway := &gatewayv1.Gateway{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), gateway); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured Gateway to structured: %v", err))
			}
			if gateway.Spec.Infrastructure == nil || gateway.Spec.Infrastructure.ParametersRef == nil {
				return nil
			}
			return []gwcommon.GKNN{
				{
					Name:      string(gateway.Spec.Infrastructure.ParametersRef.Name),
					Group:     string(gateway.Spec.Infrastructure.ParametersRef.Group),
					Kind:      string(gateway.Spec.Infrastructure.ParametersRef.Kind),
					Namespace: gateway.GetNamespace(),
				},
			}
		},
	}

	HTTPRouteChildHTTPRouteRefsRelation = &topology.Relation{
		From: gwcommon.HTTPRouteGK,
		To:   gwcommon.HTTPRouteGK,
		Name: "HTTPRouteDelegationChildRef",
		NeighborFunc: func(u *unstructured.Unstructured) []gwcommon.GKNN {
			httpRoute := &gatewayv1.HTTPRoute{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), httpRoute); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured HTTPRoute to structured: %v", err))
			}
			// Aggregate all BackendRefs
			var backendRefs []gatewayv1.BackendObjectReference
			for _, rule := range httpRoute.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					if strings.EqualFold(string(*backendRef.Kind), gwcommon.HTTPRouteGK.Kind) && strings.EqualFold(string(*backendRef.Group), gwcommon.HTTPRouteGK.Group) {
						backendRefs = append(backendRefs, backendRef.BackendObjectReference)
					}
				}
				for _, filter := range rule.Filters {
					if strings.ToLower(string(filter.Type)) != "httproute" {
						continue
					}
					if filter.RequestMirror == nil {
						continue
					}
					if !strings.EqualFold(string(filter.ExtensionRef.Kind), gwcommon.HTTPRouteGK.Kind) {
						continue
					}
					backendRefs = append(backendRefs, filter.RequestMirror.BackendRef)
				}
			}

			// Convert each BackendRef to GKNN. GNKK does not use pointers and
			// thus is easily comparable.
			resultSet := make(map[gwcommon.GKNN]bool)
			for _, backendRef := range backendRefs {
				objRef := gwcommon.GKNN{
					Name: string(backendRef.Name),
					// Assume namespace is unspecified in the backendRef and
					// check later to override the default value.
					Namespace: httpRoute.GetNamespace(),
				}
				if backendRef.Group != nil {
					objRef.Group = string(*backendRef.Group)
				}
				objRef.Kind = string(*backendRef.Kind)
				if backendRef.Namespace != nil {
					objRef.Namespace = string(*backendRef.Namespace)
				}
				resultSet[objRef] = true
			}

			// Return unique objRefs
			var result []gwcommon.GKNN
			for objRef := range resultSet {
				result = append(result, objRef)
			}
			return result
		},
	}

	HTTPRouteParentRelation = &topology.Relation{
		From: gwcommon.HTTPRouteGK,
		To:   gwcommon.HTTPRouteGK,
		Name: "ParentRef",
		NeighborFunc: func(u *unstructured.Unstructured) []gwcommon.GKNN {
			httpRoute := &gatewayv1.HTTPRoute{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), httpRoute); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured HTTPRoute to structured: %v", err))
			}
			result := []gwcommon.GKNN{}
			for _, gatewayRef := range httpRoute.Spec.ParentRefs {
				namespace := httpRoute.GetNamespace()
				if namespace == "" {
					namespace = metav1.NamespaceDefault
				}
				if gatewayRef.Namespace != nil {
					namespace = string(*gatewayRef.Namespace)
				}
				var group string
				if gatewayRef.Group != nil {
					group = string(*gatewayRef.Group)
				}
				if group == "" {
					group = gwcommon.GatewayGK.Group
				}
				var kind string
				if gatewayRef.Kind != nil {
					kind = string(*gatewayRef.Kind)
				}
				if kind == "" {
					kind = gwcommon.GatewayGK.Kind
				}
				result = append(result, gwcommon.GKNN{
					Group:     group,
					Kind:      kind,
					Namespace: namespace,
					Name:      string(gatewayRef.Name),
				})
			}
			return result
		},
	}

	HTTPRouteChildLabeledRelation = &topology.Relation{
		From: gwcommon.HTTPRouteGK,
		To:   common.LabelFauxGK,
		Name: "Label",
		NeighborFunc: func(u *unstructured.Unstructured) []gwcommon.GKNN {
			httpRoute := &gatewayv1.HTTPRoute{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), httpRoute); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured HTTPRoute to structured: %v", err))
			}
			// Aggregate all BackendRefs
			var backendRefs []gatewayv1.BackendObjectReference
			for _, rule := range httpRoute.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					if strings.EqualFold(string(*backendRef.Kind), common.LabelFauxGK.Kind) && strings.EqualFold(string(*backendRef.Group), common.LabelFauxGK.Group) {
						backendRefs = append(backendRefs, backendRef.BackendObjectReference)
					}
				}
				for _, filter := range rule.Filters {
					if strings.ToLower(string(filter.Type)) != "httproute" {
						continue
					}
					if filter.RequestMirror == nil {
						continue
					}
					if !strings.EqualFold(string(filter.ExtensionRef.Kind), common.LabelFauxGK.Kind) {
						continue
					}
					backendRefs = append(backendRefs, filter.RequestMirror.BackendRef)
				}
			}

			// Convert each BackendRef to GKNN. GNKK does not use pointers and
			// thus is easily comparable.
			resultSet := make(map[gwcommon.GKNN]bool)
			for _, backendRef := range backendRefs {
				objRef := gwcommon.GKNN{
					Name: string(backendRef.Name),
					// Assume namespace is unspecified in the backendRef and
					// check later to override the default value.
					Namespace: httpRoute.GetNamespace(),
				}
				if backendRef.Group != nil {
					objRef.Group = string(*backendRef.Group)
				}
				objRef.Kind = string(*backendRef.Kind)
				if backendRef.Namespace != nil {
					objRef.Namespace = string(*backendRef.Namespace)
				}
				resultSet[objRef] = true
			}

			// Return unique objRefs
			var result []gwcommon.GKNN
			for objRef := range resultSet {
				result = append(result, objRef)
			}
			return result
		},
	}
)

type gatewayClassNode interface {
	Gateways() map[gwcommon.GKNN]*topology.Node
}

type gatewayNodeClassImpl struct {
	node *topology.Node
}

func GatewayClassNode(node *topology.Node) gatewayClassNode { //nolint:revive
	return &gatewayNodeClassImpl{node: node}
}

func (n *gatewayNodeClassImpl) Gateways() map[gwcommon.GKNN]*topology.Node {
	return n.node.InNeighbors[topologygw.GatewayParentGatewayClassRelation]
}

type gatewayNode interface {
	Namespace() *topology.Node
	GatewayClass() *topology.Node
	HTTPRoutes() map[gwcommon.GKNN]*topology.Node
}

type gatewayNodeImpl struct {
	node *topology.Node
}

func GatewayNode(node *topology.Node) gatewayNode { //nolint:revive
	return &gatewayNodeImpl{node: node}
}

func (n *gatewayNodeImpl) Namespace() *topology.Node {
	for _, namespaceNode := range n.node.OutNeighbors[topologygw.GatewayNamespace] {
		return namespaceNode
	}
	return nil
}

func (n *gatewayNodeImpl) GatewayClass() *topology.Node {
	for _, gatewayClassNode := range n.node.OutNeighbors[topologygw.GatewayParentGatewayClassRelation] {
		return gatewayClassNode
	}
	return nil
}

func (n *gatewayNodeImpl) HTTPRoutes() map[gwcommon.GKNN]*topology.Node {
	return n.node.InNeighbors[HTTPRouteParentRelation]
}

type httpRouteNode interface {
	Namespace() *topology.Node
	Gateways() map[gwcommon.GKNN]*topology.Node
	Backends() map[gwcommon.GKNN]*topology.Node
	DirectResponses() map[gwcommon.GKNN]*topology.Node
}

type httpRouteNodeImpl struct {
	node *topology.Node
}

func HTTPRouteNode(node *topology.Node) httpRouteNode {
	return &httpRouteNodeImpl{node: node}
}

func (n *httpRouteNodeImpl) Namespace() *topology.Node {
	for _, namespaceNode := range n.node.OutNeighbors[topologygw.HTTPRouteNamespace] {
		return namespaceNode
	}
	return nil
}

func (n *httpRouteNodeImpl) Gateways() map[gwcommon.GKNN]*topology.Node {
	return n.node.OutNeighbors[HTTPRouteParentRelation]
}

func (n *httpRouteNodeImpl) Backends() map[gwcommon.GKNN]*topology.Node {
	backends := make(map[gwcommon.GKNN]*topology.Node)
	maps.Copy(backends, n.node.OutNeighbors[topologygw.HTTPRouteChildBackendRefsRelation])
	maps.Copy(backends, n.node.OutNeighbors[HTTPRouteChildBackendRefsRelation])
	maps.Copy(backends, n.node.OutNeighbors[HTTPRouteChildHTTPRouteRefsRelation])
	maps.Copy(backends, n.node.OutNeighbors[HTTPRouteChildDirectResponseRefsRelation])
	maps.Copy(backends, n.node.OutNeighbors[HTTPRouteChildLabeledRelation])
	return backends
}

func (n *httpRouteNodeImpl) DirectResponses() map[gwcommon.GKNN]*topology.Node {
	return n.node.OutNeighbors[HTTPRouteChildDirectResponseRefsRelation]
}

type backendNode interface {
	Namespace() *topology.Node
	HTTPRoutes() map[gwcommon.GKNN]*topology.Node
}

type backendNodeImpl struct {
	node *topology.Node
}

func BackendNode(node *topology.Node) backendNode {
	return &backendNodeImpl{node: node}
}

func (n *backendNodeImpl) Namespace() *topology.Node {
	for _, namespaceNode := range n.node.OutNeighbors[topologygw.BackendNamespace] {
		return namespaceNode
	}
	return nil
}

func (n *backendNodeImpl) HTTPRoutes() map[gwcommon.GKNN]*topology.Node {
	return n.node.InNeighbors[topologygw.HTTPRouteChildBackendRefsRelation]
}

type directResponseNode interface {
	Namespace() *topology.Node
	HTTPRoutes() map[gwcommon.GKNN]*topology.Node
}

type directResponseNodeImpl struct {
	node *topology.Node
}

func DirectResponseNode(node *topology.Node) directResponseNode {
	return &directResponseNodeImpl{node: node}
}

func (n *directResponseNodeImpl) Namespace() *topology.Node {
	for _, namespaceNode := range n.node.OutNeighbors[topologygw.BackendNamespace] {
		return namespaceNode
	}
	return nil
}

func (n *directResponseNodeImpl) HTTPRoutes() map[gwcommon.GKNN]*topology.Node {
	return n.node.InNeighbors[HTTPRouteChildDirectResponseRefsRelation]
}
