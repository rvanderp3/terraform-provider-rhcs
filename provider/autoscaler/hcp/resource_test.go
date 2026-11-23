/*
Copyright (c) 2024 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hcp

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	. "github.com/onsi/ginkgo/v2" // nolint
	. "github.com/onsi/gomega"    // nolint
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

const clusterId = "123"

var _ = Describe("Cluster Autoscaler", func() {
	Context("conversion to terraform state", func() {
		It("successfully populates all fields", func() {
			autoscalerSpec, err := cmv1.NewClusterAutoscaler().
				MaxPodGracePeriod(10).
				PodPriorityThreshold(-10).
				MaxNodeProvisionTime("1h").
				ResourceLimits(cmv1.NewAutoscalerResourceLimits().MaxNodesTotal(10)).Build()
			Expect(err).ToNot(HaveOccurred())

			var state ClusterAutoscalerState
			Expect(populateAutoscalerState(autoscalerSpec, clusterId, &state)).To(Succeed())
			Expect(state).To(Equal(ClusterAutoscalerState{
				Cluster:              types.StringValue(clusterId),
				MaxPodGracePeriod:    types.Int64Value(10),
				PodPriorityThreshold: types.Int64Value(-10),
				MaxNodeProvisionTime: types.StringValue("1h"),
				ResourceLimits: types.ObjectValueMust(resourceLimitsAttributeTypes, map[string]attr.Value{
					"max_nodes_total": types.Int64Value(10),
				}),
			}))
		})

		It("clears fields omitted by the API on refresh", func() {
			autoscaler, err := cmv1.NewClusterAutoscaler().Build()
			Expect(err).ToNot(HaveOccurred())
			state := ClusterAutoscalerState{
				ResourceLimits: types.ObjectValueMust(resourceLimitsAttributeTypes, map[string]attr.Value{
					"max_nodes_total": types.Int64Value(10),
				}),
			}
			Expect(populateAutoscalerState(autoscaler, clusterId, &state)).To(Succeed())
			Expect(state).To(Equal(ClusterAutoscalerState{
				Cluster:              types.StringValue(clusterId),
				MaxPodGracePeriod:    types.Int64Null(),
				PodPriorityThreshold: types.Int64Null(),
				MaxNodeProvisionTime: types.StringNull(),
				ResourceLimits:       types.ObjectNull(resourceLimitsAttributeTypes),
			}))
		})
	})

	Context("conversion to an OCM API object", func() {
		It("successfully converts all fields from a terraform state", func() {
			state := ClusterAutoscalerState{
				Cluster:              types.StringValue(clusterId),
				MaxPodGracePeriod:    types.Int64Value(10),
				PodPriorityThreshold: types.Int64Value(-10),
				MaxNodeProvisionTime: types.StringValue("1h"),
				ResourceLimits: types.ObjectValueMust(resourceLimitsAttributeTypes, map[string]attr.Value{
					"max_nodes_total": types.Int64Value(10),
				}),
			}
			autoscaler, err := clusterAutoscalerStateToObject(&state)
			Expect(err).ToNot(HaveOccurred())
			expected, err := cmv1.NewClusterAutoscaler().
				MaxPodGracePeriod(10).
				PodPriorityThreshold(-10).
				MaxNodeProvisionTime("1h").
				ResourceLimits(cmv1.NewAutoscalerResourceLimits().MaxNodesTotal(10)).Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(autoscaler).To(Equal(expected))
		})

		DescribeTable("omits unset fields instead of sending zero values", func(state ClusterAutoscalerState) {
			autoscaler, err := clusterAutoscalerStateToObject(&state)
			Expect(err).ToNot(HaveOccurred())
			expected, err := cmv1.NewClusterAutoscaler().Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(autoscaler).To(Equal(expected))
		},
			Entry("null values", ClusterAutoscalerState{
				MaxPodGracePeriod:    types.Int64Null(),
				PodPriorityThreshold: types.Int64Null(),
				MaxNodeProvisionTime: types.StringNull(),
				ResourceLimits:       types.ObjectNull(resourceLimitsAttributeTypes),
			}),
			Entry("unknown values", ClusterAutoscalerState{
				MaxPodGracePeriod:    types.Int64Unknown(),
				PodPriorityThreshold: types.Int64Unknown(),
				MaxNodeProvisionTime: types.StringUnknown(),
				ResourceLimits:       types.ObjectUnknown(resourceLimitsAttributeTypes),
			}),
		)

		It("preserves explicitly configured zero values", func() {
			autoscaler, err := clusterAutoscalerStateToObject(&ClusterAutoscalerState{
				MaxPodGracePeriod:    types.Int64Value(0),
				PodPriorityThreshold: types.Int64Value(0),
				MaxNodeProvisionTime: types.StringNull(),
				ResourceLimits: types.ObjectValueMust(resourceLimitsAttributeTypes, map[string]attr.Value{
					"max_nodes_total": types.Int64Value(0),
				}),
			})
			Expect(err).ToNot(HaveOccurred())
			_, exists := autoscaler.GetMaxPodGracePeriod()
			Expect(exists).To(BeTrue())
			_, exists = autoscaler.GetPodPriorityThreshold()
			Expect(exists).To(BeTrue())
			_, exists = autoscaler.ResourceLimits().GetMaxNodesTotal()
			Expect(exists).To(BeTrue())
		})
	})
})
