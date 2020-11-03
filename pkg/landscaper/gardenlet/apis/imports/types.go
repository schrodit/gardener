// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package imports

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	gardenletconfigv1alpha1 "github.com/gardener/gardener/pkg/gardenlet/apis/config/v1alpha1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GardenletLandscaperImport defines the landscaper import for the Gardenlet.
// structure defined in Blueprint
type GardenletLandscaperImport struct {
	metav1.TypeMeta
	// RuntimeCluster is the landscaper target containing the kubeconfig for the cluster
	// where the Gardenlet should be deployed.
	// This is the Kubernetes cluster targeted as Seed (via in-cluster mounted service account token),
	// if not otherwise specified in `.componentConfiguration.seedClientConnection.kubeconfig`.
	RuntimeCluster Target
	// GardenCluster is the landscaper target containing the kubeconfig for the
	// Garden cluster (having Gardener resource groups!)
	GardenCluster Target
	// ImageVectorOverwrite contains the image vector override
	ImageVectorOverwrite *runtime.RawExtension
	// ImageVectorOverwrite contains the image vector override for components deployed by the gardenlet
	ComponentImageVectorOverwrites *runtime.RawExtension
	// SeedBackup contains configuration for an optional backup provider for the Seed cluster registered by the Gardenlet
	// required when gardenlet.componentConfiguration.seedConfig.backup.secretRef is not set
	// backup secret is deployed into the Garden cluster
	SeedBackup *SeedBackup
	// RevisionHistoryLimit is the revision history limit for the Gardenlet deployment
	// Defaults to 10
	RevisionHistoryLimit *int32
	// Resources are the resource requirements for the Gardenlet pod
	Resources *corev1.ResourceRequirements
	// ComponentConfiguration is the Gardenlet component configuration
	// NOTE: please set componentConfiguration.SeedConfig.Spec.Settings.VerticalPodAutoscaler.Enabled to enable VPA for the configured Seed
	ComponentConfiguration gardenletconfigv1alpha1.GardenletConfiguration
}

// SeedBackup contains configuration for an optional backup provider for the Seed cluster registered by the Gardenlet
type SeedBackup struct {
	// Provider is the provider name {aws,gcp,...}
	Provider string
	// Credentials contains provider specific credentials
	// Please check the documentation of the respective extension provider for the concrete format
	Credentials  *runtime.RawExtension
}

// Taken from github.com/gardener/landscaper/pkg/apis/core/v1alpha1
// Avoid importing the Landscaper repository for a simple type

// TargetType defines the type of the target.
type TargetType string

// Target defines a specific data object that defines target environment.
// Every deploy item can have a target which is used by the deployer to install the specific application.
type Target struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	Spec TargetSpec
}

// TargetSpec contains the definition of a target.
type TargetSpec struct {
	// Type is the type of the target that defines its data structure.
	// The actual schema may be defined by a target type crd in the future.
	Type TargetType
	// Configuration contains the target type specific configuration.
	Configuration KubernetesClusterTargetConfig
}

// KubernetesClusterTargetConfig defines the landscaper kubenretes cluster target config.
type KubernetesClusterTargetConfig struct {
	// Kubeconfig defines kubeconfig as string.
	Kubeconfig string
}