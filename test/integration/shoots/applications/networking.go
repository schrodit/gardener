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

/**
	Overview
		- Tests the communication between all nodes of the shoot

	AfterSuite
		- Cleanup Workload in Shoot

	Test: Create a nginx daemonset and test if it is reachable from each node.
	Expected Output
		- nginx's are reachable from each node
 **/

package applications

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/test/framework"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"io/ioutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"text/template"
	"time"

	"github.com/onsi/ginkgo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	networkTestTimeout = 1800 * time.Second
)

const (
	nginxTemplateName = "network-nginx-deamonset.yaml.tpl"
)

var _ = ginkgo.Describe("Shoot network testing", func() {

	f := framework.NewShootFramework(&framework.ShootConfig{
		CreateTestNamespace: true,
	})

	var (
		name = "net-test"
	)

	f.Beta().CIt("should reach all webservers on all nodes", func(ctx context.Context) {
		templateFilepath := filepath.Join(f.TemplatesDir, nginxTemplateName)
		nettestTmpl, err := template.ParseFiles(templateFilepath)
		framework.ExpectNoError(err)

		ginkgo.By("Deploy the net test daemon set")
		var writer bytes.Buffer
		err = nettestTmpl.Execute(&writer, map[string]string{
			"name":      name,
			"namespace": f.Namespace,
		})
		framework.ExpectNoError(err)

		manifestReader := kubernetes.NewManifestReader(writer.Bytes())
		err = f.ShootClient.Applier().ApplyManifest(ctx, manifestReader, kubernetes.DefaultApplierOptions)
		framework.ExpectNoError(err)

		defer func() {
			ginkgo.By("cleanup network test daemonset")
			err := f.ShootClient.Client().Delete(ctx, &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: f.Namespace}})
			if err != nil {
				if !apierrors.IsNotFound(err) {
					framework.ExpectNoError(err)
				}
			}
		}()

		err = f.WaitUntilDaemonSetIsRunning(ctx, f.ShootClient.Client(), name, f.Namespace)
		framework.ExpectNoError(err)

		pods := &corev1.PodList{}
		err = f.ShootClient.Client().List(ctx, pods, client.InNamespace(f.Namespace), client.MatchingLabels{"app": "net-nginx"})
		framework.ExpectNoError(err)

		// check if all webservers can be reached from all nodes
		ginkgo.By("test connectivity to webservers")
		shootRESTConfig := f.ShootClient.RESTConfig()
		var res error
		for _, from := range pods.Items {
			for _, to := range pods.Items {
				ginkgo.By(fmt.Sprintf("Testing %s to %s", from.GetName(), to.GetName()))
				reader, err := kubernetes.NewPodExecutor(shootRESTConfig).Execute(ctx, from.Namespace, from.Name, "net-curl", fmt.Sprintf("curl -L %s:80 --fail -m 10", to.Status.PodIP))
				if err != nil {
					res = multierror.Append(res, errors.Wrapf(err, "%s to %s", from.GetName(), to.GetName()))
					continue
				}
				data, err := ioutil.ReadAll(reader)
				if err != nil {
					f.Logger.Error(err)
					continue
				}
				f.Logger.Infof("%s to %s: %s", from.GetName(), to.GetName(), data)
			}
		}
		framework.ExpectNoError(err)
	}, networkTestTimeout)

})
