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
)

type ClusterAutoscalerState struct {
	Cluster                types.String `tfsdk:"cluster"`
	MaxPodGracePeriod      types.Int64  `tfsdk:"max_pod_grace_period"`
	PodPriorityThreshold   types.Int64  `tfsdk:"pod_priority_threshold"`
	MaxNodeProvisionTime   types.String `tfsdk:"max_node_provision_time"`
	ResourceLimits         types.Object `tfsdk:"resource_limits"`
	BalancingIgnoredLabels types.List   `tfsdk:"balancing_ignored_labels"`
	Expanders              types.List   `tfsdk:"expanders"`
	ScaleDown              types.Object `tfsdk:"scale_down"`
}

const (
	maxNodesTotalAttribute        = "max_nodes_total"
	enabledAttribute              = "enabled"
	unneededTimeAttribute         = "unneeded_time"
	utilizationThresholdAttribute = "utilization_threshold"
	delayAfterAddAttribute        = "delay_after_add"
	delayAfterDeleteAttribute     = "delay_after_delete"
	delayAfterFailureAttribute    = "delay_after_failure"
)

var scaleDownAttributeTypes = map[string]attr.Type{
	enabledAttribute:              types.BoolType,
	unneededTimeAttribute:         types.StringType,
	utilizationThresholdAttribute: types.StringType,
	delayAfterAddAttribute:        types.StringType,
	delayAfterDeleteAttribute:     types.StringType,
	delayAfterFailureAttribute:    types.StringType,
}

var resourceLimitsAttributeTypes = map[string]attr.Type{
	maxNodesTotalAttribute: types.Int64Type,
}
