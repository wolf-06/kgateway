//go:build kgwctl

/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.

You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

This file is a fork of sigs.k8s.io/gwctl/cmd/get/get.go
It is modified to use use kgateway resource relations in the topology graph and our custom extensions.
*/
package get

import (
	"fmt"
	"os"
	"strings"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/config"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/utils/clock"

	gwcommon "sigs.k8s.io/gwctl/pkg/common"
	gwctlflags "sigs.k8s.io/gwctl/pkg/flags"
	gwctltopology "sigs.k8s.io/gwctl/pkg/topology"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/policymanager"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/printer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/topology"
	topologykgw "github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/topology/kgateway"
)

func NewCmd(factory gwcommon.Factory, iostreams genericiooptions.IOStreams, isDescribe bool, cliConfig *config.CliConfig) *cobra.Command {
	flags := newGetFlags()

	cmdName := "get"
	if isDescribe {
		cmdName = "describe"
	}

	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%v TYPE [RESOURCE_NAME]", cmdName),
		Short: "Display one or many resources",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(_ *cobra.Command, args []string) {
			o, err := flags.ToOptions(args, factory, iostreams, isDescribe, cliConfig)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v", err)
				os.Exit(1)
			}

			err = o.Run(args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v", err)
				os.Exit(1)
			}
		},
	}

	flags.resourceBuilderFlags.AddFlags(cmd.Flags())

	if !isDescribe {
		printableAllowedFormats := strings.Join(printer.AllowedOutputFormatsForHelp(), ",")
		cmd.Flags().StringVarP(&flags.outputFormat, "output", "o", "", fmt.Sprintf("Output format. Must be one of: %v", printableAllowedFormats))

		flags.forFlag.AddFlag(cmd.Flags())
	}

	return cmd
}

// getFlags contains the flags used with get command.
type getFlags struct {
	resourceBuilderFlags *genericclioptions.ResourceBuilderFlags
	outputFormat         string
	forFlag              gwctlflags.ForFlag
}

func newGetFlags() *getFlags {
	resourceBuilderFlags := genericclioptions.NewResourceBuilderFlags().
		WithAllNamespaces(false).
		WithLabelSelector("")
	resourceBuilderFlags.FileNameFlags = nil

	return &getFlags{
		resourceBuilderFlags: resourceBuilderFlags,
	}
}

func (f *getFlags) ToOptions(args []string, factory gwcommon.Factory, iostreams genericiooptions.IOStreams, isDescribe bool, cliConfig *config.CliConfig) (*getOptions, error) {
	o := &getOptions{
		isDescribe:        isDescribe,
		factory:           factory,
		IOStreams:         iostreams,
		relations:         cliConfig.Relations,
		executeExtensions: cliConfig.ExecuteExtensions,
		allNamespaces:     *f.resourceBuilderFlags.AllNamespaces,
		labelSelector:     *f.resourceBuilderFlags.LabelSelector,
	}

	var err error
	o.isPolicy, o.isPolicyCRD, err = parseResourceTypeOrNameArgs(args)
	if err != nil {
		return nil, err
	}

	o.namespace, _, err = factory.KubeConfigNamespace()
	if err != nil {
		return nil, err
	}

	// Parse outputFormat
	o.output, err = printer.ValidateAndReturnOutputFormat(f.outputFormat)
	if err != nil {
		return nil, err
	}

	return o, nil
}

type getOptions struct {
	isDescribe bool

	factory           gwcommon.Factory
	relations         []*gwctltopology.Relation
	executeExtensions func(graph *topology.Graph, factory gwcommon.Factory, sources []*unstructured.Unstructured) error

	allNamespaces bool
	namespace     string
	labelSelector string
	output        printer.OutputFormat

	isPolicy    bool
	isPolicyCRD bool

	genericclioptions.IOStreams
}

func (o *getOptions) Run(args []string) error {
	if o.isPolicy || o.isPolicyCRD {
		return o.handlePolicy(args)
	}
	infos, err := o.factory.NewBuilder().
		Unstructured().
		Flatten().
		NamespaceParam(o.namespace).DefaultNamespace().AllNamespaces(o.allNamespaces).
		ResourceTypeOrNameArgs(true, args...).
		LabelSelectorParam(o.labelSelector).
		ContinueOnError().
		Do().
		Infos()
	if err != nil {
		return err
	}

	sources := []*unstructured.Unstructured{}
	for _, info := range infos {
		o, err := runtime.DefaultUnstructuredConverter.ToUnstructured(info.Object) //nolint:govet
		if err != nil {
			return err
		}
		u := &unstructured.Unstructured{Object: o}
		sources = append(sources, u)
	}

	var graph *topology.Graph
	if o.isDescribe || o.output == printer.OutputFormatWide || o.output == printer.OutputFormatGraph {
		graph, err = topology.NewBuilder(gwcommon.NewDefaultGroupKindFetcher(o.factory)).
			StartFrom(sources).
			UseRelationships(o.relations).
			Build()
		if err != nil {
			return err
		}

		if err := o.executeExtensions(graph, o.factory, sources); err != nil {
			return err
		}
	} else {
		graph, err = topology.NewBuilder(gwcommon.NewDefaultGroupKindFetcher(o.factory)).
			StartFrom(sources).
			Build()
		if err != nil {
			return err
		}
	}

	if o.output == printer.OutputFormatGraph {
		toDotGraph, err := topologykgw.ToDot(graph)
		if err != nil {
			return err
		}
		fmt.Fprintf(o.IOStreams.Out, "%v\n", string(toDotGraph))

		return nil
	}

	return o.printNodes(graph.Sources)
}

func (o *getOptions) handlePolicy(args []string) error {
	policyManager := policymanager.New(gwcommon.NewDefaultGroupKindFetcher(o.factory))
	if err := policyManager.Init(); err != nil {
		return err
	}

	nodes := []*gwctltopology.Node{}
	if o.isPolicy {
		for _, policy := range policyManager.GetPolicies() {
			shouldSkip := (!o.allNamespaces && o.namespace != policy.GKNN().Namespace) ||
				(len(args) == 2 && args[1] != policy.GKNN().Name)
			if shouldSkip {
				continue
			}
			nodes = append(nodes, encodePolicyAsNode(policy))
		}
	} else {
		for _, policyCRD := range policyManager.GetCRDs() {
			shouldSkip := len(args) == 2 && (args[1] != policyCRD.CRD.GetName())
			if shouldSkip {
				continue
			}
			node, err := encodePolicyCRDAsNode(policyCRD)
			if err != nil {
				return err
			}
			nodes = append(nodes, node)
		}
	}

	return o.printNodes(nodes)
}

func (o *getOptions) printNodes(nodes []*gwctltopology.Node) error {
	printerOptions := printer.PrinterOptions{
		OutputFormat: o.output,
		Clock:        clock.RealClock{},
		Description:  o.isDescribe,
		EventFetcher: printer.NewDefaultEventFetcher(o.factory),
	}
	p := printer.NewPrinter(printerOptions)
	defer p.Flush(o.IOStreams.Out)
	for _, node := range gwctltopology.SortedNodes(nodes) {
		err := p.PrintNode(node, o.IOStreams.Out)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseResourceTypeOrNameArgs(args []string) (isPolicy, isPolicyCRD bool, err error) {
	if strings.Contains(args[0], ",") {
		return false, false, fmt.Errorf("cannot specify more than one type, received types: %v", strings.Split(args[0], ","))
	}

	switch args[0] {
	case "policy", "policies":
		isPolicy = true

	case "policycrd", "policycrds":
		isPolicyCRD = true
	}

	return isPolicy, isPolicyCRD, nil
}

func encodePolicyAsNode(policy *policymanager.Policy) *gwctltopology.Node {
	return &gwctltopology.Node{
		Object: policy.Unstructured,
		Metadata: map[string]any{
			gwcommon.PolicyGK.String(): policy,
		},
	}
}

func encodePolicyCRDAsNode(policyCRD *policymanager.PolicyCRD) (*gwctltopology.Node, error) {
	o, err := runtime.DefaultUnstructuredConverter.ToUnstructured(policyCRD.CRD)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: o}

	return &gwctltopology.Node{
		Object: u,
		Metadata: map[string]any{
			gwcommon.PolicyCRDGK.String(): policyCRD,
		},
	}, nil
}
