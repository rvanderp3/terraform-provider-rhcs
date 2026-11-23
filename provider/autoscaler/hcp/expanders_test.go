// Copyright Red Hat
// SPDX-License-Identifier: Apache-2.0

package hcp

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	. "github.com/onsi/ginkgo/v2" // nolint
	. "github.com/onsi/gomega"    // nolint
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

var _ = Describe("HCP expander mapping", func() {
	It("preserves expander order and casing even with scale-down disabled", func() {
		object, err := cmv1.NewClusterAutoscaler().Expanders("Priority", "LeastWaste", "Random").
			ScaleDown(cmv1.NewAutoscalerScaleDownConfig().Enabled(false)).Build()
		Expect(err).NotTo(HaveOccurred())
		var state ClusterAutoscalerState
		Expect(populateAutoscalerState(object, clusterId, &state)).To(Succeed())
		Expect(state.Expanders).To(Equal(types.ListValueMust(types.StringType, []attr.Value{
			types.StringValue("Priority"), types.StringValue("LeastWaste"), types.StringValue("Random"),
		})))
		result, err := clusterAutoscalerStateToObject(&state)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(object))

		By("clearing stale expanders when the API omits them")
		empty, err := cmv1.NewClusterAutoscaler().Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(populateAutoscalerState(empty, clusterId, &state)).To(Succeed())
		Expect(state.Expanders).To(Equal(types.ListNull(types.StringType)))
	})

	DescribeTable("omits unset expanders from requests", func(expanders types.List) {
		object, err := clusterAutoscalerStateToObject(&ClusterAutoscalerState{Expanders: expanders})
		Expect(err).NotTo(HaveOccurred())
		_, exists := object.GetExpanders()
		Expect(exists).To(BeFalse())
	},
		Entry("null", types.ListNull(types.StringType)),
		Entry("unknown", types.ListUnknown(types.StringType)),
	)
})
