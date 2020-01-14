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

package framework_test

import (
	"github.com/gardener/gardener/test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test Descriptions tests", func() {

	DescribeTable("define test labels",
		func(td framework.TestDescription, expected string) {
			Expect(td.String()).To(Equal(expected))
		},
		Entry("beta default - beta default", framework.TestDescription{}.Beta().Default(), "[BETA] [DEFAULT]"),
		Entry("serial beta - beta serial", framework.TestDescription{}.Serial().Beta(), "[BETA] [SERIAL]"),
		Entry("serial beta release - beta release serial", framework.TestDescription{}.Serial().Beta().Release(), "[BETA] [RELEASE] [SERIAL]"),
		Entry("serial beta beta - beta serial", framework.TestDescription{}.Serial().Beta().Beta(), "[BETA] [SERIAL]"),
	)

})
