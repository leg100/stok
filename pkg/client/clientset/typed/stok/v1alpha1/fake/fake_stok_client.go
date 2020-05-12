// Copyright © 2020 Louis Garman <louisgarman@gmail.com>
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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/leg100/stok/pkg/client/clientset/typed/stok/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeStokV1alpha1 struct {
	*testing.Fake
}

func (c *FakeStokV1alpha1) Applies(namespace string) v1alpha1.ApplyInterface {
	return &FakeApplies{c, namespace}
}

func (c *FakeStokV1alpha1) Destroys(namespace string) v1alpha1.DestroyInterface {
	return &FakeDestroys{c, namespace}
}

func (c *FakeStokV1alpha1) ForceUnlocks(namespace string) v1alpha1.ForceUnlockInterface {
	return &FakeForceUnlocks{c, namespace}
}

func (c *FakeStokV1alpha1) Gets(namespace string) v1alpha1.GetInterface {
	return &FakeGets{c, namespace}
}

func (c *FakeStokV1alpha1) Imps(namespace string) v1alpha1.ImpInterface {
	return &FakeImps{c, namespace}
}

func (c *FakeStokV1alpha1) Inits(namespace string) v1alpha1.InitInterface {
	return &FakeInits{c, namespace}
}

func (c *FakeStokV1alpha1) Outputs(namespace string) v1alpha1.OutputInterface {
	return &FakeOutputs{c, namespace}
}

func (c *FakeStokV1alpha1) Plans(namespace string) v1alpha1.PlanInterface {
	return &FakePlans{c, namespace}
}

func (c *FakeStokV1alpha1) Refreshes(namespace string) v1alpha1.RefreshInterface {
	return &FakeRefreshes{c, namespace}
}

func (c *FakeStokV1alpha1) Shells(namespace string) v1alpha1.ShellInterface {
	return &FakeShells{c, namespace}
}

func (c *FakeStokV1alpha1) Shows(namespace string) v1alpha1.ShowInterface {
	return &FakeShows{c, namespace}
}

func (c *FakeStokV1alpha1) States(namespace string) v1alpha1.StateInterface {
	return &FakeStates{c, namespace}
}

func (c *FakeStokV1alpha1) Taints(namespace string) v1alpha1.TaintInterface {
	return &FakeTaints{c, namespace}
}

func (c *FakeStokV1alpha1) Untaints(namespace string) v1alpha1.UntaintInterface {
	return &FakeUntaints{c, namespace}
}

func (c *FakeStokV1alpha1) Validates(namespace string) v1alpha1.ValidateInterface {
	return &FakeValidates{c, namespace}
}

func (c *FakeStokV1alpha1) Workspaces(namespace string) v1alpha1.WorkspaceInterface {
	return &FakeWorkspaces{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeStokV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
