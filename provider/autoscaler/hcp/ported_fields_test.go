// Copyright Red Hat
// SPDX-License-Identifier: Apache-2.0

package hcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	. "github.com/onsi/ginkgo/v2" // nolint
	. "github.com/onsi/gomega"    // nolint
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

var _ = Describe("HCP ported autoscaler mapping", func() {
	It("round trips all supported scale-down fields and labels", func() {
		object, err := cmv1.NewClusterAutoscaler().BalancingIgnoredLabels("custom.label/zone").
			ScaleDown(cmv1.NewAutoscalerScaleDownConfig().Enabled(true).UnneededTime("10m").
				UtilizationThreshold("0.5").DelayAfterAdd("10m").DelayAfterDelete("0s").DelayAfterFailure("3m")).Build()
		Expect(err).NotTo(HaveOccurred())
		var state ClusterAutoscalerState
		Expect(populateAutoscalerState(object, clusterId, &state)).To(Succeed())
		Expect(state.BalancingIgnoredLabels).To(Equal(types.ListValueMust(types.StringType, []attr.Value{types.StringValue("custom.label/zone")})))
		Expect(state.ScaleDown.Attributes()).To(Equal(map[string]attr.Value{
			"enabled": types.BoolValue(true), "unneeded_time": types.StringValue("10m"),
			"utilization_threshold": types.StringValue("0.5"), "delay_after_add": types.StringValue("10m"),
			"delay_after_delete": types.StringValue("0s"), "delay_after_failure": types.StringValue("3m"),
		}))
		result, err := clusterAutoscalerStateToObject(&state)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(object))

		By("clearing stale state when the API omits the new fields")
		empty, err := cmv1.NewClusterAutoscaler().Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(populateAutoscalerState(empty, clusterId, &state)).To(Succeed())
		Expect(state.BalancingIgnoredLabels).To(Equal(types.ListNull(types.StringType)))
		Expect(state.ScaleDown).To(Equal(types.ObjectNull(scaleDownAttributeTypes)))
	})

	It("omits unknown ported values", func() {
		object, err := clusterAutoscalerStateToObject(&ClusterAutoscalerState{
			BalancingIgnoredLabels: types.ListUnknown(types.StringType),
			ScaleDown:              types.ObjectUnknown(scaleDownAttributeTypes),
		})
		Expect(err).NotTo(HaveOccurred())
		expected, err := cmv1.NewClusterAutoscaler().Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(object).To(Equal(expected))
	})

	It("preserves explicit false and omits null or unknown children", func() {
		object, err := clusterAutoscalerStateToObject(&ClusterAutoscalerState{
			ScaleDown: types.ObjectValueMust(scaleDownAttributeTypes, map[string]attr.Value{
				"enabled": types.BoolValue(false), "unneeded_time": types.StringUnknown(),
				"utilization_threshold": types.StringNull(), "delay_after_add": types.StringUnknown(),
				"delay_after_delete": types.StringNull(), "delay_after_failure": types.StringUnknown(),
			}),
		})
		Expect(err).NotTo(HaveOccurred())
		expected, err := cmv1.NewClusterAutoscaler().ScaleDown(cmv1.NewAutoscalerScaleDownConfig().Enabled(false)).Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(object).To(Equal(expected))
	})

	It("does not expose Classic-only attributes or resource limits", func() {
		var response resource.SchemaResponse
		(&ClusterAutoscalerResource{}).Schema(context.Background(), resource.SchemaRequest{}, &response)
		Expect(response.Diagnostics.HasError()).To(BeFalse())
		for _, name := range []string{"log_verbosity", "balance_similar_node_groups", "skip_nodes_with_local_storage", "ignore_daemonsets_utilization"} {
			Expect(response.Schema.Attributes).NotTo(HaveKey(name))
		}
		limits := response.Schema.Attributes["resource_limits"].(schema.SingleNestedAttribute)
		Expect(limits.Attributes).To(HaveLen(1))
		Expect(limits.Attributes).To(HaveKey("max_nodes_total"))
	})
})
