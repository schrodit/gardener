// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package framework

import (
	"fmt"
	"github.com/gardener/gardener/pkg/apis/core/v1alpha1/helper"
	"github.com/ghodss/yaml"
	"io/ioutil"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	v1alpha1constants "github.com/gardener/gardener/pkg/apis/core/v1alpha1/constants"

	"github.com/gardener/gardener/pkg/utils"

	apimachineryRuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	gardencorev1alpha1 "github.com/gardener/gardener/pkg/apis/core/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
)

var (
	decoder = serializer.NewCodecFactory(kubernetes.GardenScheme).UniversalDecoder()

	// TemplateDir relative path for helm templates dir
	TemplateDir = filepath.Join("..", "..", "resources", "templates")
)

const (

	// IntegrationTestPrefix is the default prefix that will be used for test shoots if none other is specified
	IntegrationTestPrefix = "itest-"
	WorkerNamePrefix      = "worker-"
)

// CreateShootTestArtifacts creates a shoot object from the given path and sets common attributes (test-individual settings like workers have to be handled by each test).
func CreateShootTestArtifacts(shootTestYamlPath string, prefix *string, projectNamespace, shootRegion, cloudProfile, secretbinding, providerType, k8sVersion, externalDomain *string, clearDNS bool, clearExtensions bool) (string, *gardencorev1alpha1.Shoot, error) {
	shoot := &gardencorev1alpha1.Shoot{}
	if err := ReadObject(shootTestYamlPath, shoot); err != nil {
		return "", nil, err
	}

	if shootRegion != nil && len(*shootRegion) > 0 {
		shoot.Spec.Region = *shootRegion
	}

	if externalDomain != nil && len(*externalDomain) > 0 {
		shoot.Spec.DNS = &gardencorev1alpha1.DNS{Domain: externalDomain}
		clearDNS = false
	}

	if projectNamespace != nil && len(*projectNamespace) > 0 {
		shoot.Namespace = *projectNamespace
	}

	if prefix != nil && len(*prefix) != 0 {
		integrationTestName, err := generateRandomShootName(*prefix, 8)
		if err != nil {
			return "", nil, err
		}
		shoot.Name = integrationTestName
	}

	if cloudProfile != nil && len(*cloudProfile) > 0 {
		shoot.Spec.CloudProfileName = *cloudProfile
	}

	if secretbinding != nil && len(*secretbinding) > 0 {
		shoot.Spec.SecretBindingName = *secretbinding
	}

	if providerType != nil && len(*providerType) > 0 {
		shoot.Spec.Provider.Type = *providerType
	}

	if k8sVersion != nil && len(*k8sVersion) > 0 {
		shoot.Spec.Kubernetes.Version = *k8sVersion
	}

	if clearDNS {
		shoot.Spec.DNS = &gardencorev1alpha1.DNS{}
	}

	if clearExtensions {
		shoot.Spec.Extensions = nil
	}

	if shoot.Annotations == nil {
		shoot.Annotations = map[string]string{}
	}
	shoot.Annotations[v1alpha1constants.AnnotationShootIgnoreAlerts] = "true"

	return shoot.Name, shoot, nil
}

// SetProviderConfigsFromFilepath parses the infrastructure, controlPlane and networking provider-configs and sets them on the shoot
func SetProviderConfigsFromFilepath(shoot *gardencorev1alpha1.Shoot, infrastructure, controlPlane, networking *string) error {
	// clear provider configs first
	shoot.Spec.Provider.InfrastructureConfig = nil
	shoot.Spec.Provider.ControlPlaneConfig = nil
	shoot.Spec.Networking.ProviderConfig = nil

	if infrastructure != nil && len(*infrastructure) != 0 {
		infrastructureProviderConfig, err := ParseFileAsProviderConfig(*infrastructure)
		if err != nil {
			return err
		}
		shoot.Spec.Provider.InfrastructureConfig = infrastructureProviderConfig
	}

	if len(*controlPlane) != 0 {
		controlPlaneProviderConfig, err := ParseFileAsProviderConfig(*controlPlane)
		if err != nil {
			return err
		}
		shoot.Spec.Provider.ControlPlaneConfig = controlPlaneProviderConfig
	}

	if len(*networking) != 0 {
		networkingProviderConfig, err := ParseFileAsProviderConfig(*networking)
		if err != nil {
			return err
		}
		shoot.Spec.Networking.ProviderConfig = networkingProviderConfig
	}

	return nil
}

func generateRandomShootName(prefix string, length int) (string, error) {
	randomString, err := utils.GenerateRandomString(length)
	if err != nil {
		return "", err
	}

	if len(prefix) > 0 {
		return prefix + strings.ToLower(randomString), nil
	}

	return IntegrationTestPrefix + strings.ToLower(randomString), nil
}

// CreatePlantTestArtifacts creates a plant object which is read from the resources directory
func CreatePlantTestArtifacts(plantTestYamlPath string) (*gardencorev1alpha1.Plant, error) {

	plant := &gardencorev1alpha1.Plant{}
	if err := ReadObject(plantTestYamlPath, plant); err != nil {
		return nil, err
	}

	return plant, nil
}

// ReadObject loads the contents of file and decodes it as an object.
func ReadObject(file string, into apimachineryRuntime.Object) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	_, _, err = decoder.Decode(data, nil, into)
	return err
}

// ParseFileAsProviderConfig parses a file as a ProviderConfig
func ParseFileAsProviderConfig(filepath string) (*gardencorev1alpha1.ProviderConfig, error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	// apiServer needs JSON for the Raw data
	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("unable to decode ProviderConfig: %v", err)
	}
	return &gardencorev1alpha1.ProviderConfig{RawExtension: apimachineryRuntime.RawExtension{Raw: jsonData}}, nil
}

// GetProjectRootPath gets the root path of the project relative to the integration test folder
func GetProjectRootPath() string {
	_, filename, _, _ := runtime.Caller(0)
	dir := path.Join(path.Dir(filename), "../../..")
	return dir
}

// AddWorkerForName adds a valid worker to the shoot for the given machine image name. Returns an error if the machine image cannot be found in the CloudProfile.
func AddWorkerForName(shoot *gardencorev1alpha1.Shoot, cloudProfile *gardencorev1alpha1.CloudProfile, machineImageName *string, workerZone *string) error {
	found, image, err := helper.DetermineMachineImageForName(cloudProfile, *machineImageName)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("could not find machine image '%s' in CloudProfile '%s'", *machineImageName, cloudProfile.Name)
	}

	return AddWorker(shoot, cloudProfile, image, workerZone)
}

// AddWorker adds a valid default worker to the shoot for the given machineImage and CloudProfile.
func AddWorker(shoot *gardencorev1alpha1.Shoot, cloudProfile *gardencorev1alpha1.CloudProfile, machineImage gardencorev1alpha1.MachineImage, workerZone *string) error {
	_, shootMachineImage, err := helper.GetShootMachineImageFromLatestMachineImageVersion(machineImage)
	if err != nil {
		return err
	}

	if len(cloudProfile.Spec.VolumeTypes) == 0 {
		return fmt.Errorf("no VolumeTypes configured in the Cloudprofile '%s'", cloudProfile.Name)
	}

	if len(cloudProfile.Spec.MachineTypes) == 0 {
		return fmt.Errorf("no MachineTypes configured in the Cloudprofile '%s'", cloudProfile.Name)
	}

	workerName, err := generateRandomWorkerName(fmt.Sprintf("%s-", shootMachineImage.Name))
	if err != nil {
		return err
	}

	shoot.Spec.Provider.Workers = append(shoot.Spec.Provider.Workers, gardencorev1alpha1.Worker{
		Name:    workerName,
		Maximum: 3,
		Minimum: 1,
		Machine: gardencorev1alpha1.Machine{
			Type:  cloudProfile.Spec.MachineTypes[0].Name,
			Image: &shootMachineImage,
		},
		Volume: &gardencorev1alpha1.Volume{
			Type: cloudProfile.Spec.VolumeTypes[0].Name,
			Size: "35Gi",
		},
	})

	if workerZone != nil && len(*workerZone) > 0 {
		// using one zone as default
		shoot.Spec.Provider.Workers[0].Zones = []string{*workerZone}
	}

	return nil
}

func generateRandomWorkerName(prefix string) (string, error) {
	var length int
	remainingCharacters := 15 - len(prefix)
	if remainingCharacters > 0 {
		length = remainingCharacters
	} else {
		prefix = WorkerNamePrefix
		length = 15 - len(WorkerNamePrefix)
	}

	randomString, err := utils.GenerateRandomString(length)
	if err != nil {
		return "", err
	}

	return prefix + strings.ToLower(randomString), nil
}
