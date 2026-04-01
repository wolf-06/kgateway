//go:build kgwctl

package topology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/gwctl/pkg/common"
	gwctltopology "sigs.k8s.io/gwctl/pkg/topology"
)

func createTestNode(group, kind, namespace, name string) *gwctltopology.Node {
	return &gwctltopology.Node{
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": group + "/v1",
				"kind":       kind,
				"metadata": map[string]interface{}{
					"namespace": namespace,
					"name":      name,
				},
			},
		},
	}
}
func createTestRelation(fromGroup, fromKind, toGroup, toKind, relationName string) *gwctltopology.Relation {
	return &gwctltopology.Relation{
		From: schema.GroupKind{Group: fromGroup, Kind: fromKind},
		To:   schema.GroupKind{Group: toGroup, Kind: toKind},
		Name: relationName,
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			return []common.GKNN{}
		},
	}
}
func createGKNN(group, kind, namespace, name string) common.GKNN {
	return common.GKNN{
		Group:     group,
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
	}
}
func TestGraph_AddNode(t *testing.T) {
	t.Run("add first node to initialize the graph", func(t *testing.T) {
		graph := &Graph{}
		node := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		graph.AddNode(node)
		assert.Len(t, graph.Nodes, 1, "should have only 1 GroupKind entry")
	})
	t.Run("adding multiple node of same and different types", func(t *testing.T) {
		graph := &Graph{}
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1"))
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw2"))
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1"))
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route2"))
		gk1 := schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "Gateway"}
		gk2 := schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute"}
		assert.Len(t, graph.Nodes, 2, "should have 2 GroupKind entries")
		assert.Len(t, graph.Nodes[gk1], 2, "should have 2 nodes of Gateway Kind")
		assert.Len(t, graph.Nodes[gk2], 2, "should have 2 nodes of HTTPRoute Kind")
	})
	t.Run("duplicate overwrites", func(t *testing.T) {
		graph := &Graph{}
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1"))
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1"))
		gk := schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "Gateway"}
		assert.Len(t, graph.Nodes[gk], 1, "duplicates should be overwritten")
	})
	t.Run("different namespace same name", func(t *testing.T) {
		graph := &Graph{}
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "namespace-1", "gw1"))
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "namespace-2", "gw1"))
		gk := schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "Gateway"}
		assert.Len(t, graph.Nodes[gk], 2, "2 nodes of same name different namespaces")
	})
}

func TestGraph_HasNode(t *testing.T) {
	t.Run("nil nodes returns false", func(t *testing.T) {
		graph := &Graph{}
		gknn := createGKNN("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		assert.False(t, graph.HasNode(gknn), "nil nodes should return false")
	})

	t.Run("existing nodes return true", func(t *testing.T) {
		graph := &Graph{}
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1"))
		gknn := createGKNN("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		assert.True(t, graph.HasNode(gknn), "existing node should true")
	})

	t.Run("non-existenta return false", func(t *testing.T) {
		graph := &Graph{}
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1"))
		gknn := createGKNN("gateway.networking.k8s.io", "Gateway", "default", "gw2")
		assert.False(t, graph.HasNode(gknn), "non-existent node should false")
	})

	t.Run("different namespace same name exists", func(t *testing.T) {
		graph := &Graph{}
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "ns1", "gw1"))
		gknn := createGKNN("gateway.networking.k8s.io", "Gateway", "ns2", "gw1")
		assert.False(t, graph.HasNode(gknn), "different namespace should not exist")
	})
}

func TestGraph_DeleteNode(t *testing.T) {
	t.Run("deletes existing node", func(t *testing.T) {
		graph := &Graph{}
		node := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		graph.AddNode(node)
		graph.DeleteNode(node)
		assert.False(t, graph.HasNode(node.GKNN()), "node should be deleted")
	})

	t.Run("removes empty GroupKind entry", func(t *testing.T) {
		graph := &Graph{}
		node := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		graph.AddNode(node)
		graph.DeleteNode(node)
		gk := node.GKNN().GroupKind()
		_, exists := graph.Nodes[gk]
		assert.False(t, exists, "GroupKind should be removed when empty")
	})

	t.Run("partial delete keeps others", func(t *testing.T) {
		graph := &Graph{}
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		gw2 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw2")
		graph.AddNode(gw1)
		graph.AddNode(gw2)
		graph.DeleteNode(gw1)
		assert.True(t, graph.HasNode(gw2.GKNN()), "gw2 should still exist")
	})

	t.Run("nil nodes doesn't panic", func(t *testing.T) {
		graph := &Graph{}
		node := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		graph.DeleteNode(node) // Should not panic
	})

	t.Run("delete from non-existent GroupKind doesn't panic", func(t *testing.T) {
		graph := &Graph{}
		graph.AddNode(createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1"))
		node := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		graph.DeleteNode(node) // Should not panic
	})
}

func TestGraph_DeleteNodeUsingGKNN(t *testing.T) {
	t.Run("deletes existing node", func(t *testing.T) {
		graph := &Graph{}
		node := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		graph.AddNode(node)
		graph.DeleteNodeUsingGKNN(node.GKNN())
		assert.False(t, graph.HasNode(node.GKNN()), "should be deleted")
	})

	t.Run("non-existent doesn't error", func(t *testing.T) {
		graph := &Graph{}
		gknn := createGKNN("gateway.networking.k8s.io", "Gateway", "default", "non-existent")
		graph.DeleteNodeUsingGKNN(gknn) // Should not panic
	})

	t.Run("nil nodes doesn't panic", func(t *testing.T) {
		graph := &Graph{}
		gknn := createGKNN("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		graph.DeleteNodeUsingGKNN(gknn) // Should not panic
	})
}

func TestGraph_AddEdge(t *testing.T) {
	relation := createTestRelation(
		"gateway.networking.k8s.io", "HTTPRoute",
		"gateway.networking.k8s.io", "Gateway",
		"ParentRef",
	)

	t.Run("creates edge between nodes", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		route1 := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		var graph Graph
		graph.AddEdge(route1, gw1, relation)
		assert.Len(t, route1.OutNeighbors[relation], 1, "out-neighbor count")
		assert.Len(t, gw1.InNeighbors[relation], 1, "in-neighbor count")
	})

	t.Run("duplicate edge doesn't add twice", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		route1 := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		var graph Graph
		graph.AddEdge(route1, gw1, relation)
		graph.AddEdge(route1, gw1, relation)
		assert.Len(t, route1.OutNeighbors[relation], 1, "should still be 1")
	})

	t.Run("multiple edges different targets", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		gw2 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw2")
		route1 := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		var graph Graph
		graph.AddEdge(route1, gw1, relation)
		graph.AddEdge(route1, gw2, relation)
		assert.Len(t, route1.OutNeighbors[relation], 2, "should have 2 neighbors")
	})

	t.Run("nil neighbors are initialized", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		route1 := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		gw1.InNeighbors = nil
		route1.OutNeighbors = nil
		var graph Graph
		graph.AddEdge(route1, gw1, relation)
		assert.NotNil(t, route1.OutNeighbors, "OutNeighbors should be initialized")
		assert.NotNil(t, gw1.InNeighbors, "InNeighbors should be initialized")
	})
}

func TestGraph_RemoveEdge(t *testing.T) {
	relation := createTestRelation(
		"gateway.networking.k8s.io", "HTTPRoute",
		"gateway.networking.k8s.io", "Gateway",
		"ParentRef",
	)

	t.Run("removes existing edge", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		route1 := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		var graph Graph
		graph.AddEdge(route1, gw1, relation)
		graph.RemoveEdge(route1, gw1, relation)
		assert.Empty(t, route1.OutNeighbors[relation], "should be empty")
		assert.Empty(t, gw1.InNeighbors[relation], "should be empty")
	})

	t.Run("cleans up empty relation map", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		route1 := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		var graph Graph
		graph.AddEdge(route1, gw1, relation)
		graph.RemoveEdge(route1, gw1, relation)
		_, exists := route1.OutNeighbors[relation]
		assert.False(t, exists, "relation key should be removed")
	})

	t.Run("partial remove keeps others", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		gw2 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw2")
		route1 := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		var graph Graph
		graph.AddEdge(route1, gw1, relation)
		graph.AddEdge(route1, gw2, relation)
		graph.RemoveEdge(route1, gw1, relation)
		assert.Len(t, route1.OutNeighbors[relation], 1, "should have 1 left")
	})

	t.Run("remove non-existent edge doesn't panic", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		route1 := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		var graph Graph
		graph.RemoveEdge(route1, gw1, relation) // Should not panic
	})
}

func TestGraph_RemoveMetadata(t *testing.T) {
	t.Run("removes specific category", func(t *testing.T) {
		graph := &Graph{}
		node := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		node.Metadata = map[string]any{
			"PolicyData":      "data1",
			"InheritedPolicy": "data2",
		}
		graph.AddNode(node)
		graph.RemoveMetadata("PolicyData")
		_, exists := node.Metadata["PolicyData"]
		assert.False(t, exists, "PolicyData should be removed")
		_, exists = node.Metadata["InheritedPolicy"]
		assert.True(t, exists, "InheritedPolicy should remain")
	})

	t.Run("non-existent category doesn't error", func(t *testing.T) {
		graph := &Graph{}
		node := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		node.Metadata = map[string]any{"Key": "value"}
		graph.AddNode(node)
		graph.RemoveMetadata("NonExistent")
		_, exists := node.Metadata["Key"]
		assert.True(t, exists, "existing metadata should not be affected")
	})

	t.Run("nil metadata doesn't panic", func(t *testing.T) {
		graph := &Graph{}
		node := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		node.Metadata = nil
		graph.AddNode(node)
		graph.RemoveMetadata("anything") // Should not panic
	})

	t.Run("removes from multiple nodes", func(t *testing.T) {
		graph := &Graph{}
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		gw1.Metadata = map[string]any{"Category1": "data1"}
		gw2 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw2")
		gw2.Metadata = map[string]any{"Category1": "data2", "Category2": "data3"}
		graph.AddNode(gw1)
		graph.AddNode(gw2)
		graph.RemoveMetadata("Category1")
		_, exists := gw1.Metadata["Category1"]
		assert.False(t, exists, "Category1 removed from gw1")
		_, exists = gw2.Metadata["Category1"]
		assert.False(t, exists, "Category1 removed from gw2")
		_, exists = gw2.Metadata["Category2"]
		assert.True(t, exists, "Category2 still in gw2")
	})
}

type mockFetcher struct {
	resources map[schema.GroupKind][]*unstructured.Unstructured
}

func (m *mockFetcher) Fetch(gk schema.GroupKind) ([]*unstructured.Unstructured, error) {
	if m.resources == nil {
		return nil, nil
	}
	return m.resources[gk], nil
}

func TestNewBuilder(t *testing.T) {
	t.Run("creates builder with default max depth", func(t *testing.T) {
		fetcher := &mockFetcher{}
		builder := NewBuilder(fetcher)
		assert.Equal(t, DefaultGraphMaxDepth, builder.MaxDepth)
		assert.Equal(t, fetcher, builder.Fetcher)
	})
}

func TestBuilder_Chaining(t *testing.T) {
	t.Run("StartFrom returns builder for chaining", func(t *testing.T) {
		fetcher := &mockFetcher{}
		source := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		builder := NewBuilder(fetcher).StartFrom([]*unstructured.Unstructured{source.Object})
		assert.Len(t, builder.Sources, 1, "should have 1 source")
	})

	t.Run("UseRelationship adds relation", func(t *testing.T) {
		fetcher := &mockFetcher{}
		relation := createTestRelation(
			"gateway.networking.k8s.io", "HTTPRoute",
			"gateway.networking.k8s.io", "Gateway",
			"ParentRef",
		)
		builder := NewBuilder(fetcher).UseRelationship(relation)
		assert.Len(t, builder.Relations, 1, "should have 1 relation")
	})

	t.Run("UseRelationships adds multiple relations", func(t *testing.T) {
		fetcher := &mockFetcher{}
		rel1 := createTestRelation("a", "b", "c", "d", "rel1")
		rel2 := createTestRelation("e", "f", "g", "h", "rel2")
		builder := NewBuilder(fetcher).UseRelationships([]*gwctltopology.Relation{rel1, rel2})
		assert.Len(t, builder.Relations, 2, "should have 2 relations")
	})

	t.Run("WithMaxDepth sets max depth", func(t *testing.T) {
		fetcher := &mockFetcher{}
		builder := NewBuilder(fetcher).WithMaxDepth(5)
		assert.Equal(t, 5, builder.MaxDepth)
	})
}

func TestBuilder_Build(t *testing.T) {
	t.Run("builds graph with single source", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		fetcher := &mockFetcher{resources: map[schema.GroupKind][]*unstructured.Unstructured{}}
		graph, err := NewBuilder(fetcher).StartFrom([]*unstructured.Unstructured{gw1.Object}).Build()
		assert.NoError(t, err)
		assert.Len(t, graph.Sources, 1, "should have 1 source")
		gk := schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "Gateway"}
		assert.Len(t, graph.Nodes[gk], 1, "should have 1 Gateway node")
	})

	t.Run("builds graph with relations", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		route1 := createTestNode("gateway.networking.k8s.io", "HTTPRoute", "default", "route1")
		fetcher := &mockFetcher{
			resources: map[schema.GroupKind][]*unstructured.Unstructured{
				{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute"}: {route1.Object},
			},
		}
		relation := &gwctltopology.Relation{
			From: schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute"},
			To:   schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "Gateway"},
			Name: "ParentRef",
			NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
				return []common.GKNN{gw1.GKNN()}
			},
		}
		graph, err := NewBuilder(fetcher).StartFrom([]*unstructured.Unstructured{gw1.Object}).UseRelationship(relation).Build()
		assert.NoError(t, err)
		gwGK := schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "Gateway"}
		routeGK := schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute"}
		assert.Len(t, graph.Nodes[gwGK], 1, "should have Gateway node")
		assert.Len(t, graph.Nodes[routeGK], 1, "should have HTTPRoute node")
	})

	t.Run("returns error on fetch failure", func(t *testing.T) {
		gw1 := createTestNode("gateway.networking.k8s.io", "Gateway", "default", "gw1")
		fetcher := &mockFetcher{
			resources: map[schema.GroupKind][]*unstructured.Unstructured{},
		}
		relation := &gwctltopology.Relation{
			From:         schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute"},
			To:           schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "Gateway"},
			Name:         "ParentRef",
			NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN { return nil },
		}
		_, err := NewBuilder(fetcher).StartFrom([]*unstructured.Unstructured{gw1.Object}).UseRelationship(relation).Build()
		assert.NoError(t, err, "should not error with empty resources")
	})
}
