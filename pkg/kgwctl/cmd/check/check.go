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

package check

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1client "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gwcommon "sigs.k8s.io/gwctl/pkg/common"
	gwctltopology "sigs.k8s.io/gwctl/pkg/topology"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/common"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/config"
	extensionutils "github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/extension/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/topology"
	topologykgw "github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/topology/kgateway"
)

func NewCmd(factory common.Factory, iostreams genericiooptions.IOStreams, cliConfig *config.CliConfig) *cobra.Command {
	flags := newCheckFlags()

	cmd := &cobra.Command{
		Use:   "check",
		Short: fmt.Sprintf("Check the health of the %s installation.", cliConfig.GatewayName),
		Run: func(_ *cobra.Command, args []string) {
			o, err := flags.ToOptions(args, factory, iostreams, cliConfig)
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

	return cmd
}

// checkFlags contains the flags used with check command.
type checkFlags struct {
	resourceBuilderFlags *genericclioptions.ResourceBuilderFlags
}

func newCheckFlags() *checkFlags {
	resourceBuilderFlags := genericclioptions.NewResourceBuilderFlags().
		WithAllNamespaces(false).
		WithLabelSelector("")
	return &checkFlags{
		resourceBuilderFlags: resourceBuilderFlags,
	}
}

func (f *checkFlags) ToOptions(args []string, factory common.Factory, iostreams genericiooptions.IOStreams, cliConfig *config.CliConfig) (*checkOptions, error) {
	o := &checkOptions{
		factory:                  factory,
		IOStreams:                iostreams,
		labelSelector:            *f.resourceBuilderFlags.LabelSelector,
		relations:                cliConfig.Relations,
		gatewayName:              cliConfig.GatewayName,
		controllerName:           cliConfig.ControllerName,
		controllerDeploymentName: cliConfig.ControllerDeploymentName,
		executeExtensions:        cliConfig.ExecuteExtensions,
	}

	var err error
	var overriden bool
	o.namespace, overriden, err = factory.KubeConfigNamespace()
	if err != nil {
		return nil, err
	}

	if !overriden {
		o.namespace = cliConfig.ControllerDefaultNamespace
	}

	restConfig, err := factory.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	gatewayClient, err := apiv1client.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	o.gatewayClient = gatewayClient

	k8sClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	o.k8sClient = k8sClient

	return o, nil
}

type checkOptions struct {
	factory       common.Factory
	gatewayClient *apiv1client.Clientset
	k8sClient     *kubernetes.Clientset
	relations     []*gwctltopology.Relation

	namespace                string
	labelSelector            string
	gatewayName              string
	controllerName           string
	controllerDeploymentName string

	executeExtensions func(graph *topology.Graph, factory gwcommon.Factory, sources []*unstructured.Unstructured) error

	genericclioptions.IOStreams
}

// TODO Add a check for unbound resources with no status.
func (o *checkOptions) Run(args []string) error {
	// Get all Gatewaylasses in the cluster.
	allGatewayClasses, err := o.gatewayClient.GatewayV1().GatewayClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(o.IOStreams.ErrOut, "Error getting GatewayClass: %s\n", err)
		return err
	}

	if len(allGatewayClasses.Items) == 0 {
		fmt.Fprintf(o.IOStreams.ErrOut, "No GatewayClasses found for %s \n", o.controllerName)
		return fmt.Errorf("No GatewayClasses found for %s \n", o.controllerName)
	}

	// Filter to just kgateway GatewayClasses.
	var kgatewayClasses []apiv1.GatewayClass
	for _, kgatewayClass := range allGatewayClasses.Items {
		if string(kgatewayClass.Spec.ControllerName) == o.controllerName {
			kgatewayClasses = append(kgatewayClasses, kgatewayClass)
		}
	}

	checkResult := &checkResult{
		out:            o.IOStreams.Out,
		controllerName: o.controllerName,
	}

	// Check the health of the kgateway controller deployment.
	checkResult.controllerResults, err = o.checkController(o.namespace, o.controllerDeploymentName)
	if err != nil {
		return err
	}

	// Check the health of the all the relevant gateways by GatewayClass.
	checkResult.gatewayClassResults, err = o.checkGatewaysForGatewayClasses(kgatewayClasses)
	if err != nil {
		return err
	}

	checkResult.PrettyPrint()

	return nil
}

// checkController checks the health of the kgateway controller deployment.
// First return value is the error found in the controller.
// Second return value is the error encountered if we couldn't access the controller information.
func (o *checkOptions) checkController(namespace, controllerDeploymentName string) (*controllerCheckResult, error) {
	result := &controllerCheckResult{}

	deployment, err := o.k8sClient.AppsV1().Deployments(namespace).Get(context.Background(), controllerDeploymentName, metav1.GetOptions{})
	if err != nil {
		return result, err
	}

	if deployment == nil {
		result.errorMsg = fmt.Sprintf("Deployment %s/%s not found", namespace, controllerDeploymentName)
		return result, nil
	}

	result.deployment = deployment

	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			// If the Status is good then we have no errors to report.
			if condition.Status == corev1.ConditionTrue {
				// If the Status is good then we have no errors to report. Clear the error message if we accidentally set it.
				result.errorMsg = ""
				break
			}
		}
		if condition.Type == appsv1.DeploymentProgressing {
			if condition.Status == corev1.ConditionTrue {
				result.errorMsg = fmt.Sprintf("Deployment %s found to be progressing\n", deployment.GetName())
				break
			}
		}
		if condition.Type == appsv1.DeploymentReplicaFailure {
			if condition.Status == corev1.ConditionTrue {
				result.errorMsg = fmt.Sprintf("Deployment %s has failed to become available: %s\n", deployment.GetName(), condition.Message)
				break
			}
		}
	}

	return result, nil
}

func (o *checkOptions) checkGatewaysForGatewayClasses(gatewayClasses []apiv1.GatewayClass) ([]*gatewayClassCheckResult, error) {
	var results []*gatewayClassCheckResult

	// Get all Gateways in the cluster across all namespaces.
	allGateways := []*apiv1.Gateway{}

	// First, get all namespaces
	namespaces, err := o.k8sClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return results, err
	}

	// Collect gateways from all namespaces
	for _, namespaceInfo := range namespaces.Items {
		namespace := namespaceInfo.Name
		gateways, err := o.gatewayClient.GatewayV1().Gateways(namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			// Skip namespaces where we can't list gateways (e.g., system namespaces)
			continue
		}

		for i := range gateways.Items {
			allGateways = append(allGateways, &gateways.Items[i])
		}
	}

	fmt.Fprintf(o.IOStreams.Out, "Found %d Gateways across all namespaces\n", len(allGateways))

	// group gateways by gateway class but have an entry for each gateway class even if it has no gateways
	gatewaysByGatewayClass := make(map[string][]*apiv1.Gateway)
	for _, gatewayClass := range gatewayClasses {
		gatewaysByGatewayClass[gatewayClass.GetName()] = []*apiv1.Gateway{}
	}

	for _, gateway := range allGateways {
		gatewayClassName := string(gateway.Spec.GatewayClassName)
		if _, ok := gatewaysByGatewayClass[gatewayClassName]; ok {
			gatewaysByGatewayClass[gatewayClassName] = append(gatewaysByGatewayClass[gatewayClassName], gateway)
		}
	}

	for _, gatewayClass := range gatewayClasses {
		gatewayCheckResults, err := o.checkGateways(gatewaysByGatewayClass[gatewayClass.GetName()])
		if err != nil {
			return results, err
		}
		results = append(results, &gatewayClassCheckResult{
			gatewayClass: &gatewayClass,
			gateways:     gatewayCheckResults,
		})
	}

	return results, nil
}

// checkGateways checks the health of all kgateway gateways associated with a given GatewayClass.
func (o *checkOptions) checkGateways(gateways []*apiv1.Gateway) ([]*gatewayCheckResult, error) {
	results := []*gatewayCheckResult{}
	if len(gateways) == 0 {
		return results, nil
	}

	for _, gateway := range gateways {
		result, err := o.checkGateway(gateway)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (o *checkOptions) checkGateway(gateway *apiv1.Gateway) (*gatewayCheckResult, error) {
	result := &gatewayCheckResult{
		gateway: gateway,
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(gateway)
	if err != nil {
		return result, err
	}
	sources := []*unstructured.Unstructured{{Object: obj}}

	graph, err := topology.NewBuilder(gwcommon.NewDefaultGroupKindFetcher(o.factory, gwcommon.WithAdditionalResources(sources))).
		StartFrom(sources).
		UseRelationships(topologykgw.AllRelations).
		WithMaxDepth(4).
		Build()
	if err != nil {
		return result, err
	}

	err = o.executeExtensions(graph, o.factory, sources)
	if err != nil {
		return result, err
	}

	errors, err := collectErrors(graph)
	if err != nil {
		return result, err
	}

	for err := range errors {
		result.errors = append(result.errors, err)
	}

	return result, nil
}

func collectErrors(graph *topology.Graph) (map[string]bool, error) {
	errors := map[string]bool{}
	for i := range graph.Nodes {
		for j := range graph.Nodes[i] {
			node := graph.Nodes[i][j]
			aggregateAnalysisErrors, err := extensionutils.AggregateAnalysisErrors(node)
			if err != nil {
				return nil, err
			}
			for _, err := range aggregateAnalysisErrors {
				s := fmt.Sprintf("%v: %v", node.GKNN(), err)
				errors[s] = true
			}
		}
	}
	return errors, nil
}

type checkResult struct {
	out                 io.Writer
	controllerName      string
	controllerResults   *controllerCheckResult
	gatewayClassResults []*gatewayClassCheckResult
}

type controllerCheckResult struct {
	deployment *appsv1.Deployment
	errorMsg   string
}

type gatewayClassCheckResult struct {
	gatewayClass *apiv1.GatewayClass
	gateways     []*gatewayCheckResult
}

type gatewayCheckResult struct {
	gateway *apiv1.Gateway
	errors  []string
}

func (r *checkResult) PrettyPrint() {
	fmt.Fprintf(r.out, "Controller: %s\n", r.controllerName)

	if r.controllerResults == nil {
		fmt.Fprintf(r.out, " - No Controller found.\n")
		return
	}
	fmt.Fprintf(r.out, "- deployment: %s | ", r.controllerResults.deployment.GetName())
	if r.controllerResults.errorMsg == "" {
		fmt.Fprintf(r.out, "%s\n", color.GreenString("healthy"))
	} else {
		fmt.Fprintf(r.out, "%s\n", color.RedString("not healthy"))
		fmt.Fprintf(r.out, "  - %s\n", r.controllerResults.errorMsg)
	}

	fmt.Fprintf(r.out, "\nGatewayClasses:")
	for _, gatewayClassResult := range r.gatewayClassResults {
		fmt.Fprintf(r.out, "\n")
		fmt.Fprintf(r.out, "- name: %s\n", gatewayClassResult.gatewayClass.GetName())
		fmt.Fprintf(r.out, "  Gateways:\n")
		if len(gatewayClassResult.gateways) == 0 {
			fmt.Fprintf(r.out, "  - %s.\n", color.CyanString("None found"))
			continue
		}
		for _, gatewayResult := range gatewayClassResult.gateways {
			status := fmt.Sprintf("%v issues found", len(gatewayResult.errors))
			if len(gatewayResult.errors) == 0 {
				status = color.GreenString(status)
			} else {
				status = color.RedString(status)
			}
			fmt.Fprintf(r.out, "  - %s/%s - %s\n", gatewayResult.gateway.GetNamespace(), gatewayResult.gateway.GetName(), status)
			for _, err := range gatewayResult.errors {
				fmt.Fprintf(r.out, "    - %s\n", err)
			}
		}
	}
}
