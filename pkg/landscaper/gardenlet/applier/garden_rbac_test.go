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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cr "github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/landscaper/gardenlet/applier"
	"github.com/gardener/gardener/pkg/operation/botanist/component"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
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
		crd = applier.NewGardenRBACApplier(chartApplier, c, chartsRootPath)

		Expect(crd.Deploy(ctx)).ToNot(HaveOccurred(), "RBAC deploy succeeds")

		expectRBACResourcesAreCreated(ctx, c)
	})

	It("it should verify that RBAC resources are not deleted during the destroy() operation", func() {
		crd = applier.NewGardenRBACApplier(chartApplier, c, chartsRootPath)

		Expect(crd.Deploy(ctx)).ToNot(HaveOccurred(), "RBAC deploy succeeds")
		Expect(crd.Destroy(ctx)).ToNot(HaveOccurred(), "RBAC destroy succeeds")

		expectRBACResourcesAreCreated(ctx, c)
	})
})

func expectRBACResourcesAreCreated(ctx context.Context, c client.Client) {
	systemSeedRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gardener.cloud:system:seeds",
		},
	}
	expectedSystemSeedRole := *systemSeedRole
	expectedSystemSeedRole.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	}

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(systemSeedRole),
		systemSeedRole,
	)).ToNot(HaveOccurred())
	Expect(systemSeedRole.Rules).To(Equal(expectedSystemSeedRole.Rules))

	systemSeedBootstrapperRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gardener.cloud:system:seed-bootstrapper",
		},
	}
	expectedSystemSeedBootstrapperRole := *systemSeedBootstrapperRole
	expectedSystemSeedBootstrapperRole.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{"certificates.k8s.io"},
			Resources: []string{"certificatesigningrequests"},
			Verbs:     []string{"create", "get", "list", "watch"},
		},
		{
			APIGroups: []string{"certificates.k8s.io"},
			Resources: []string{"certificatesigningrequests/seedclient"},
			Verbs:     []string{"create"},
		},
	}

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(systemSeedBootstrapperRole),
		systemSeedBootstrapperRole,
	)).ToNot(HaveOccurred())
	Expect(systemSeedBootstrapperRole.Rules).To(Equal(expectedSystemSeedBootstrapperRole.Rules))

	systemSeedsRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gardener.cloud:system:seeds",
		},
	}
	expectedSystemSeedsRoleBinding := *systemSeedsRoleBinding
	expectedSystemSeedsRoleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "gardener.cloud:system:seeds",
	}
	expectedSystemSeedsRoleBinding.Subjects = []rbacv1.Subject{{
		Kind:     "Group",
		APIGroup: "rbac.authorization.k8s.io",
		Name:     "gardener.cloud:system:seeds",
	}}

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(systemSeedsRoleBinding),
		systemSeedsRoleBinding,
	)).ToNot(HaveOccurred())
	Expect(systemSeedsRoleBinding.RoleRef).To(Equal(expectedSystemSeedsRoleBinding.RoleRef))
	Expect(systemSeedsRoleBinding.Subjects).To(Equal(expectedSystemSeedsRoleBinding.Subjects))

	systemSeedsBootstrapperRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gardener.cloud:system:seed-bootstrapper",
		},
	}
	expectedSystemSeedsBootstrapperRoleBinding := *systemSeedsBootstrapperRoleBinding
	expectedSystemSeedsBootstrapperRoleBinding.RoleRef = rbacv1.RoleRef{
		Kind:     "ClusterRole",
		APIGroup: "rbac.authorization.k8s.io",
		Name:     "gardener.cloud:system:seed-bootstrapper",
	}
	expectedSystemSeedsBootstrapperRoleBinding.Subjects = []rbacv1.Subject{{
		Kind:     "Group",
		Name:     "system:bootstrappers",
		APIGroup: "rbac.authorization.k8s.io",
	}}

	Expect(c.Get(
		ctx,
		kutil.KeyFromObject(systemSeedsBootstrapperRoleBinding),
		systemSeedsBootstrapperRoleBinding,
	)).ToNot(HaveOccurred())
	Expect(systemSeedsBootstrapperRoleBinding.RoleRef).To(Equal(expectedSystemSeedsBootstrapperRoleBinding.RoleRef))
	Expect(systemSeedsBootstrapperRoleBinding.Subjects).To(Equal(expectedSystemSeedsBootstrapperRoleBinding.Subjects))
}
