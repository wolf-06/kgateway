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

package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"

	gwcommon "sigs.k8s.io/gwctl/pkg/common"

	cmdanalyze "github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/cmd/analyze"
	cmdcheck "github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/cmd/check"
	cmdget "github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/cmd/get"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/common"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/config"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/constants"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/extension"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/extension/directlyattachedpolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/extension/gatewayeffectivepolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/extension/notfoundrefvalidator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/extension/refgrantvalidator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/policymanager"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/topology"
	topologykgw "github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/topology/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/version"
)

var RootCmd = &cobra.Command{
	Use:     "kgwctl",
	Version: version.GetVersionInfo(),
	Short:   "Commands for interacting with kgateway installations.",
}

// Execute executes the root command.
func Execute() error {
	globalConfig := genericclioptions.NewConfigFlags(true)
	globalConfig.AddFlags(RootCmd.PersistentFlags())

	// Initialize flags for klog.
	//
	// These are not directly added to the rootCmd since we ony want to expose the
	// verbosity (-v) flag and not the rest. To achieve that, we'll define a
	// separate verbosity flag whose value we'll propagate to the klogFlags.
	var verbosity int
	RootCmd.PersistentFlags().IntVarP(&verbosity, "v", "v", 0, "number for the log level verbosity (defaults to 0)")
	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)

	cobra.OnInitialize(func() {
		if err := klogFlags.Set("v", fmt.Sprintf("%v", verbosity)); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to configure verbosity for logging")
		}
	})

	ioStreams := genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	cliConfig := &config.CliConfig{
		Relations:                  topologykgw.AllRelations,
		ControllerName:             constants.KgatewayControllerName,
		ControllerDefaultNamespace: constants.KgatewayDefaultControllerNamespace,
		ControllerDeploymentName:   constants.KgatewayControllerDeploymentName,
		GatewayName:                "kgateway",
		ExecuteExtensions: func(graph *topology.Graph, factory gwcommon.Factory, sources []*unstructured.Unstructured) error {
			policyManager := policymanager.New(gwcommon.NewDefaultGroupKindFetcher(factory, gwcommon.WithAdditionalResources(sources)))
			if err := policyManager.Init(); err != nil { //nolint:govet
				return err
			}
			return extension.ExecuteAll(graph,
				directlyattachedpolicy.NewExtension(policyManager),
				gatewayeffectivepolicy.NewExtension(),
				refgrantvalidator.NewExtension(
					refgrantvalidator.NewDefaultReferenceGrantFetcher(factory, refgrantvalidator.WithAdditionalResources(sources)),
				),
				notfoundrefvalidator.NewExtension(),
			)
		},
	}

	factory := common.NewFactory(globalConfig)
	RootCmd.AddCommand(cmdget.NewCmd(factory, ioStreams, false, cliConfig))
	RootCmd.AddCommand(cmdget.NewCmd(factory, ioStreams, true, cliConfig))
	RootCmd.AddCommand(cmdanalyze.NewCmd(factory, ioStreams, cliConfig))
	RootCmd.AddCommand(cmdcheck.NewCmd(factory, ioStreams, cliConfig))

	return RootCmd.Execute()
}
