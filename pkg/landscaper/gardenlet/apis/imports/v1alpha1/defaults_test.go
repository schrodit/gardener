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

package v1alpha1_test

import (
	. "github.com/gardener/gardener/pkg/landscaper/gardenlet/apis/imports/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Defaults", func() {
	Describe("#SetDefaults_Imports", func() {
		var obj *Imports

		BeforeEach(func() {
			obj = &Imports{}
		})

		It("should default the imports configuration", func() {
			SetDefaults_Imports(obj)

			Expect(obj.ComponentConfiguration.GardenClientConnection).NotTo(BeNil())
			Expect(obj.ComponentConfiguration.GardenClientConnection.ClientConnectionConfiguration).NotTo(BeNil())
			Expect(obj.ComponentConfiguration.GardenClientConnection.QPS).To(Equal(float32(100)))
			Expect(obj.ComponentConfiguration.GardenClientConnection.Burst).To(Equal(int32(130)))
			Expect(obj.ComponentConfiguration.GardenClientConnection.AcceptContentTypes).To(Equal("application/json"))
			Expect(obj.ComponentConfiguration.GardenClientConnection.ContentType).To(Equal("application/json"))

			Expect(obj.ComponentConfiguration.GardenClientConnection.BootstrapKubeconfig).NotTo(BeNil())
			Expect(obj.ComponentConfiguration.GardenClientConnection.BootstrapKubeconfig.Name).To(Equal("gardenlet-kubeconfig-bootstrap"))
			Expect(obj.ComponentConfiguration.GardenClientConnection.BootstrapKubeconfig.Namespace).To(Equal("garden"))

			Expect(obj.ComponentConfiguration.GardenClientConnection.KubeconfigSecret).NotTo(BeNil())
			Expect(obj.ComponentConfiguration.GardenClientConnection.KubeconfigSecret.Name).To(Equal("gardenlet-kubeconfig"))
			Expect(obj.ComponentConfiguration.GardenClientConnection.KubeconfigSecret.Namespace).To(Equal("garden"))
		})
	})
})
