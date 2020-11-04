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

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/operation/botanist/component"
	gardenletoperation "github.com/gardener/gardener/pkg/operation/seed/gardenlet"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type gardenlet struct {
	kubernetes.ChartApplier
	chartPath string
	client    client.Client
	values    map[string]interface{}
}

// NewGardenletChartApplier can be used to deploy the Gardenlet chart.
func NewGardenletChartApplier(
	applier kubernetes.ChartApplier,
	client client.Client,
	values map[string]interface{},
	chartsRootPath string,
) (component.Deployer, error) {
	return &gardenlet{
		ChartApplier: applier,
		// TODO: make sure to include dir in Dockerfile
		chartPath: filepath.Join(chartsRootPath, "gardener", "gardenlet"),
		client:    client,
		values:    values,
	}, nil
}

func (c *gardenlet) Deploy(ctx context.Context) error {
	return c.Apply(ctx, c.chartPath, v1beta1constants.GardenNamespace, "gardenlet", kubernetes.Values(c.values))
}

// TODO create unit test
func (c *gardenlet) Destroy(ctx context.Context) error {
	return gardenletoperation.DeleteGardenlet(ctx, c.client)
}
