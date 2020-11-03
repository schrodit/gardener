// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package applier

import (
	"context"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/operation/botanist/component"
)

type vpa struct {
	kubernetes.ChartApplier
	chartPath  string
	seedClient client.Client
}

// NewVPACRDApplier can be used to deploy the VPA CRD.
// Destroy does nothing.
func NewVPACRDApplier(
	applier kubernetes.ChartApplier,
	client client.Client,
	chartsRootPath string,
) (component.Deployer, error) {
	return &vpa{
		ChartApplier: applier,
		// TODO make sure to include dir in Dockerfile
		chartPath:  filepath.Join(chartsRootPath, "landscaper", "gardenlet", "vpa-crd"),
		seedClient: client,
	}, nil
}

func (c *vpa) Deploy(ctx context.Context) error {
	return c.Apply(ctx, c.chartPath, "", "vpa-crd")
}

func (c *vpa) Destroy(ctx context.Context) error {
	// do not remove CRD as we do not know if the CRD has existed before the Gardenlet was deployed
	return nil
}