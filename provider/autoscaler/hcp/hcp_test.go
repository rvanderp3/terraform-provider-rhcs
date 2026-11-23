// Copyright Red Hat
// SPDX-License-Identifier: Apache-2.0

package hcp

import (
	"testing"

	. "github.com/onsi/ginkgo/v2" // nolint
	. "github.com/onsi/gomega"    // nolint
)

func TestHCPAutoscaler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HCP Autoscaler")
}
