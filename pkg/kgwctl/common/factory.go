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

This file is a fork of sigs.k8s.io/gwctl/pkg/common/factory.go
It is modified to expose the RESTConfig for use in clients other than the builder.
*/

package common

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest"
)

type Factory interface {
	NewBuilder() *resource.Builder
	KubeConfigNamespace() (string, bool, error)
	ToRESTConfig() (*rest.Config, error)
}

type factoryImpl struct {
	clientGetter genericclioptions.RESTClientGetter
}

func NewFactory(clientGetter genericclioptions.RESTClientGetter) Factory {
	return &factoryImpl{clientGetter: clientGetter}
}

func (f *factoryImpl) NewBuilder() *resource.Builder {
	return resource.NewBuilder(f.clientGetter)
}

func (f *factoryImpl) ToRESTConfig() (*rest.Config, error) {
	return f.clientGetter.ToRESTConfig()
}

func (f *factoryImpl) KubeConfigNamespace() (string, bool, error) {
	return f.clientGetter.ToRawKubeConfigLoader().Namespace()
}
