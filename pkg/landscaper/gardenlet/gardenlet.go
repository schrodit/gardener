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

package gardenlet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	v2 "github.com/gardener/component-spec/bindings-go/apis/v2"
	"github.com/gardener/component-spec/bindings-go/codec"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/apis/imports"
	"github.com/gardener/gardener/pkg/logger"
)

const (
	landscaperReconciliation = "RECONCILE"
	landscaperDeletion       = "DELETE"
	gardenerComponentName    = "github.com/gardener/gardener"
	gardenletImageName       = "gardenlet"
)

// Landscaper has all the context and parameters needed to run a Gardenlet landscaper.
type Landscaper struct {
	log                            *logrus.Entry
	gardenClient                   kubernetes.Interface
	seedClient                     kubernetes.Interface
	Imports                        *imports.LandscaperGardenletImport
	landscaperOperation            string
	imageVectorOverride            *string
	componentImageVectorOverwrites *string
	gardenletImageRepository       string
	gardenletImageTag              string
}

// NewGardenletLandscaper creates a new Gardenlet landscaper.
func NewGardenletLandscaper(imports *imports.LandscaperGardenletImport, landscaperOperation, componentDescriptorPath string) (*Landscaper, error) {
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

	err = landscaper.parseImageVectorOverride()
	if err != nil {
		return nil, fmt.Errorf("failed to parse Gardenlet landscaper imports: %v", err)
	}

	// Create Garden client
	gardenClient, err := kubernetes.NewClientFromBytes([]byte(imports.GardenCluster.Spec.Configuration.Kubeconfig), kubernetes.WithClientOptions(
		client.Options{
			Scheme: kubernetes.GardenScheme,
		}))
	if err != nil {
		return nil, fmt.Errorf("failed to create the Garden cluster client: %v", err)
	}

	landscaper.gardenClient = gardenClient

	// Create Seed client
	seedClient, err := kubernetes.NewClientFromBytes([]byte(imports.RuntimeCluster.Spec.Configuration.Kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create the runtime cluster client: %v", err)
	}

	landscaper.seedClient = seedClient

	return landscaper, nil
}

func (g Landscaper) Run(ctx context.Context) error {
	switch g.landscaperOperation {
	case landscaperReconciliation:
		return g.Reconcile(ctx)
	case landscaperDeletion:
		return g.Delete(ctx)
	default:
		return fmt.Errorf(fmt.Sprintf("environment variable \"OPERATION\" must either be set to %q or %q", landscaperReconciliation, landscaperDeletion))
	}
}

// parseImageVectorOverride parses the image vectors as a string from the import values
// the image vectors are already properly created by the landscaper and are only handed
// over as-is to the Gardenlet helm chart
func (g *Landscaper) parseImageVectorOverride() error {
	if g.Imports.ImageVectorOverwrite != nil {
		var imageVectorOverwrite string
		if err := json.Unmarshal(g.Imports.ImageVectorOverwrite.Raw, &imageVectorOverwrite); err != nil {
			return err
		}
		g.imageVectorOverride = &imageVectorOverwrite
	}

	if g.Imports.ComponentImageVectorOverwrites != nil {
		var componentImageVectorOverwrites string
		if err := json.Unmarshal(g.Imports.ComponentImageVectorOverwrites.Raw, &componentImageVectorOverwrites); err != nil {
			return err
		}
		g.componentImageVectorOverwrites = &componentImageVectorOverwrites
	}
	return nil
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
	res := components[0].GetLocalResourcesByName(v2.OCIImageType, gardenletImageName)
	if len(res) == 0 {
		return fmt.Errorf("OCI image for gardenlet not found in component descriptor")
	}
	gardenletResource := res[0]
	imageVersion := gardenletResource.GetVersion()
	if len(imageVersion) == 0 {
		return fmt.Errorf("OCI image version for gardenlet not found in component descriptor")
	}
	imageReference := gardenletResource.Access.(*v2.OCIRegistryAccess).ImageReference
	if len(imageReference) == 0 {
		return fmt.Errorf("OCI image reference for gardenlet not found in component descriptor")
	}

	// split version from reference e.g eu.gcr.io/gardener-project/gardener/gardenlet:v1.11.3
	split := strings.Split(imageReference, ":")
	if len(split) != 2 {
		return fmt.Errorf("OCI image repository for gardenlet not found in component descriptor")
	}
	g.gardenletImageRepository = split[0]
	g.gardenletImageTag = split[1]
	return nil
}
