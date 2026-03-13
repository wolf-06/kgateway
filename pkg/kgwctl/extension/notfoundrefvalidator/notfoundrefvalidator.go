//go:build kgwctl

/*
Copyright 2024 The Kubernetes Authors.

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

package notfoundrefvalidator

import (
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"sigs.k8s.io/gwctl/pkg/common"
	gwctltopology "sigs.k8s.io/gwctl/pkg/topology"

	kgwcommon "github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/common"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/topology"
)

const (
	extensionName = "NotFoundReferenceValidator"
)

type Extension struct{}

func NewExtension() *Extension {
	return &Extension{}
}

func (a *Extension) Execute(graph *topology.Graph) error {
	graph.RemoveMetadata(extensionName)
	for _, relation := range graph.Relations {
		for _, fromNode := range graph.Nodes[relation.From] {
			if fromNode.Depth > graph.MaxDepth {
				klog.V(3).InfoS("Not validating resource since it's depth is greater than the max depth",
					"extension", extensionName, "resource", fromNode.GKNN(), "depth", fromNode.Depth, "MaxDepth", graph.MaxDepth,
				)
			}

			for _, toNodeGKNN := range relation.NeighborFunc(fromNode.Object) {
				err := common.ReferenceToNonExistentResourceError{ReferenceFromTo: common.ReferenceFromTo{
					ReferringObject: fromNode.GKNN(),
					ReferredObject:  toNodeGKNN,
				}}

				// Label selectors are not real resources in the cluster so they are not in the graph as nodes. So this needs to be done first.
				// Check to see if there is at least 1 HTTPRoute that matches the selector criteria.
				if strings.EqualFold(toNodeGKNN.GroupKind().String(), kgwcommon.LabelFauxGK.String()) {
					if _, ok := graph.Nodes[common.HTTPRouteGK]; !ok {
						continue
					}
					foundTarget := false
					for _, node := range graph.Nodes[common.HTTPRouteGK] {
						if node.GKNN().Namespace != toNodeGKNN.Namespace {
							continue
						}
						expectedLabelKey := kgwcommon.KgatewayDelegationGroupName + "/" + kgwcommon.LabelFauxGK.Kind
						labels := node.Object.GetLabels()
						if _, ok := labels[expectedLabelKey]; !ok {
							continue
						}
						if labels[expectedLabelKey] != toNodeGKNN.Name {
							continue
						}
						foundTarget = true
					}
					if !foundTarget {
						if err := a.puErrorInNode(fromNode, err); err != nil {
							return err
						}
						klog.V(1).Info(err)
					}
					continue
				}

				if _, ok := graph.Nodes[toNodeGKNN.GroupKind()]; !ok {
					if err := a.puErrorInNode(fromNode, err); err != nil {
						return err
					}
					klog.V(1).Info(err)
					continue
				}

				// For HTTPRoutes that delegate to other HTTPRoutes using a wildcard name,
				// Check to see if there is a least 1 HTTPRoute that matches the selector criteria.
				if toNodeGKNN.GroupKind() == common.HTTPRouteGK && (toNodeGKNN.Name == "*" || toNodeGKNN.Name == `"*"`) {
					foundTarget := false
					for _, node := range graph.Nodes[toNodeGKNN.GroupKind()] {
						if node.GKNN().Kind == toNodeGKNN.Kind && node.GKNN().Namespace == toNodeGKNN.Namespace {
							foundTarget = true
							break
						}
					}
					if !foundTarget {
						if err := a.puErrorInNode(fromNode, err); err != nil {
							return err
						}
						klog.V(1).Info(err)
					}
					continue
				}

				toNode := graph.Nodes[toNodeGKNN.GroupKind()][toNodeGKNN.NamespacedName()]
				if toNode == nil {
					if err := a.puErrorInNode(fromNode, err); err != nil {
						return err
					}
					klog.V(1).Info(err)
				}
			}
		}
	}
	return nil
}

func (a *Extension) puErrorInNode(node *gwctltopology.Node, notFoundErr error) error {
	if node.Metadata == nil {
		node.Metadata = map[string]any{}
	}
	if node.Metadata[extensionName] == nil {
		node.Metadata[extensionName] = &NodeMetadata{
			Errors: make([]error, 0),
		}
	}

	data, err := Access(node)
	if err != nil {
		return err
	}
	for _, err := range data.Errors {
		if err.Error() == notFoundErr.Error() {
			return nil
		}
	}
	data.Errors = append(data.Errors, notFoundErr)
	return nil
}

type NodeMetadata struct {
	Errors []error
}

func Access(node *gwctltopology.Node) (*NodeMetadata, error) {
	rawData, ok := node.Metadata[extensionName]
	if !ok || rawData == nil {
		klog.V(3).InfoS(fmt.Sprintf("no data found in node for %v", extensionName), "node", node.GKNN())
		return nil, nil
	}
	data, ok := rawData.(*NodeMetadata)
	if !ok {
		return nil, fmt.Errorf("unable to perform type assertion for %v in node %v", extensionName, node.GKNN())
	}
	return data, nil
}
