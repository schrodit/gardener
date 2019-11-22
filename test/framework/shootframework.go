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
	"context"
	"flag"
	"fmt"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils/retry"
	"github.com/onsi/ginkgo"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsscheme "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	corescheme "k8s.io/client-go/kubernetes/scheme"
	apiregistrationscheme "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/scheme"
	metricsscheme "k8s.io/metrics/pkg/client/clientset/versioned/scheme"
)

var shootCfg *ShootConfig

// ShootConfig is the configuration for a shoot framework
type ShootConfig struct {
	GardenerConfig *GardenerConfig
	ShootName      string

	CreateTestNamespace         bool
	DisableTestNamespaceCleanup bool
}

// ShootFramework represents the shoot test framework that includes
// test functions that can be executed ona specific shoot
type ShootFramework struct {
	*GardenerFramework
	TestDescription
	Config *ShootConfig

	SeedClient  kubernetes.Interface
	ShootClient kubernetes.Interface

	Seed         *gardencorev1beta1.Seed
	CloudProfile *gardencorev1beta1.CloudProfile
	Shoot        *gardencorev1beta1.Shoot
	Project      *gardencorev1beta1.Project

	Namespace string
}

// NewShootFramework creates a new simple Shoot framework
func NewShootFramework(cfg *ShootConfig) *ShootFramework {
	f := &ShootFramework{
		GardenerFramework: NewGardenerFrameworkFromConfig(nil),
		TestDescription:   NewTestDescription("SHOOT"),
		Config:            cfg,
	}

	ginkgo.BeforeEach(func() {
		f.GardenerFramework.BeforeEach()
		f.BeforeEach()
	})
	CAfterEach(f.AfterEach, 10*time.Minute)
	return f
}

// NewShootFrameworkFromConfig creates a new Shoot framework from a shoot configuration
func NewShootFrameworkFromConfig(cfg *ShootConfig) (*ShootFramework, error) {
	var gardenerConfig *GardenerConfig
	if cfg != nil {
		gardenerConfig = cfg.GardenerConfig
	}
	f := &ShootFramework{
		GardenerFramework: NewGardenerFramework(gardenerConfig),
		TestDescription:   NewTestDescription("SHOOT"),
		Config:            cfg,
	}
	if cfg != nil && gardenerConfig != nil {
		if err := f.AddShoot(context.TODO(), cfg.ShootName, cfg.GardenerConfig.ProjectNamespace); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// BeforeEach should be called in ginkgo's BeforeEach.
// It sets up the shoot framework.
func (f *ShootFramework) BeforeEach() {
	ctx := context.Background()
	defer ctx.Done()
	f.Config = mergeShootConfig(f.Config, shootCfg)
	validateFlags(f.Config)
	err := f.AddShoot(ctx, f.Config.ShootName, f.ProjectNamespace)
	ExpectNoError(err)

	if f.Config.CreateTestNamespace {
		_, err := f.CreateNewNamespace(ctx)
		ExpectNoError(err)
	}
}

// AfterEach should be called in ginkgo's AfterEach.
// Cleans up resources and dumps the shoot state if the test failed
func (f *ShootFramework) AfterEach(ctx context.Context) {
	if ginkgo.CurrentGinkgoTestDescription().Failed {
		f.DumpState(ctx)
	}
	if !f.Config.DisableTestNamespaceCleanup && f.Namespace != "" {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: f.Namespace},
		}
		err := f.ShootClient.Client().Delete(ctx, ns)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				ExpectNoError(err)
			}
		}
		err = f.WaitUntilNamespaceIsDeleted(ctx, f.ShootClient, f.Namespace)
		ExpectNoError(err)
		f.Namespace = ""
		ginkgo.By(fmt.Sprintf("deleted test namespace %s", f.Namespace))
	}
}

// CreateNewNamespace creates a new namespace with a generated name prefixed with "gardener-e2e-".
// The created namespace is automatically cleaned up when the test is finished.
func (f *ShootFramework) CreateNewNamespace(ctx context.Context) (string, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "gardener-e2e-",
		},
	}
	if err := f.ShootClient.Client().Create(ctx, ns); err != nil {
		return "", err
	}

	f.Namespace = ns.GetName()
	return ns.GetName(), nil
}

// AddShoot sets the shoot and its seed for the GardenerOperation.
func (f *ShootFramework) AddShoot(ctx context.Context, shootName, shootNamespace string) error {
	if f.GardenClient == nil {
		return errors.New("no gardener client is defined")
	}

	var (
		shootClient kubernetes.Interface
		shoot       = &gardencorev1beta1.Shoot{}
		err         error
	)

	if err := f.GardenClient.Client().Get(ctx, client.ObjectKey{Namespace: shootNamespace, Name: shootName}, shoot); err != nil {
		return errors.Wrapf(err, "could not get shoot")
	}

	f.CloudProfile, err = f.GardenerFramework.GetCloudProfile(ctx, shoot.Spec.CloudProfileName)
	if err != nil {
		return errors.Wrapf(err, "unable to get cloudprofile %s", shoot.Spec.CloudProfileName)
	}

	f.Project, err = f.GetShootProject(ctx, shootNamespace)
	if err != nil {
		return err
	}

	f.Seed, f.SeedClient, err = f.GetSeed(ctx, *shoot.Spec.SeedName)
	if err != nil {
		return err
	}

	f.Shoot = shoot

	shootScheme := runtime.NewScheme()
	shootSchemeBuilder := runtime.NewSchemeBuilder(
		corescheme.AddToScheme,
		apiextensionsscheme.AddToScheme,
		apiregistrationscheme.AddToScheme,
		metricsscheme.AddToScheme,
	)
	err = shootSchemeBuilder.AddToScheme(shootScheme)
	if err != nil {
		return errors.Wrap(err, "could not add schemes to shoot scheme")
	}
	if err := retry.UntilTimeout(ctx, k8sClientInitPollInterval, k8sClientInitTimeout, func(ctx context.Context) (bool, error) {
		shootClient, err = kubernetes.NewClientFromSecret(f.SeedClient, computeTechnicalID(f.Project.Name, shoot), gardencorev1beta1.GardenerName, kubernetes.WithClientOptions(client.Options{
			Scheme: shootScheme,
		}))
		if err != nil {
			return retry.MinorError(errors.Wrap(err, "could not construct Shoot client"))
		}
		return retry.Ok()
	}); err != nil {
		return err
	}

	f.ShootClient = shootClient

	return nil
}

func validateFlags(cfg *ShootConfig) {
	if cfg == nil {
		ginkgo.Fail("no shoot framework configuration provided")
	}
	if !StringSet(cfg.ShootName) {
		ginkgo.Fail("You should specify a shootName to test against")
	}
}

func mergeShootConfig(base, overwrite *ShootConfig) *ShootConfig {
	if base == nil {
		return overwrite
	}
	if overwrite == nil {
		return base
	}

	if overwrite.GardenerConfig != nil {
		base.GardenerConfig = overwrite.GardenerConfig
	}
	if StringSet(overwrite.ShootName) {
		base.ShootName = overwrite.ShootName
	}
	if overwrite.CreateTestNamespace {
		base.CreateTestNamespace = overwrite.CreateTestNamespace
	}
	if overwrite.DisableTestNamespaceCleanup {
		base.DisableTestNamespaceCleanup = overwrite.DisableTestNamespaceCleanup
	}

	return base
}

// RegisterShootFrameworkFlags adds all flags that are needed to configure a shoot framework to the provided flagset.
func RegisterShootFrameworkFlags(flagset *flag.FlagSet) *ShootConfig {
	if flagset == nil {
		flagset = flag.CommandLine
	}

	_ = RegisterGardenerFrameworkFlags(flagset)

	newCfg := &ShootConfig{}

	flag.StringVar(&newCfg.ShootName, "shoot-name", "", "name of the shoot")

	shootCfg = newCfg
	return shootCfg
}

// HibernateShoot hibernates the shoot of the framework
func (f *ShootFramework) HibernateShoot(ctx context.Context) error {
	return f.GardenerFramework.HibernateShoot(ctx, f.Shoot)
}

// WakeUpShoot wakes up the hibernated shoot of the framework
func (f *ShootFramework) WakeUpShoot(ctx context.Context) error {
	return f.GardenerFramework.WakeUpShoot(ctx, f.Shoot)
}

// UpdateShoot Updates a shoot from a shoot Object and waits for its reconciliation
func (f *ShootFramework) UpdateShoot(ctx context.Context, update func(shoot *gardencorev1beta1.Shoot) error) error {
	return f.GardenerFramework.UpdateShoot(ctx, f.Shoot, update)
}

// GetCloudProfile returns the cloudprofile of the shoot
func (f *ShootFramework) GetCloudProfile(ctx context.Context) (*gardencorev1beta1.CloudProfile, error) {
	cloudProfile := &gardencorev1beta1.CloudProfile{}
	if err := f.GardenClient.Client().Get(ctx, client.ObjectKey{Name: f.Shoot.Spec.CloudProfileName}, cloudProfile); err != nil {
		return nil, errors.Wrap(err, "could not get Seed's CloudProvider in Garden cluster")
	}
	return cloudProfile, nil
}
