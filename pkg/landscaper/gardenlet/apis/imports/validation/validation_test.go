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

package validation_test

import (
	"encoding/json"

	landscaperv1alpha1 "github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardenletconfigv1alpha1 "github.com/gardener/gardener/pkg/gardenlet/apis/config/v1alpha1"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/apis/imports"
	. "github.com/gardener/gardener/pkg/landscaper/gardenlet/apis/imports/validation"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("ValidateLandscaperImport", func() {
	var (
		landscaperGardenletImport *imports.Imports
	)

	BeforeEach(func() {
		defaultSNIIngresServiceName := gardenletconfigv1alpha1.DefaultSNIIngresServiceName
		ingressDomain := "super.domain"
		gardenletConfiguration := gardenletconfigv1alpha1.GardenletConfiguration{
			SeedConfig: &gardenletconfigv1alpha1.SeedConfig{
				Seed: gardencorev1beta1.Seed{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-seed",
					},
					Spec: gardencorev1beta1.SeedSpec{
						Provider: gardencorev1beta1.SeedProvider{
							Type:   "a",
							Region: "north-west",
						},
						DNS: gardencorev1beta1.SeedDNS{IngressDomain: &ingressDomain},
						Networks: gardencorev1beta1.SeedNetworks{
							Pods:     "100.96.0.0/32",
							Services: "100.40.0.0/32",
						},
					},
				},
			},
			Server: &gardenletconfigv1alpha1.ServerConfiguration{
				HTTPS: gardenletconfigv1alpha1.HTTPSServer{
					Server: gardenletconfigv1alpha1.Server{
						BindAddress: "0.0.0.0",
						Port:        2720,
					},
				},
			},
			SNI: &gardenletconfigv1alpha1.SNI{
				Ingress: &gardenletconfigv1alpha1.SNIIngress{
					ServiceName: &defaultSNIIngresServiceName,
					Namespace:   pointer.StringPtr("xy"),
					Labels: map[string]string{
						"some-label": "to make validation happy",
					},
				},
			},
		}

		landscaperGardenletImport = &imports.Imports{
			RuntimeCluster: landscaperv1alpha1.Target{Spec: landscaperv1alpha1.TargetSpec{
				Configuration: []byte("dummy"),
			}},
			GardenCluster: landscaperv1alpha1.Target{Spec: landscaperv1alpha1.TargetSpec{
				Configuration: []byte("dummy"),
			}},
			ComponentConfiguration: gardenletConfiguration,
		}

	})

	Describe("#ValidGardenletConfiguration", func() {
		It("should allow valid configurations", func() {
			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(BeEmpty())
		})

		It("validate the runtime cluster is set", func() {
			landscaperGardenletImport.RuntimeCluster = landscaperv1alpha1.Target{}
			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeRequired),
					"Field": Equal("runtimeCluster"),
				})),
			))
		})

		It("validate the garden cluster is set", func() {
			landscaperGardenletImport.GardenCluster = landscaperv1alpha1.Target{}
			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeRequired),
					"Field": Equal("gardenCluster"),
				})),
			))
		})

		It("validate the backup configuration - do not require landscaperImports.SeedBackup to be set if Seed is configured with Backup", func() {
			landscaperGardenletImport.ComponentConfiguration.SeedConfig.Spec.Backup = &gardencorev1beta1.SeedBackup{
				Provider: "a",
				SecretRef: corev1.SecretReference{
					Name:      "a",
					Namespace: "b",
				},
			}

			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(BeEmpty())
		})

		It("validate the backup configuration - require field componentConfiguration.SeedConfig.Spec.Backup in imports to be set when field seedBackup is configured", func() {
			landscaperGardenletImport.SeedBackup = &imports.SeedBackup{
				Credentials: json.RawMessage{},
			}

			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeRequired),
					"Field": Equal("componentConfiguration.seedConfig.spec.backup"),
				})),
			))
		})

		It("validate the backup configuration - Seed Backup provider missing", func() {
			landscaperGardenletImport.ComponentConfiguration.SeedConfig.Spec.Backup = &gardencorev1beta1.SeedBackup{
				Provider: "a",
				SecretRef: corev1.SecretReference{
					Name:      "a",
					Namespace: "b",
				},
			}
			landscaperGardenletImport.SeedBackup = &imports.SeedBackup{
				Credentials: json.RawMessage{},
			}

			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal(field.ErrorTypeRequired),
					"Field":  Equal("componentConfiguration.seedConfig.spec.backup.provider"),
					"Detail": ContainSubstring("seedBackup.provider"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeRequired),
					"Field": Equal("componentConfiguration.seedConfig.spec.backup.provider"),
				})),
			))
		})

		It("validate the backup configuration - Seed Backup credentials missing", func() {
			landscaperGardenletImport.ComponentConfiguration.SeedConfig.Spec.Backup = &gardencorev1beta1.SeedBackup{
				Provider: "a",
				SecretRef: corev1.SecretReference{
					Name:      "a",
					Namespace: "b",
				},
			}
			landscaperGardenletImport.SeedBackup = &imports.SeedBackup{
				Provider: "a",
			}

			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeRequired),
					"Field": Equal("componentConfiguration.seedConfig.spec.backup.credentials"),
				})),
			))
		})

		It("validate the backup configuration - Seed Backup provider does not match", func() {
			landscaperGardenletImport.ComponentConfiguration.SeedConfig.Spec.Backup = &gardencorev1beta1.SeedBackup{
				Provider: "a",
				SecretRef: corev1.SecretReference{
					Name:      "a",
					Namespace: "b",
				},
			}
			landscaperGardenletImport.SeedBackup = &imports.SeedBackup{
				Provider:    "b",
				Credentials: json.RawMessage{},
			}

			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal(field.ErrorTypeRequired),
					"Field":  Equal("componentConfiguration.seedConfig.spec.backup.provider"),
					"Detail": ContainSubstring("seedBackup.provider"),
				})),
			))
		})

		It("validate the component configuration configuration", func() {
			// only pick one required component configuration to show that the configuration is verified
			landscaperGardenletImport.ComponentConfiguration.SNI = nil
			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(HaveLen(1))
		})

		It("validate the Seed configuration", func() {
			// only pick one required Seed configuration to show that the configuration is verified
			landscaperGardenletImport.ComponentConfiguration.SeedConfig.Spec.Provider.Type = ""
			errorList := ValidateLandscaperImport(landscaperGardenletImport)
			Expect(errorList).To(HaveLen(1))
		})
	})
})
