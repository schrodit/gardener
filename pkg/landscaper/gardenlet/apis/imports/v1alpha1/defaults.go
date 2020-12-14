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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/gardenlet/apis/config/v1alpha1"
	"github.com/gardener/gardener/pkg/operation/common"
)

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

// SetDefaults_Imports sets defaults for the configuration of the Gardenlet Landscaper.
func SetDefaults_Imports(obj *Imports) {
	if obj.ComponentConfiguration.GardenClientConnection == nil {
		obj.ComponentConfiguration.GardenClientConnection = &v1alpha1.GardenClientConnection{}
		componentbaseconfigv1alpha1.RecommendedDefaultClientConnectionConfiguration(&obj.ComponentConfiguration.GardenClientConnection.ClientConnectionConfiguration)
		obj.ComponentConfiguration.GardenClientConnection.QPS = 100
		obj.ComponentConfiguration.GardenClientConnection.Burst = 130
		obj.ComponentConfiguration.GardenClientConnection.AcceptContentTypes = "application/json"
		obj.ComponentConfiguration.GardenClientConnection.ContentType = "application/json"
	}

	if obj.ComponentConfiguration.GardenClientConnection.BootstrapKubeconfig == nil {
		obj.ComponentConfiguration.GardenClientConnection.BootstrapKubeconfig = &corev1.SecretReference{
			Name:      common.GardenletDefaultKubeconfigBootstrapSecretName,
			Namespace: v1beta1constants.GardenNamespace,
		}
	}

	if obj.ComponentConfiguration.GardenClientConnection.KubeconfigSecret == nil {
		obj.ComponentConfiguration.GardenClientConnection.KubeconfigSecret = &corev1.SecretReference{
			Name:      common.GardenletDefaultKubeconfigSecretName,
			Namespace: v1beta1constants.GardenNamespace,
		}
	}
}
