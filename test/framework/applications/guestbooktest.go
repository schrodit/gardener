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

package applications

import (
	"context"
	"fmt"
	"github.com/gardener/gardener/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/retry"
	framework "github.com/gardener/gardener/test/framework"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

const (
	// GuestBook is the name of the guestbook k8s resources
	GuestBook = "guestbook"

	// RedisMaster is the name of the redis master deployed by the helm chart
	RedisMaster = "redis-master"

	guestBookTemplateName = "guestbook-app.yaml.tpl"
	redisChart            = "stable/redis"
	redisChartVersion     = "10.2.1"
)

// GuestBookTest is simple application tests.
// It deploys a guestbook application with a redis backend and a frontend
// that is exposed via an ingress.
type GuestBookTest struct {
	framework *framework.ShootFramework

	guestBookAppHost string
}

// NewGuestBookTest creates a new guestbook application test
// It takes the path to the test resources.
func NewGuestBookTest(f *framework.ShootFramework) (*GuestBookTest, error) {
	allowedCharacters := "0123456789abcdefghijklmnopqrstuvwxyz"
	randURLSuffix, err := utils.GenerateRandomStringFromCharset(3, allowedCharacters)
	if err != nil {
		return nil, err
	}
	return &GuestBookTest{
		framework:        f,
		guestBookAppHost: fmt.Sprintf("guestbook-%s.ingress.%s", randURLSuffix, *f.Shoot.Spec.DNS.Domain),
	}, nil
}

// WaitUntilPrerequisitesAreReady waits until the redis master is ready.
func (t *GuestBookTest) WaitUntilPrerequisitesAreReady(ctx context.Context) {
	err := t.framework.WaitUntilStatefulSetIsRunning(ctx, RedisMaster, t.framework.Namespace, t.framework.ShootClient)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// WaitUntilGuestbookAppIsAvailable waits until the deployed guestbook application can be reached via http
func (t *GuestBookTest) WaitUntilGuestbookAppIsAvailable(ctx context.Context, guestbookAppUrls []string) error {
	defaultPollInterval := time.Minute
	return retry.UntilTimeout(ctx, defaultPollInterval, 5*time.Minute, func(ctx context.Context) (done bool, err error) {
		for _, guestbookAppURL := range guestbookAppUrls {
			response, err := framework.HTTPGet(ctx, guestbookAppURL)
			if err != nil {
				t.framework.Logger.Infof("Guestbook app url: %q is not available yet: %s", guestbookAppURL, err.Error())
				return retry.MinorError(err)
			}

			if response.StatusCode != http.StatusOK {
				t.framework.Logger.Infof("Guestbook app url: %q is not available yet", guestbookAppURL)
				return retry.MinorError(fmt.Errorf("guestbook app url %q returned status %s", guestbookAppURL, response.Status))
			}

			responseBytes, err := ioutil.ReadAll(response.Body)
			if err != nil {
				return retry.SevereError(err)
			}

			bodyString := string(responseBytes)
			if strings.Contains(bodyString, "404") || strings.Contains(bodyString, "503") {
				t.framework.Logger.Infof("Guestbook app is not ready yet")
				return retry.MinorError(fmt.Errorf("guestbook response body contained an error code"))
			}
		}
		t.framework.Logger.Infof("Rejoice, the guestbook app urls are available now!")
		return retry.Ok()
	})
}

// DeployGuestBookApp deploys the redis helmchart and the guestbook app application
func (t *GuestBookTest) DeployGuestBookApp(ctx context.Context) {
	if t.framework.Namespace == "" {
		_, err := t.framework.CreateNewNamespace(ctx)
		framework.ExpectNoError(err)
	}
	shoot := t.framework.Shoot
	if !shoot.Spec.Addons.NginxIngress.Enabled {
		ginkgo.Fail("The test requires .spec.addons.nginxIngress.enabled to be true")
	} else if shoot.Spec.Kubernetes.AllowPrivilegedContainers == nil || !*shoot.Spec.Kubernetes.AllowPrivilegedContainers {
		ginkgo.Fail("The test requires .spec.kubernetes.allowPrivilegedContainers to be true")
	}

	ginkgo.By("Applying redis chart")
	// redis-slaves are not required for test success
	chartOverrides := map[string]interface{}{
		"cluster": map[string]interface{}{
			"enabled": false,
		},
	}
	if shoot.Spec.Provider.Type == "alicloud" {
		// AliCloud requires a minimum of 20 GB for its PVCs
		chartOverrides["master"] = map[string]interface{}{
			"persistence": map[string]interface{}{
				"size": "20Gi",
			}}
	}

	err := t.framework.RenderAndDeployChart(ctx, t.framework.ShootClient, framework.Chart{
		Name:        redisChart,
		ReleaseName: "redis",
		Namespace:   t.framework.Namespace,
		Version:     redisChartVersion,
	}, chartOverrides)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	t.WaitUntilPrerequisitesAreReady(ctx)

	ginkgo.By("Deploy the guestbook application")
	guestBookParams := struct {
		HelmDeployNamespace string
		ShootDNSHost        string
	}{
		t.framework.Namespace,
		t.guestBookAppHost,
	}
	err = t.framework.RenderAndDeployTemplate(ctx, t.framework.ShootClient, guestBookTemplateName, guestBookParams)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("Guestbook app was deployed successfully!")
}

// Test tests that a deployed guestbook application is working correctly
func (t *GuestBookTest) Test(ctx context.Context) {
	shoot := t.framework.Shoot
	// define guestbook app urls
	guestBookAppURL := fmt.Sprintf("http://%s", t.guestBookAppHost)
	pushString := fmt.Sprintf("foobar-%s", shoot.Name)
	pushURL := fmt.Sprintf("%s/rpush/guestbook/%s", guestBookAppURL, pushString)
	pullURL := fmt.Sprintf("%s/lrange/guestbook", guestBookAppURL)

	// Check availability of the guestbook app
	err := t.WaitUntilGuestbookAppIsAvailable(ctx, []string{guestBookAppURL, pushURL, pullURL})
	framework.ExpectNoError(err)

	// Push foobar-<shoot-name> to the guestbook app
	_, err = framework.HTTPGet(ctx, pushURL)
	framework.ExpectNoError(err)

	// Pull foobar
	pullResponse, err := framework.HTTPGet(ctx, pullURL)
	framework.ExpectNoError(err)
	gomega.Expect(pullResponse.StatusCode).To(gomega.Equal(http.StatusOK))

	responseBytes, err := ioutil.ReadAll(pullResponse.Body)
	framework.ExpectNoError(err)

	// test if foobar-<shoot-name> was pulled successfully
	bodyString := string(responseBytes)
	gomega.Expect(bodyString).To(gomega.ContainSubstring(fmt.Sprintf("foobar-%s", shoot.Name)))
}

// Cleanup cleans up all resources depoyed by the guestbook test
func (t *GuestBookTest) Cleanup(ctx context.Context) {
	// Clean up shoot
	ginkgo.By("Cleaning up guestbook app resources")
	deleteResource := func(ctx context.Context, resource runtime.Object) error {
		err := t.framework.ShootClient.Client().Delete(ctx, resource)
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	cleanupGuestbook := func() {
		var (
			guestBookIngressToDelete = &apiextensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: t.framework.Namespace,
					Name:      GuestBook,
				}}

			guestBookDeploymentToDelete = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: t.framework.Namespace,
					Name:      GuestBook,
				},
			}

			guestBookServiceToDelete = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: t.framework.Namespace,
					Name:      GuestBook,
				},
			}
		)

		err := deleteResource(ctx, guestBookIngressToDelete)
		framework.ExpectNoError(err)

		err = deleteResource(ctx, guestBookDeploymentToDelete)
		framework.ExpectNoError(err)

		err = deleteResource(ctx, guestBookServiceToDelete)
		framework.ExpectNoError(err)
	}

	cleanupRedis := func() {
		var (
			redisMasterServiceToDelete = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: t.framework.Namespace,
					Name:      RedisMaster,
				},
			}
			redisMasterStatefulSetToDelete = &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: t.framework.Namespace,
					Name:      RedisMaster,
				},
			}
		)

		err := deleteResource(ctx, redisMasterServiceToDelete)
		framework.ExpectNoError(err)

		err = deleteResource(ctx, redisMasterStatefulSetToDelete)
		framework.ExpectNoError(err)

	}
	cleanupGuestbook()
	cleanupRedis()

	ginkgo.By("redis and the guestbook app have been cleaned up!")
}
