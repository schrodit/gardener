// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package applier_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cr "github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/applier"
	appliercommon "github.com/gardener/gardener/pkg/landscaper/gardenlet/applier/common"
	"github.com/gardener/gardener/pkg/operation/botanist/component"
)

var _ = Describe("#Garden RBAC", func() {
	var (
		ctx          context.Context
		c            client.Client
		crd          component.Deployer
		chartApplier kubernetes.ChartApplier
	)

	BeforeEach(func() {
		ctx = context.TODO()

		s := runtime.NewScheme()
		Expect(rbacv1.AddToScheme(s)).NotTo(HaveOccurred())

		c = fake.NewFakeClientWithScheme(s)

		mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{rbacv1.SchemeGroupVersion})
		mapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"), meta.RESTScopeRoot)
		mapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRoleBinding"), meta.RESTScopeRoot)

		renderer := cr.NewWithServerVersion(&version.Info{})

		chartApplier = kubernetes.NewChartApplier(renderer, kubernetes.NewApplier(c, mapper))
		Expect(chartApplier).NotTo(BeNil(), "should return chart applier")
	})

	It("it should verify that all RBAC resources are applied", func() {
		crd = applier.NewGardenRBACApplier(chartApplier, chartsRootPath)

		Expect(crd.Deploy(ctx)).ToNot(HaveOccurred(), "RBAC deploy succeeds")

		appliercommon.ValidateGardenletRBACChartResources(ctx, c)
	})

	It("it should verify that RBAC resources are not deleted during the destroy() operation", func() {
		crd = applier.NewGardenRBACApplier(chartApplier, chartsRootPath)

		Expect(crd.Deploy(ctx)).ToNot(HaveOccurred(), "RBAC deploy succeeds")
		Expect(crd.Destroy(ctx)).ToNot(HaveOccurred(), "RBAC destroy succeeds")

		appliercommon.ValidateGardenletRBACChartResources(ctx, c)
	})
})
