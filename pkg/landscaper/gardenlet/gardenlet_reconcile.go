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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/applier"
	"github.com/gardener/gardener/pkg/operation/common"
	"github.com/gardener/gardener/pkg/utils"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/kubernetes/bootstrap"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
	"github.com/gardener/gardener/pkg/utils/retry"
)

// GetChartPath gets the path to the chart directory
// exposed for testing
var GetChartPath = func() string { return common.ChartPath }

// Reconcile deploys the Gardenlet into the Seed cluster
func (g *Landscaper) Reconcile(ctx context.Context) error {
	seedConfig := g.Imports.ComponentConfiguration.SeedConfig

	// deploy the Seed secret containing the Seed cluster kubeconfig to the Garden cluster
	// only deployed if secret reference is explicitly configured in the Seed resource in the import configuration
	if g.Imports.ComponentConfiguration.SeedConfig.Spec.SecretRef != nil {
		seedKubeconfig := g.Imports.RuntimeCluster.Spec.Configuration
		if g.Imports.ComponentConfiguration.SeedClientConnection != nil && len(g.Imports.ComponentConfiguration.SeedClientConnection.Kubeconfig) > 0 {
			seedKubeconfig = []byte(g.Imports.ComponentConfiguration.SeedClientConnection.Kubeconfig)
		}

		if err := g.deploySeedSecret(ctx, seedKubeconfig, g.Imports.ComponentConfiguration.SeedConfig.Spec.SecretRef); err != nil {
			return fmt.Errorf("failed to deploy secret for the Seed resource containing the Seed cluster's kubeconfig: %v", err)
		}
	}

	// if configured, deploy the seed-backup secret to the Garden cluster
	if g.Imports.SeedBackup != nil && seedConfig.Spec.Backup != nil {
		credentials := make(map[string][]byte)
		marshalJSON, err := g.Imports.SeedBackup.Credentials.MarshalJSON()
		if err != nil {
			return err
		}

		if err := json.Unmarshal(marshalJSON, &credentials); err != nil {
			return err
		}

		if err := g.deployBackupSecret(ctx, g.Imports.SeedBackup.Provider, credentials, seedConfig.Spec.Backup.SecretRef); err != nil {
			return fmt.Errorf("failed to deploy the Seed backup secret to the Garden cluster: %v", err)
		}
	}

	// check if Gardenlet is already deployed / check Seed exists
	isAlreadyBootstrapped, err := g.isSeedBootstrapped(ctx, seedConfig.ObjectMeta)
	if err != nil {
		return fmt.Errorf("failed to check if seed %q is already bootstrapped: %v", seedConfig.ObjectMeta.Name, err)
	}

	var bootstrapKubeconfig []byte
	if !isAlreadyBootstrapped {
		bootstrapKubeconfig, err = g.getKubeconfigWithBootstrapToken(ctx, seedConfig.ObjectMeta.Name)
		if err != nil {
			return fmt.Errorf("failed to compute the bootstrap kubeconfig: %v", err)
		}
	}

	gardenNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: v1beta1constants.GardenNamespace}}
	if err := g.seedClient.Client().Create(ctx, gardenNamespace); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	values := g.computeGardenletChartValues(bootstrapKubeconfig)
	applier := applier.NewGardenletChartApplier(g.seedClient.ChartApplier(), g.seedClient.Client(), values, GetChartPath())
	if err := applier.Deploy(ctx); err != nil {
		return fmt.Errorf("failed deploying Gardenlet chart to the Seed cluster: %v", err)
	}

	g.log.Infof("Successfully deployed Gardenlet resources for Seed %q", g.Imports.ComponentConfiguration.SeedConfig.Name)

	return g.waitForRolloutToBeComplete(ctx)
}

func (g Landscaper) computeGardenletChartValues(bootstrapKubeconfig []byte) map[string]interface{} {
	gardenClientConnection := map[string]interface{}{
		"acceptContentTypes": g.Imports.ComponentConfiguration.GardenClientConnection.AcceptContentTypes,
		"contentType":        g.Imports.ComponentConfiguration.GardenClientConnection.ContentType,
		"qps":                g.Imports.ComponentConfiguration.GardenClientConnection.QPS,
		"burst":              g.Imports.ComponentConfiguration.GardenClientConnection.Burst,
		"kubeconfigSecret":   *g.Imports.ComponentConfiguration.GardenClientConnection.KubeconfigSecret,
	}

	if len(bootstrapKubeconfig) > 0 {
		gardenClientConnection["bootstrapKubeconfig"] = map[string]interface{}{
			"name":       g.Imports.ComponentConfiguration.GardenClientConnection.BootstrapKubeconfig.Name,
			"namespace":  g.Imports.ComponentConfiguration.GardenClientConnection.BootstrapKubeconfig.Namespace,
			"kubeconfig": string(bootstrapKubeconfig),
		}
	}

	if g.Imports.ComponentConfiguration.GardenClientConnection.GardenClusterAddress != nil {
		gardenClientConnection["gardenClusterAddress"] = *g.Imports.ComponentConfiguration.GardenClientConnection.GardenClusterAddress
	}

	configValues := map[string]interface{}{
		"gardenClientConnection": gardenClientConnection,
		"featureGates":           g.Imports.ComponentConfiguration.FeatureGates,
		"server":                 *g.Imports.ComponentConfiguration.Server,
		"seedConfig":             *g.Imports.ComponentConfiguration.SeedConfig,
	}

	if g.Imports.ComponentConfiguration.ShootClientConnection != nil {
		configValues["shootClientConnection"] = *g.Imports.ComponentConfiguration.ShootClientConnection
	}

	if g.Imports.ComponentConfiguration.SeedClientConnection != nil {
		configValues["seedClientConnection"] = *g.Imports.ComponentConfiguration.SeedClientConnection
	}

	if g.Imports.ComponentConfiguration.Controllers != nil {
		configValues["controllers"] = *g.Imports.ComponentConfiguration.Controllers
	}

	if g.Imports.ComponentConfiguration.LeaderElection != nil {
		configValues["leaderElection"] = *g.Imports.ComponentConfiguration.LeaderElection
	}

	if g.Imports.ComponentConfiguration.LogLevel != nil {
		configValues["logLevel"] = *g.Imports.ComponentConfiguration.LogLevel
	}

	if g.Imports.ComponentConfiguration.KubernetesLogLevel != nil {
		configValues["kubernetesLogLevel"] = *g.Imports.ComponentConfiguration.KubernetesLogLevel
	}

	if g.Imports.ComponentConfiguration.Logging != nil {
		configValues["logging"] = *g.Imports.ComponentConfiguration.Logging
	}

	gardenletValues := map[string]interface{}{
		"replicaCount":       1,
		"serviceAccountName": "gardenlet",
		"image": map[string]interface{}{
			"repository": g.gardenletImageRepository,
			"tag":        g.gardenletImageTag,
		},
		"config": configValues,
	}

	if g.Imports.ImageVectorOverwrite != nil {
		gardenletValues["imageVectorOverwrite"] = *g.Imports.ImageVectorOverwrite
	}

	if g.Imports.ComponentImageVectorOverwrites != nil {
		gardenletValues["componentImageVectorOverwrites"] = *g.Imports.ComponentImageVectorOverwrites
	}

	if g.Imports.Resources != nil {
		gardenletValues["resources"] = *g.Imports.Resources
	}

	return map[string]interface{}{
		"global": map[string]interface{}{
			"gardenlet": gardenletValues,
		},
	}
}

// Sleep returns the time the process should sleep to give the gardenlet process
// time to startup and either fail or proceed
// exposed for testing
var Sleep = func() time.Duration { return 10 * time.Second }

func (g *Landscaper) waitForRolloutToBeComplete(ctx context.Context) error {
	g.log.Info("Waiting for the Gardenlet to be rolled out successfully...")

	// sleep for couple seconds to give the gardenlet process time to startup and either fail or proceed
	time.Sleep(Sleep())

	var (
		deploymentRolloutSuccessful bool
		seedIsRegistered            bool
	)

	return retry.UntilTimeout(ctx, 10*time.Second, 15*time.Minute, func(ctx context.Context) (done bool, err error) {
		gardenletDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gardenlet",
				Namespace: v1beta1constants.GardenNamespace,
			},
		}

		err = g.seedClient.Client().Get(ctx, client.ObjectKey{Namespace: v1beta1constants.GardenNamespace, Name: v1beta1constants.DeploymentNameGardenlet}, gardenletDeployment)
		if err != nil {
			return retry.SevereError(err)
		}

		if err := health.CheckDeployment(gardenletDeployment); err != nil {
			msg := fmt.Sprintf("gardenlet deployment is not rolled out successfuly yet...: %v", err)
			g.log.Info(msg)
			return retry.MinorError(fmt.Errorf(msg))
		}

		if !deploymentRolloutSuccessful {
			g.log.Info("Gardenlet deployment rollout successful!")
			deploymentRolloutSuccessful = true
		}

		seed := &gardencorev1beta1.Seed{
			ObjectMeta: metav1.ObjectMeta{
				Name: g.Imports.ComponentConfiguration.SeedConfig.Name,
			},
		}

		err = g.gardenClient.Client().Get(ctx, kutil.KeyFromObject(seed), seed)
		if err != nil {
			if apierrors.IsNotFound(err) {
				msg := fmt.Sprintf("seed %q is not yet registered...: %v", seed.Name, err)
				g.log.Info(msg)
				return retry.MinorError(fmt.Errorf(msg))
			}
			return retry.SevereError(err)
		}

		if !seedIsRegistered {
			g.log.Infof("Seed %q is registered!", seed.Name)
			seedIsRegistered = true
		}

		// Check Seed without caring for the observing Gardener version (unknown to the landscaper)
		err = health.CheckSeed(seed, seed.Status.Gardener)
		if err != nil {
			msg := fmt.Sprintf("seed %q is not yet ready...: %v", seed.Name, err)
			g.log.Info(msg)
			return retry.MinorError(fmt.Errorf(msg))
		}
		g.log.Info("Seed is bootstrapped and ready!")
		return retry.Ok()
	})
}

func (g *Landscaper) getKubeconfigWithBootstrapToken(ctx context.Context, seedName string) ([]byte, error) {
	var (
		tokenID  = utils.ComputeSHA256Hex([]byte(seedName))[:6]
		validity = 24 * time.Hour
	)
	return bootstrap.ComputeKubeconfigWithBootstrapToken(ctx, g.gardenClient.Client(), g.gardenClient.RESTConfig(), tokenID, fmt.Sprintf("A bootstrap token for the Gardenlet for seed %q.", seedName), validity)
}

func (g *Landscaper) isSeedBootstrapped(ctx context.Context, seedMeta metav1.ObjectMeta) (bool, error) {
	seed := &gardencorev1beta1.Seed{ObjectMeta: seedMeta}
	if err := g.gardenClient.Client().Get(ctx, kutil.KeyFromObject(seed), seed); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return seed.Status.KubernetesVersion != nil, nil
}

func (g *Landscaper) deploySeedSecret(ctx context.Context, runtimeClusterKubeconfig []byte, secretRef *corev1.SecretReference) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRef.Name,
			Namespace: secretRef.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, g.gardenClient.Client(), secret, func() error {
		secret.Data = map[string][]byte{
			kubernetes.KubeConfig: runtimeClusterKubeconfig,
		}
		secret.Type = corev1.SecretTypeOpaque
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (g *Landscaper) deployBackupSecret(ctx context.Context, providerName string, credentials map[string][]byte, backupSecretRef corev1.SecretReference) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupSecretRef.Name,
			Namespace: backupSecretRef.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, g.gardenClient.Client(), secret, func() error {
		secret.Data = credentials
		secret.Type = corev1.SecretTypeOpaque
		secret.Labels = map[string]string{
			"provider": providerName,
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}
