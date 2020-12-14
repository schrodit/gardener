// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package gardenlet

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	v2 "github.com/gardener/component-spec/bindings-go/apis/v2"
	"github.com/gardener/component-spec/bindings-go/codec"
	landscaperv1alpha1 "github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	landscaperconstants "github.com/gardener/landscaper/pkg/apis/deployer/container"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/apis/imports"
	"github.com/gardener/gardener/pkg/logger"
)

const (
	gardenerComponentName = "github.com/gardener/gardener"
	gardenletImageName    = "gardenlet"
)

// Landscaper has all the context and parameters needed to run a Gardenlet landscaper.
type Landscaper struct {
	log                      *logrus.Entry
	gardenClient             kubernetes.Interface
	seedClient               kubernetes.Interface
	Imports                  *imports.Imports
	landscaperOperation      string
	gardenletImageRepository string
	gardenletImageTag        string
}

// NewGardenletLandscaper creates a new Gardenlet landscaper.
func NewGardenletLandscaper(imports *imports.Imports, landscaperOperation, componentDescriptorPath string) (*Landscaper, error) {
	landscaper := &Landscaper{
		log:                 logger.NewFieldLogger(logger.NewLogger("info"), "landscaper-gardenlet operation", landscaperOperation),
		Imports:             imports,
		landscaperOperation: landscaperOperation,
	}

	componentDescriptorData, err := ioutil.ReadFile(componentDescriptorPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the Gardenlet component descriptor: %v", err)
	}

	componentDescriptorList := &v2.ComponentDescriptorList{}
	err = codec.Decode(componentDescriptorData, componentDescriptorList)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the Gardenlet component descriptor: %v", err)
	}

	err = landscaper.parseGardenletImage(componentDescriptorList)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the component descriptor: %v", err)
	}

	gardenTargetConfig := &landscaperv1alpha1.KubernetesClusterTargetConfig{}
	if err := json.Unmarshal(imports.GardenCluster.Spec.Configuration, gardenTargetConfig); err != nil {
		return nil, fmt.Errorf("failed to parse the Garden cluster kubeconfig : %v", err)
	}

	// Create Garden client
	gardenClient, err := kubernetes.NewClientFromBytes([]byte(gardenTargetConfig.Kubeconfig), kubernetes.WithClientOptions(
		client.Options{
			Scheme: kubernetes.GardenScheme,
		}))
	if err != nil {
		return nil, fmt.Errorf("failed to create the Garden cluster client: %v", err)
	}

	landscaper.gardenClient = gardenClient

	runtimeClusterTargetConfig := &landscaperv1alpha1.KubernetesClusterTargetConfig{}
	if err := json.Unmarshal(imports.RuntimeCluster.Spec.Configuration, runtimeClusterTargetConfig); err != nil {
		return nil, fmt.Errorf("failed to parse the Runtime cluster kubeconfig : %v", err)
	}

	// Create Seed client
	seedClient, err := kubernetes.NewClientFromBytes([]byte(runtimeClusterTargetConfig.Kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create the runtime cluster client: %v", err)
	}

	landscaper.seedClient = seedClient

	return landscaper, nil
}

func (g Landscaper) Run(ctx context.Context) error {
	switch g.landscaperOperation {
	case string(landscaperconstants.OperationReconcile):
		return g.Reconcile(ctx)
	case string(landscaperconstants.OperationDelete):
		return g.Delete(ctx)
	default:
		return fmt.Errorf(fmt.Sprintf("environment variable \"OPERATION\" must either be set to %q or %q", landscaperconstants.OperationReconcile, landscaperconstants.OperationDelete))
	}
}

// parseGardenletImage gets the Gardenlet image from the component descriptor
// The component descriptor is the only image source and must be provided
func (g *Landscaper) parseGardenletImage(componentDescriptorList *v2.ComponentDescriptorList) error {
	// The function returns a list as there could be multiple components with the same name but different version
	components := componentDescriptorList.GetComponentByName(gardenerComponentName)
	if len(components) != 1 {
		return fmt.Errorf(fmt.Sprintf("expecting exactly one component with name %q", gardenerComponentName))
	}

	// get gardenlet image from component descriptor
	res, err := components[0].GetResourcesByName(gardenletImageName)
	if err != nil {
		return fmt.Errorf("failed to get OCI image for gardenlet from component descriptor: %v", err)
	}
	if len(res) == 0 {
		return fmt.Errorf("OCI image for gardenlet not found in component descriptor")
	}

	gardenletResource := res[0]
	imageVersion := gardenletResource.GetVersion()
	if len(imageVersion) == 0 {
		return fmt.Errorf("OCI image version for gardenlet not found in component descriptor")
	}

	access := gardenletResource.Access
	if access == nil {
		return fmt.Errorf("cannot get OCI image reference for gardenlet from component descriptor: %v", err)
	}

	ociRegistryObjectAccessor := &v2.OCIRegistryAccess{}
	if err := ociRegistryObjectAccessor.SetData(access.Raw); err != nil {
		return fmt.Errorf("OCI image reference for gardenlet not found in component descriptor: %v", err)
	}

	imageReference := ociRegistryObjectAccessor.ImageReference
	if len(imageReference) == 0 {
		return fmt.Errorf("OCI image reference for gardenlet not found in component descriptor")
	}

	// split version from reference e.g eu.gcr.io/gardener-project/gardener/gardenlet:v1.11.3
	split := strings.Split(imageReference, ":")
	if len(split) == 2 {
		g.gardenletImageRepository = split[0]
		g.gardenletImageTag = split[1]
		return nil
	}

	if len(split) == 3 {
		g.gardenletImageRepository = fmt.Sprintf("%s:%s", split[0], split[1])
		g.gardenletImageTag = split[2]
		return nil
	}

	return fmt.Errorf("unable to determine the Gardenlet image repository and tag from the provided image reference %q in the component descriptor", imageReference)
}
