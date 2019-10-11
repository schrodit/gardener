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

package helper_test

import (
	"time"

	gardencorev1alpha1 "github.com/gardener/gardener/pkg/apis/core/v1alpha1"
	v1alpha1constants "github.com/gardener/gardener/pkg/apis/core/v1alpha1/constants"
	. "github.com/gardener/gardener/pkg/apis/core/v1alpha1/helper"

	"github.com/Masterminds/semver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("helper", func() {
	var (
		trueVar  = true
		falseVar = false
	)

	Describe("errors", func() {
		var zeroTime metav1.Time

		DescribeTable("#UpdatedCondition",
			func(condition gardencorev1alpha1.Condition, status gardencorev1alpha1.ConditionStatus, reason, message string, matcher types.GomegaMatcher) {
				updated := UpdatedCondition(condition, status, reason, message)

				Expect(updated).To(matcher)
			},
			Entry("no update",
				gardencorev1alpha1.Condition{
					Status:  gardencorev1alpha1.ConditionTrue,
					Reason:  "reason",
					Message: "message",
				},
				gardencorev1alpha1.ConditionTrue,
				"reason",
				"message",
				MatchFields(IgnoreExtras, Fields{
					"Status":             Equal(gardencorev1alpha1.ConditionTrue),
					"Reason":             Equal("reason"),
					"Message":            Equal("message"),
					"LastTransitionTime": Equal(zeroTime),
					"LastUpdateTime":     Not(Equal(zeroTime)),
				}),
			),
			Entry("update reason",
				gardencorev1alpha1.Condition{
					Status:  gardencorev1alpha1.ConditionTrue,
					Reason:  "reason",
					Message: "message",
				},
				gardencorev1alpha1.ConditionTrue,
				"OtherReason",
				"message",
				MatchFields(IgnoreExtras, Fields{
					"Status":             Equal(gardencorev1alpha1.ConditionTrue),
					"Reason":             Equal("OtherReason"),
					"Message":            Equal("message"),
					"LastTransitionTime": Equal(zeroTime),
					"LastUpdateTime":     Not(Equal(zeroTime)),
				}),
			),
			Entry("update status",
				gardencorev1alpha1.Condition{
					Status:  gardencorev1alpha1.ConditionTrue,
					Reason:  "reason",
					Message: "message",
				},
				gardencorev1alpha1.ConditionFalse,
				"OtherReason",
				"message",
				MatchFields(IgnoreExtras, Fields{
					"Status":             Equal(gardencorev1alpha1.ConditionFalse),
					"Reason":             Equal("OtherReason"),
					"Message":            Equal("message"),
					"LastTransitionTime": Not(Equal(zeroTime)),
					"LastUpdateTime":     Not(Equal(zeroTime)),
				}),
			),
		)

		Describe("#MergeConditions", func() {
			It("should merge the conditions", func() {
				var (
					typeFoo gardencorev1alpha1.ConditionType = "foo"
					typeBar gardencorev1alpha1.ConditionType = "bar"
				)

				oldConditions := []gardencorev1alpha1.Condition{
					{
						Type:   typeFoo,
						Reason: "hugo",
					},
				}

				result := MergeConditions(oldConditions, gardencorev1alpha1.Condition{Type: typeFoo}, gardencorev1alpha1.Condition{Type: typeBar})

				Expect(result).To(Equal([]gardencorev1alpha1.Condition{{Type: typeFoo}, {Type: typeBar}}))
			})
		})

		Describe("#GetCondition", func() {
			It("should return the found condition", func() {
				var (
					conditionType gardencorev1alpha1.ConditionType = "test-1"
					condition                                      = gardencorev1alpha1.Condition{
						Type: conditionType,
					}
					conditions = []gardencorev1alpha1.Condition{condition}
				)

				cond := GetCondition(conditions, conditionType)

				Expect(cond).NotTo(BeNil())
				Expect(*cond).To(Equal(condition))
			})

			It("should return nil because the required condition could not be found", func() {
				var (
					conditionType gardencorev1alpha1.ConditionType = "test-1"
					conditions                                     = []gardencorev1alpha1.Condition{}
				)

				cond := GetCondition(conditions, conditionType)

				Expect(cond).To(BeNil())
			})
		})

		Describe("#GetOrInitCondition", func() {
			It("should get the existing condition", func() {
				var (
					c          = gardencorev1alpha1.Condition{Type: "foo"}
					conditions = []gardencorev1alpha1.Condition{c}
				)

				Expect(GetOrInitCondition(conditions, "foo")).To(Equal(c))
			})

			It("should return a new, initialized condition", func() {
				tmp := Now
				Now = func() metav1.Time {
					return metav1.NewTime(time.Unix(0, 0))
				}
				defer func() { Now = tmp }()

				Expect(GetOrInitCondition(nil, "foo")).To(Equal(InitCondition("foo")))
			})
		})

		DescribeTable("#IsResourceSupported",
			func(resources []gardencorev1alpha1.ControllerResource, resourceKind, resourceType string, expectation bool) {
				Expect(IsResourceSupported(resources, resourceKind, resourceType)).To(Equal(expectation))
			},
			Entry("expect true",
				[]gardencorev1alpha1.ControllerResource{
					{
						Kind: "foo",
						Type: "bar",
					},
				},
				"foo",
				"bar",
				true,
			),
			Entry("expect true",
				[]gardencorev1alpha1.ControllerResource{
					{
						Kind: "foo",
						Type: "bar",
					},
				},
				"foo",
				"BAR",
				true,
			),
			Entry("expect false",
				[]gardencorev1alpha1.ControllerResource{
					{
						Kind: "foo",
						Type: "bar",
					},
				},
				"foo",
				"baz",
				false,
			),
		)

		DescribeTable("#IsControllerInstallationSuccessful",
			func(conditions []gardencorev1alpha1.Condition, expectation bool) {
				controllerInstallation := gardencorev1alpha1.ControllerInstallation{
					Status: gardencorev1alpha1.ControllerInstallationStatus{
						Conditions: conditions,
					},
				}
				Expect(IsControllerInstallationSuccessful(controllerInstallation)).To(Equal(expectation))
			},
			Entry("expect true",
				[]gardencorev1alpha1.Condition{
					{
						Type:   gardencorev1alpha1.ControllerInstallationInstalled,
						Status: gardencorev1alpha1.ConditionTrue,
					},
				},
				true,
			),
			Entry("expect false",
				[]gardencorev1alpha1.Condition{
					{
						Type:   gardencorev1alpha1.ControllerInstallationInstalled,
						Status: gardencorev1alpha1.ConditionFalse,
					},
				},
				false,
			),
			Entry("expect false",
				[]gardencorev1alpha1.Condition{},
				false,
			),
		)

		DescribeTable("#TaintsHave",
			func(taints []gardencorev1alpha1.SeedTaint, key string, expectation bool) {
				Expect(TaintsHave(taints, key)).To(Equal(expectation))
			},
			Entry("taint exists", []gardencorev1alpha1.SeedTaint{{Key: "foo"}}, "foo", true),
			Entry("taint does not exist", []gardencorev1alpha1.SeedTaint{{Key: "foo"}}, "bar", false),
		)

		Describe("#ReadShootedSeed", func() {
			var (
				shoot                    *gardencorev1alpha1.Shoot
				defaultReplicas          int32 = 3
				defaultMinReplicas       int32 = 3
				defaultMaxReplicas       int32 = 3
				defaultMinimumVolumeSize       = "20Gi"

				defaultAPIServerAutoscaler = ShootedSeedAPIServerAutoscaler{
					MinReplicas: &defaultMinReplicas,
					MaxReplicas: defaultMaxReplicas,
				}

				defaultAPIServer = ShootedSeedAPIServer{
					Replicas:   &defaultReplicas,
					Autoscaler: &defaultAPIServerAutoscaler,
				}

				defaultShootedSeed = ShootedSeed{
					APIServer: &defaultAPIServer,
					Backup:    &gardencorev1alpha1.SeedBackup{},
				}
			)

			BeforeEach(func() {
				shoot = &gardencorev1alpha1.Shoot{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   v1alpha1constants.GardenNamespace,
						Annotations: nil,
					},
				}
			})

			It("should return false,nil,nil because shoot is not in the garden namespace", func() {
				shoot.Namespace = "default"

				shootedSeed, err := ReadShootedSeed(shoot)

				Expect(err).NotTo(HaveOccurred())
				Expect(shootedSeed).To(BeNil())
			})

			It("should return false,nil,nil because annotation is not set", func() {
				shootedSeed, err := ReadShootedSeed(shoot)

				Expect(err).NotTo(HaveOccurred())
				Expect(shootedSeed).To(BeNil())
			})

			It("should return false,nil,nil because annotation is set with no usages", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "",
				}

				shootedSeed, err := ReadShootedSeed(shoot)

				Expect(err).NotTo(HaveOccurred())
				Expect(shootedSeed).To(BeNil())
			})

			It("should return true,nil,nil because annotation is set with normal usage", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true",
				}

				shootedSeed, err := ReadShootedSeed(shoot)

				Expect(err).NotTo(HaveOccurred())
				Expect(shootedSeed).To(Equal(&defaultShootedSeed))
			})

			It("should return true,true,true because annotation is set with protected and visible usage", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true,protected,visible",
				}

				shootedSeed, err := ReadShootedSeed(shoot)

				Expect(err).NotTo(HaveOccurred())
				Expect(shootedSeed).To(Equal(&ShootedSeed{
					Protected: &trueVar,
					Visible:   &trueVar,
					APIServer: &defaultAPIServer,
					Backup:    &gardencorev1alpha1.SeedBackup{},
				}))
			})

			It("should return true,true,true because annotation is set with unprotected and invisible usage", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true,unprotected,invisible",
				}

				shootedSeed, err := ReadShootedSeed(shoot)

				Expect(err).NotTo(HaveOccurred())
				Expect(shootedSeed).To(Equal(&ShootedSeed{
					Protected:         &falseVar,
					Visible:           &falseVar,
					APIServer:         &defaultAPIServer,
					Backup:            &gardencorev1alpha1.SeedBackup{},
					MinimumVolumeSize: nil,
				}))
			})

			It("should return the min volume size because annotation is set properly", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true,unprotected,invisible,minimumVolumeSize=20Gi",
				}

				shootedSeed, err := ReadShootedSeed(shoot)

				Expect(err).NotTo(HaveOccurred())
				Expect(shootedSeed).To(Equal(&ShootedSeed{
					Protected:         &falseVar,
					Visible:           &falseVar,
					APIServer:         &defaultAPIServer,
					Backup:            &gardencorev1alpha1.SeedBackup{},
					MinimumVolumeSize: &defaultMinimumVolumeSize,
				}))
			})

			It("should return a filled apiserver config", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true,apiServer.replicas=1,apiServer.autoscaler.minReplicas=2,apiServer.autoscaler.maxReplicas=3",
				}

				shootedSeed, err := ReadShootedSeed(shoot)

				var (
					one   int32 = 1
					two   int32 = 2
					three int32 = 3
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(shootedSeed).To(Equal(&ShootedSeed{
					APIServer: &ShootedSeedAPIServer{
						Replicas: &one,
						Autoscaler: &ShootedSeedAPIServerAutoscaler{
							MinReplicas: &two,
							MaxReplicas: three,
						},
					},
					Backup: &gardencorev1alpha1.SeedBackup{},
				}))
			})

			It("should fail due to maxReplicas not being specified", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true,apiServer.autoscaler.minReplicas=2",
				}

				_, err := ReadShootedSeed(shoot)
				Expect(err).To(HaveOccurred())
			})

			It("should fail due to API server replicas being less than one", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true,apiServer.replicas=0",
				}

				_, err := ReadShootedSeed(shoot)
				Expect(err).To(HaveOccurred())
			})

			It("should fail due to API server autoscaler minReplicas being less than one", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true,apiServer.autoscaler.minReplicas=0,apiServer.autoscaler.maxReplicas=1",
				}

				_, err := ReadShootedSeed(shoot)
				Expect(err).To(HaveOccurred())
			})

			It("should fail due to API server autoscaler maxReplicas being less than one", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true,apiServer.autoscaler.maxReplicas=0",
				}

				_, err := ReadShootedSeed(shoot)
				Expect(err).To(HaveOccurred())
			})

			It("should fail due to API server autoscaler minReplicas being greater than maxReplicas", func() {
				shoot.Annotations = map[string]string{
					v1alpha1constants.AnnotationShootUseAsSeed: "true,apiServer.autoscaler.maxReplicas=1,apiServer.autoscaler.minReplicas=2",
				}

				_, err := ReadShootedSeed(shoot)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	DescribeTable("#HibernationIsEnabled",
		func(shoot *gardencorev1alpha1.Shoot, hibernated bool) {
			Expect(HibernationIsEnabled(shoot)).To(Equal(hibernated))
		},
		Entry("no hibernation section", &gardencorev1alpha1.Shoot{}, false),
		Entry("hibernation.enabled = false", &gardencorev1alpha1.Shoot{
			Spec: gardencorev1alpha1.ShootSpec{
				Hibernation: &gardencorev1alpha1.Hibernation{Enabled: &falseVar},
			},
		}, false),
		Entry("hibernation.enabled = true", &gardencorev1alpha1.Shoot{
			Spec: gardencorev1alpha1.ShootSpec{
				Hibernation: &gardencorev1alpha1.Hibernation{Enabled: &trueVar},
			},
		}, true),
	)

	DescribeTable("#ShootWantsClusterAutoscaler",
		func(shoot *gardencorev1alpha1.Shoot, wantsAutoscaler bool) {
			actualWantsAutoscaler, err := ShootWantsClusterAutoscaler(shoot)

			Expect(err).NotTo(HaveOccurred())
			Expect(actualWantsAutoscaler).To(Equal(wantsAutoscaler))
		},

		Entry("no workers",
			&gardencorev1alpha1.Shoot{
				Spec: gardencorev1alpha1.ShootSpec{},
			},
			false),

		Entry("one worker no difference in auto scaler max and min",
			&gardencorev1alpha1.Shoot{
				Spec: gardencorev1alpha1.ShootSpec{
					Provider: gardencorev1alpha1.Provider{
						Workers: []gardencorev1alpha1.Worker{{Name: "foo"}},
					},
				},
			},
			false),

		Entry("one worker with difference in auto scaler max and min",
			&gardencorev1alpha1.Shoot{
				Spec: gardencorev1alpha1.ShootSpec{
					Provider: gardencorev1alpha1.Provider{
						Workers: []gardencorev1alpha1.Worker{{Name: "foo", Minimum: 1, Maximum: 2}},
					},
				},
			},
			true),
	)

	Describe("#ShootMachineImageVersionExists", func() {
		var (
			constraint        gardencorev1alpha1.MachineImage
			shootMachineImage gardencorev1alpha1.ShootMachineImage
		)

		BeforeEach(func() {
			constraint = gardencorev1alpha1.MachineImage{
				Name: "coreos",
				Versions: []gardencorev1alpha1.ExpirableVersion{
					{Version: "0.0.2"},
					{Version: "0.0.3"},
				},
			}

			shootMachineImage = gardencorev1alpha1.ShootMachineImage{
				Name:    "coreos",
				Version: "0.0.2",
			}
		})

		It("should determine that the version exists", func() {
			exists, index := ShootMachineImageVersionExists(constraint, shootMachineImage)
			Expect(exists).To(Equal(trueVar))
			Expect(index).To(Equal(0))
		})

		It("should determine that the version does not exist", func() {
			shootMachineImage.Name = "xy"
			exists, _ := ShootMachineImageVersionExists(constraint, shootMachineImage)
			Expect(exists).To(Equal(false))
		})

		It("should determine that the version does not exist", func() {
			shootMachineImage.Version = "0.0.4"
			exists, _ := ShootMachineImageVersionExists(constraint, shootMachineImage)
			Expect(exists).To(Equal(false))
		})
	})

	Describe("#GetShootMachineImageFromLatestMachineImageVersion", func() {
		It("should return the Machine Image containing only the latest machine image version", func() {
			latestVersion := "1.0.0"
			inputImage := gardencorev1alpha1.MachineImage{
				Name: "coreos",
				Versions: []gardencorev1alpha1.ExpirableVersion{
					{Version: "0.0.2"},
					{Version: latestVersion},
					{Version: "0.0.2"},
				},
			}

			version, image, err := GetShootMachineImageFromLatestMachineImageVersion(inputImage)
			Expect(err).NotTo(HaveOccurred())

			latestSemverVersion, _ := semver.NewVersion(latestVersion)
			Expect(version).To(Equal(latestSemverVersion))
			Expect(image.Name).To(Equal("coreos"))
			Expect(image.Version).To(Equal(latestVersion))
		})

		It("should return the Machine Image", func() {
			latestVersion := "1.1"
			inputImage := gardencorev1alpha1.MachineImage{
				Name: "coreos",
				Versions: []gardencorev1alpha1.ExpirableVersion{
					{Version: latestVersion},
				},
			}

			version, image, err := GetShootMachineImageFromLatestMachineImageVersion(inputImage)
			Expect(err).NotTo(HaveOccurred())

			latestSemverVersion, err := semver.NewVersion(latestVersion)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal(latestSemverVersion))
			Expect(image.Version).To(Equal(latestVersion))
		})

		It("should return an error for invalid semVerVersion", func() {
			inputImage := gardencorev1alpha1.MachineImage{
				Name: "coreos",
				Versions: []gardencorev1alpha1.ExpirableVersion{
					{Version: "0.0.XX"},
				},
			}

			_, _, err := GetShootMachineImageFromLatestMachineImageVersion(inputImage)
			Expect(err).To(HaveOccurred())
		})
	})

	var (
		kubernetesSettings = gardencorev1alpha1.KubernetesSettings{
			Versions: []gardencorev1alpha1.ExpirableVersion{
				{Version: "1.15.1"},
				{Version: "1.14.4"},
				{Version: "1.12.9"},
			},
		}
	)

	DescribeTable("#DetermineLatestKubernetesPatchVersion",
		func(cloudProfile gardencorev1alpha1.CloudProfile, currentVersion, expectedVersion string, expectVersion bool) {
			ok, newVersion, err := DetermineLatestKubernetesPatchVersion(&cloudProfile, currentVersion)
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(Equal(expectVersion))
			Expect(newVersion).To(Equal(expectedVersion))
		},

		Entry("version = 1.15.1",
			gardencorev1alpha1.CloudProfile{
				Spec: gardencorev1alpha1.CloudProfileSpec{
					Kubernetes: kubernetesSettings,
				},
			},
			"1.15.0",
			"1.15.1",
			true,
		),

		Entry("version = 1.12.9",
			gardencorev1alpha1.CloudProfile{
				Spec: gardencorev1alpha1.CloudProfileSpec{
					Kubernetes: kubernetesSettings,
				},
			},
			"1.12.4",
			"1.12.9",
			true,
		),

		Entry("no new version",
			gardencorev1alpha1.CloudProfile{
				Spec: gardencorev1alpha1.CloudProfileSpec{
					Kubernetes: kubernetesSettings,
				},
			},
			"1.15.1",
			"",
			false,
		),
	)

	DescribeTable("#DetermineNextKubernetesMinorVersion",
		func(cloudProfile gardencorev1alpha1.CloudProfile, currentVersion, expectedVersion string, expectVersion bool) {
			ok, newVersion, err := DetermineNextKubernetesMinorVersion(&cloudProfile, currentVersion)

			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(Equal(expectVersion))
			Expect(newVersion).To(Equal(expectedVersion))
		},

		Entry("version = 1.15.1",
			gardencorev1alpha1.CloudProfile{
				Spec: gardencorev1alpha1.CloudProfileSpec{
					Kubernetes: kubernetesSettings,
				},
			},
			"1.14.4",
			"1.15.1",
			true,
		),

		Entry("version = 1.12.9",
			gardencorev1alpha1.CloudProfile{
				Spec: gardencorev1alpha1.CloudProfileSpec{
					Kubernetes: kubernetesSettings,
				},
			},
			"1.11.0",
			"1.12.9",
			true,
		),

		Entry("no new version",
			gardencorev1alpha1.CloudProfile{
				Spec: gardencorev1alpha1.CloudProfileSpec{
					Kubernetes: kubernetesSettings,
				},
			},
			"1.15.1",
			"",
			false,
		),
	)

	Describe("#GetShootMachineImageFromLatestExpirableVersion", func() {
		It("should return the Machine Image containing only the latest machine image version", func() {
			latestVersion := "1.0.0"
			inputImage := gardencorev1alpha1.MachineImage{
				Name: "coreos",
				Versions: []gardencorev1alpha1.ExpirableVersion{
					{
						Version: "0.0.2",
					},
					{
						Version: latestVersion,
					},
					{
						Version: "0.0.2",
					},
				},
			}

			version, image, err := GetShootMachineImageFromLatestMachineImageVersion(inputImage)
			Expect(err).NotTo(HaveOccurred())

			latestSemverVersion, _ := semver.NewVersion(latestVersion)
			Expect(version).To(Equal(latestSemverVersion))
			Expect(image.Name).To(Equal("coreos"))
			Expect(image.Version).To(Equal(latestVersion))
		})

		It("should return the Machine Image", func() {
			latestVersion := "1.1"
			inputImage := gardencorev1alpha1.MachineImage{
				Name: "coreos",
				Versions: []gardencorev1alpha1.ExpirableVersion{
					{
						Version: latestVersion,
					},
				},
			}

			version, image, err := GetShootMachineImageFromLatestMachineImageVersion(inputImage)
			Expect(err).NotTo(HaveOccurred())

			latestSemverVersion, err := semver.NewVersion(latestVersion)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal(latestSemverVersion))
			Expect(image.Version).To(Equal(latestVersion))
		})

		It("should return an error for invalid semVerVersion", func() {
			inputImage := gardencorev1alpha1.MachineImage{
				Name: "coreos",
				Versions: []gardencorev1alpha1.ExpirableVersion{
					{
						Version: "0.0.XX",
					},
				},
			}

			_, _, err := GetShootMachineImageFromLatestMachineImageVersion(inputImage)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("#ShootExpirableVersionExists", func() {
		var (
			constraint        gardencorev1alpha1.MachineImage
			shootMachineImage gardencorev1alpha1.ShootMachineImage
		)
		BeforeEach(func() {
			constraint = gardencorev1alpha1.MachineImage{
				Name: "coreos",
				Versions: []gardencorev1alpha1.ExpirableVersion{
					{
						Version: "0.0.2",
					},
					{
						Version: "0.0.3",
					},
				},
			}

			shootMachineImage = gardencorev1alpha1.ShootMachineImage{
				Name:    "coreos",
				Version: "0.0.2",
			}
		})
		It("should determine that the version exists", func() {
			exists, index := ShootMachineImageVersionExists(constraint, shootMachineImage)
			Expect(exists).To(Equal(trueVar))
			Expect(index).To(Equal(0))
		})
		It("should determine that the version does not exist", func() {
			shootMachineImage.Name = "xy"
			exists, _ := ShootMachineImageVersionExists(constraint, shootMachineImage)
			Expect(exists).To(Equal(false))
		})
		It("should determine that the version does not exist", func() {
			shootMachineImage.Version = "0.0.4"
			exists, _ := ShootMachineImageVersionExists(constraint, shootMachineImage)
			Expect(exists).To(Equal(false))
		})
	})

	var (
		expirableVersions = []gardencorev1alpha1.ExpirableVersion{
			{Version: "1.15.1"},
			{Version: "1.14.4"},
			{Version: "1.12.9"},
		}
	)

	DescribeTable("#DetermineNextKubernetesMinorVersion",
		func(cloudProfile gardencorev1alpha1.CloudProfile, currentVersion, expectedVersion string, expectVersion bool) {
			ok, newVersion, err := DetermineNextKubernetesMinorVersion(&cloudProfile, currentVersion)
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(Equal(expectVersion))
			Expect(newVersion).To(Equal(expectedVersion))
		},
		Entry("version = 1.15.1",
			gardencorev1alpha1.CloudProfile{
				Spec: gardencorev1alpha1.CloudProfileSpec{
					Kubernetes: gardencorev1alpha1.KubernetesSettings{Versions: expirableVersions},
				},
			},
			"1.14.4",
			"1.15.1",
			true,
		),
		Entry("version = 1.12.9",
			gardencorev1alpha1.CloudProfile{
				Spec: gardencorev1alpha1.CloudProfileSpec{
					Kubernetes: gardencorev1alpha1.KubernetesSettings{Versions: expirableVersions},
				},
			},
			"1.11.0",
			"1.12.9",
			true,
		),
		Entry("no new version",
			gardencorev1alpha1.CloudProfile{
				Spec: gardencorev1alpha1.CloudProfileSpec{
					Kubernetes: gardencorev1alpha1.KubernetesSettings{Versions: expirableVersions},
				},
			},
			"1.15.1",
			"",
			false,
		),
	)
})
