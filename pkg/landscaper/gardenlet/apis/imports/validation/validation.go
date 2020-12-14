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

package validation

import (
	gardencore "github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corevalidation "github.com/gardener/gardener/pkg/apis/core/validation"
	gardenletconfig "github.com/gardener/gardener/pkg/gardenlet/apis/config"
	gardenletconfigv1alpha1 "github.com/gardener/gardener/pkg/gardenlet/apis/config/v1alpha1"
	gardenletvalidation "github.com/gardener/gardener/pkg/gardenlet/apis/config/validation"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/apis/imports"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateLandscaperImport validates a Imports object.
func ValidateLandscaperImport(imports *imports.Imports) field.ErrorList {
	allErrs := field.ErrorList{}

	if imports.GardenCluster.Spec.Configuration == nil {
		return append(allErrs, field.Required(field.NewPath("gardenCluster"), "the Garden cluster kubeconfig has to be provided."))
	}

	if imports.RuntimeCluster.Spec.Configuration == nil {
		return append(allErrs, field.Required(field.NewPath("runtimeCluster"), "the Runtime cluster kubeconfig has to be provided."))
	}

	allErrs = validateBackup(imports)

	componentConfigurationPath := field.NewPath("componentConfiguration")
	config := &gardenletconfig.GardenletConfiguration{}
	if err := gardenletconfigv1alpha1.Convert_v1alpha1_GardenletConfiguration_To_config_GardenletConfiguration(&imports.ComponentConfiguration, config, nil); err != nil {
		return append(allErrs, field.Invalid(componentConfigurationPath, imports.ComponentConfiguration, "failed to validate Gardenlet component configuration"))
	}

	allErrs = append(allErrs, gardenletvalidation.ValidateGardenletConfiguration(config)...)

	if config.GardenClientConnection != nil && len(config.GardenClientConnection.Kubeconfig) > 0 {
		allErrs = append(allErrs, field.Forbidden(componentConfigurationPath.Child("gardenClientConnection"), "directly supplying a Garden kubeconfig and therefore not using TLS bootstrapping, is not supported."))
	}

	if imports.ComponentConfiguration.SeedConfig == nil {
		return append(allErrs, field.Required(componentConfigurationPath.Child("seedConfig"), "the seed configuration has to be provided. This is used to automatically register the seed."))
	}

	seed := &gardencore.Seed{}
	if err := gardencorev1beta1.Convert_v1beta1_Seed_To_core_Seed(&imports.ComponentConfiguration.SeedConfig.Seed, seed, nil); err != nil {
		return append(allErrs, field.Invalid(componentConfigurationPath.Child("seedConfig"), imports.ComponentConfiguration.SeedConfig.Seed, "failed to validate SeedConfig"))
	}

	return append(allErrs, corevalidation.ValidateSeed(seed)...)
}

// validateBackup validates the Seed Backup configuration in the gardenlet landscaper imports
func validateBackup(imports *imports.Imports) field.ErrorList {
	allErrs := field.ErrorList{}

	if imports.SeedBackup == nil {
		return allErrs
	}

	seedBackupPath := field.NewPath("componentConfiguration.seedConfig.spec.backup")

	if imports.ComponentConfiguration.SeedConfig.Spec.Backup == nil {
		return append(allErrs, field.Required(seedBackupPath, "the Seed has to to have backup enabled when the Gardenlet landscaper is configured to deploy a backup secret via \"seedBackup\""))
	}

	if imports.ComponentConfiguration.SeedConfig.Spec.Backup != nil && imports.ComponentConfiguration.SeedConfig.Spec.Backup.Provider != imports.SeedBackup.Provider {
		allErrs = append(allErrs, field.Required(seedBackupPath.Child("provider"), "seed backup provider name must match the Seed Backup provider in \"seedBackup.provider\""))
	}

	if len(imports.SeedBackup.Provider) == 0 {
		allErrs = append(allErrs, field.Required(seedBackupPath.Child("provider"), "seed backup provider must be defined when configuring backups"))
	}
	if imports.SeedBackup.Credentials == nil {
		allErrs = append(allErrs, field.Required(seedBackupPath.Child("credentials"), "seed backup provider credentials must be defined when configuring backups"))
	}

	return allErrs
}
