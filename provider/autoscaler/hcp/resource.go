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
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"

	"github.com/terraform-redhat/terraform-provider-rhcs/provider/autoscaler"
	"github.com/terraform-redhat/terraform-provider-rhcs/provider/common"
)

type ClusterAutoscalerResourceType struct {
}

type ClusterAutoscalerResource struct {
	collection  *cmv1.ClustersClient
	clusterWait common.ClusterWait
}

func New() resource.Resource {
	return &ClusterAutoscalerResource{}
}

var _ resource.Resource = &ClusterAutoscalerResource{}
var _ resource.ResourceWithImportState = &ClusterAutoscalerResource{}
var _ resource.ResourceWithConfigure = &ClusterAutoscalerResource{}
var _ resource.ResourceWithModifyPlan = &ClusterAutoscalerResource{}

func (r *ClusterAutoscalerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hcp_cluster_autoscaler"
}

func (r *ClusterAutoscalerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `Cluster-wide autoscaling configuration.`,
		Attributes: map[string]schema.Attribute{
			"cluster": schema.StringAttribute{
				Description: "Identifier of the cluster." + common.ValueCannotBeChangedStringDescription,
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`.*\S.*`), "cluster ID may not be empty/blank string"),
				},
			},
			"max_pod_grace_period": schema.Int64Attribute{
				Description: "Gives pods graceful termination time before scaling down.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"pod_priority_threshold": schema.Int64Attribute{
				Description: "To allow users to schedule 'best-effort' pods, which shouldn't trigger " +
					"Cluster Autoscaler actions, but only run when there are spare resources available.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"max_node_provision_time": schema.StringAttribute{
				Description: "Maximum time cluster-autoscaler waits for node to be provisioned.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{autoscaler.PositiveDurationStringValidator("max node provision time validation")},
			},
			"balancing_ignored_labels": schema.ListAttribute{
				Description: "Labels that cluster autoscaler should ignore when considering node group similarity. " +
					"Requires HCP cluster version 4.20 or newer.",
				ElementType:   types.StringType,
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.List{listplanmodifier.UseNonNullStateForUnknown()},
				Validators:    []validator.List{listvalidator.SizeAtMost(64)},
			},
			"scale_down": schema.SingleNestedAttribute{
				Description: "Configuration of scale down operation. Requires HCP cluster version 4.20 or newer.",
				Optional:    true,
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					enabledAttribute: schema.BoolAttribute{
						Description: "Should cluster-autoscaler scale down the cluster. " +
							"When false, omit the other scale_down settings from configuration.",
						Optional:      true,
						Computed:      true,
						PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
					},
					unneededTimeAttribute: schema.StringAttribute{
						Description:   "How long a node should be unneeded before it is eligible for scale down.",
						Optional:      true,
						Computed:      true,
						PlanModifiers: []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators:    []validator.String{autoscaler.PositiveDurationStringValidator("unneeded time to scale down")},
					},
					utilizationThresholdAttribute: schema.StringAttribute{
						Description: "Node utilization level, defined as sum of requested resources divided " +
							"by capacity, below which a node can be considered for scale down.",
						Optional:      true,
						Computed:      true,
						PlanModifiers: []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators: []validator.String{
							autoscaler.StringFloatRangeValidator("utilization threshold validation", 0.0, 1.0),
						},
					},
					delayAfterAddAttribute: schema.StringAttribute{
						Description:   "How long after scale up that scale down evaluation resumes.",
						Optional:      true,
						Computed:      true,
						PlanModifiers: []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators:    []validator.String{autoscaler.PositiveDurationStringValidator("delay after add validation")},
					},
					delayAfterDeleteAttribute: schema.StringAttribute{
						Description:   "How long after node deletion that scale down evaluation resumes.",
						Optional:      true,
						Computed:      true,
						PlanModifiers: []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators:    []validator.String{autoscaler.PositiveDurationStringValidator("delay after delete validation")},
					},
					delayAfterFailureAttribute: schema.StringAttribute{
						Description:   "How long after scale down failure that scale down evaluation resumes.",
						Optional:      true,
						Computed:      true,
						PlanModifiers: []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators:    []validator.String{autoscaler.PositiveDurationStringValidator("delay after failure validation")},
					},
				},
			},
			"resource_limits": schema.SingleNestedAttribute{
				Description: "Constraints of autoscaling resources.",
				Optional:    true,
				Computed:    true,
				// Copying the whole prior object could freeze a null child that the API defaults on update.
				Attributes: map[string]schema.Attribute{
					maxNodesTotalAttribute: schema.Int64Attribute{
						Description: "Maximum number of nodes in all node groups. Cluster autoscaler will " +
							"not grow the cluster beyond this number.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseNonNullStateForUnknown(),
						},
					},
				},
			},
		},
	}
	return
}

func (r *ClusterAutoscalerResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var scaleDown types.Object
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("scale_down"), &scaleDown)...)
	if resp.Diagnostics.HasError() || scaleDown.IsNull() || scaleDown.IsUnknown() {
		return
	}
	var config types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("scale_down"), &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateScaleDownConfig(config, scaleDown.Attributes()[enabledAttribute].(types.Bool))...)
}

func (r *ClusterAutoscalerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	connection, ok := req.ProviderData.(*sdk.Connection)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *sdk.Connaction, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.collection = connection.ClustersMgmt().V1().Clusters()
	r.clusterWait = common.NewClusterWait(r.collection, connection)
}

func (r *ClusterAutoscalerResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	plan := &ClusterAutoscalerState{}
	diags := request.Plan.Get(ctx, plan)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	// Wait till the cluster is ready:
	_, err := r.clusterWait.WaitForClusterToBeReady(ctx, plan.Cluster.ValueString(), 60)
	if err != nil {
		response.Diagnostics.AddError(
			"Cannot poll cluster state",
			fmt.Sprintf(
				"Cannot poll state of cluster with identifier '%s': %v",
				plan.Cluster.ValueString(), err,
			),
		)
		return
	}

	autoscaler, err := clusterAutoscalerStateToObject(plan)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed building cluster autoscaler",
			fmt.Sprintf(
				"Failed building autoscaler for cluster '%s': %v",
				plan.Cluster.ValueString(), err,
			),
		)
		return
	}

	update, err := r.collection.Cluster(plan.Cluster.ValueString()).
		Autoscaler().Update().Body(autoscaler).SendContext(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed creating cluster autoscaler",
			fmt.Sprintf(
				"Failed creating autoscaler for cluster '%s': %v",
				plan.Cluster.ValueString(), err,
			),
		)
		return
	}

	object := update.Body()
	state := &ClusterAutoscalerState{BalancingIgnoredLabels: plan.BalancingIgnoredLabels}
	if err := populateAutoscalerState(object, plan.Cluster.ValueString(), state); err != nil {
		response.Diagnostics.AddError("Failed reading cluster autoscaler", err.Error())
		return
	}

	diags = response.State.Set(ctx, state)
	response.Diagnostics.Append(diags...)
}

func (r *ClusterAutoscalerResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	state := &ClusterAutoscalerState{}
	diags := request.State.Get(ctx, state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	getResponse, err := r.collection.Cluster(state.Cluster.ValueString()).Autoscaler().Get().SendContext(ctx)
	if err != nil && getResponse.Status() == http.StatusNotFound {
		tflog.Warn(ctx, fmt.Sprintf("autoscaler for cluster (%s) not found, removing from state",
			state.Cluster.ValueString(),
		))
		response.State.RemoveResource(ctx)
		return

	} else if err != nil {
		response.Diagnostics.AddError(
			"Failed getting cluster autoscaler",
			fmt.Sprintf(
				"Failed getting autoscaler for cluster '%s': %v",
				state.Cluster.ValueString(), err,
			),
		)
		return
	}

	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	if err := populateAutoscalerState(getResponse.Body(), state.Cluster.ValueString(), state); err != nil {
		response.Diagnostics.AddError("Failed reading cluster autoscaler", err.Error())
		return
	}
	diags = response.State.Set(ctx, state)
	response.Diagnostics.Append(diags...)
}

func (r *ClusterAutoscalerResource) Update(ctx context.Context, request resource.UpdateRequest,
	response *resource.UpdateResponse) {
	var diags diag.Diagnostics

	// Get the state:
	state := &ClusterAutoscalerState{}
	diags = request.State.Get(ctx, state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	// Get the plan:
	plan := &ClusterAutoscalerState{}
	diags = request.Plan.Get(ctx, plan)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	diags = validateNoImmutableAttChange(state, plan)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	_, err := r.collection.Cluster(plan.Cluster.ValueString()).Autoscaler().Get().SendContext(ctx)

	if err != nil {
		response.Diagnostics.AddError(
			"Failed getting cluster autoscaler",
			fmt.Sprintf(
				"Failed getting autoscaler for cluster '%s': %v",
				plan.Cluster.ValueString(), err,
			),
		)
		return
	}

	autoscaler, err := clusterAutoscalerStateToObject(plan)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed updating cluster autoscaler",
			fmt.Sprintf(
				"Failed updating autoscaler for cluster '%s: %v ",
				plan.Cluster.ValueString(), err,
			),
		)
		return
	}

	update, err := r.collection.Cluster(plan.Cluster.ValueString()).
		Autoscaler().Update().Body(autoscaler).SendContext(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed updating cluster autoscaler",
			fmt.Sprintf(
				"Failed updating autoscaler for cluster '%s': %v",
				plan.Cluster.ValueString(), err,
			),
		)
		return
	}

	object := update.Body()
	state = &ClusterAutoscalerState{BalancingIgnoredLabels: plan.BalancingIgnoredLabels}
	if err := populateAutoscalerState(object, plan.Cluster.ValueString(), state); err != nil {
		response.Diagnostics.AddError("Failed reading cluster autoscaler", err.Error())
		return
	}

	diags = response.State.Set(ctx, state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}

func validateNoImmutableAttChange(state, plan *ClusterAutoscalerState) diag.Diagnostics {
	diags := diag.Diagnostics{}
	common.ValidateStateAndPlanEquals(state.Cluster, plan.Cluster, "cluster", &diags)
	return diags
}

func (r *ClusterAutoscalerResource) Delete(ctx context.Context, request resource.DeleteRequest,
	response *resource.DeleteResponse) {
	state := &ClusterAutoscalerState{}
	diags := request.State.Get(ctx, state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	response.Diagnostics.AddWarning(
		"Cannot delete Hosted CP cluster autoscaler",
		fmt.Sprintf(
			"Cannot delete the cluster autoscaler for cluster '%s'. "+
				"ROSA HCP clusters must have a cluster autoscaler. "+
				"It is being removed from the Terraform state only. "+
				"To resume managing the cluster autoscaler, import it again. "+
				"It will be automatically deleted when the cluster is deleted.",
			state.Cluster.ValueString(),
		),
	)

	response.State.RemoveResource(ctx)
}

func (r *ClusterAutoscalerResource) ImportState(ctx context.Context, request resource.ImportStateRequest,
	response *resource.ImportStateResponse) {
	tflog.Debug(ctx, "begin importstate()")

	resource.ImportStatePassthroughID(ctx, path.Root("cluster"), request, response)
}

// populateAutoscalerState copies the data from the API object to the Terraform state.
func populateAutoscalerState(object *cmv1.ClusterAutoscaler, clusterId string, state *ClusterAutoscalerState) error {
	state.Cluster = types.StringValue(clusterId)

	if value, exists := object.GetMaxPodGracePeriod(); exists {
		state.MaxPodGracePeriod = types.Int64Value(int64(value))
	} else {
		state.MaxPodGracePeriod = types.Int64Null()
	}

	if value, exists := object.GetPodPriorityThreshold(); exists {
		state.PodPriorityThreshold = types.Int64Value(int64(value))
	} else {
		state.PodPriorityThreshold = types.Int64Null()
	}

	state.MaxNodeProvisionTime = common.EmptiableStringToStringType(object.MaxNodeProvisionTime())

	if labels, exists := object.GetBalancingIgnoredLabels(); exists {
		list, err := common.StringArrayToList(labels)
		if err != nil {
			return err
		}
		state.BalancingIgnoredLabels = list
	} else if state.BalancingIgnoredLabels.IsNull() || state.BalancingIgnoredLabels.IsUnknown() ||
		len(state.BalancingIgnoredLabels.Elements()) != 0 {
		state.BalancingIgnoredLabels = types.ListNull(types.StringType)
	}

	state.ScaleDown = types.ObjectNull(scaleDownAttributeTypes)
	if scaleDown := object.ScaleDown(); scaleDown != nil {
		enabled := types.BoolNull()
		if value, exists := scaleDown.GetEnabled(); exists {
			enabled = types.BoolValue(value)
		}
		state.ScaleDown = types.ObjectValueMust(scaleDownAttributeTypes, map[string]attr.Value{
			enabledAttribute:              enabled,
			unneededTimeAttribute:         common.EmptiableStringToStringType(scaleDown.UnneededTime()),
			utilizationThresholdAttribute: common.EmptiableStringToStringType(scaleDown.UtilizationThreshold()),
			delayAfterAddAttribute:        common.EmptiableStringToStringType(scaleDown.DelayAfterAdd()),
			delayAfterDeleteAttribute:     common.EmptiableStringToStringType(scaleDown.DelayAfterDelete()),
			delayAfterFailureAttribute:    common.EmptiableStringToStringType(scaleDown.DelayAfterFailure()),
		})
	}

	state.ResourceLimits = types.ObjectNull(resourceLimitsAttributeTypes)
	if object.ResourceLimits() != nil {
		maxNodesTotal := types.Int64Null()
		if value, exists := object.ResourceLimits().GetMaxNodesTotal(); exists {
			maxNodesTotal = types.Int64Value(int64(value))
		}
		state.ResourceLimits = types.ObjectValueMust(resourceLimitsAttributeTypes, map[string]attr.Value{
			maxNodesTotalAttribute: maxNodesTotal,
		})
	}
	return nil
}

// clusterAutoscalerStateToObject builds a cluster-autoscaler API object from a given Terraform state.
func clusterAutoscalerStateToObject(state *ClusterAutoscalerState) (*cmv1.ClusterAutoscaler, error) {
	builder := cmv1.NewClusterAutoscaler()

	if !state.MaxPodGracePeriod.IsNull() && !state.MaxPodGracePeriod.IsUnknown() {
		builder.MaxPodGracePeriod(int(state.MaxPodGracePeriod.ValueInt64()))
	}

	if !state.PodPriorityThreshold.IsNull() && !state.PodPriorityThreshold.IsUnknown() {
		builder.PodPriorityThreshold(int(state.PodPriorityThreshold.ValueInt64()))
	}

	if !state.MaxNodeProvisionTime.IsNull() && !state.MaxNodeProvisionTime.IsUnknown() {
		builder.MaxNodeProvisionTime(state.MaxNodeProvisionTime.ValueString())
	}

	if !state.ResourceLimits.IsNull() && !state.ResourceLimits.IsUnknown() {
		resourceLimitsBuilder := cmv1.NewAutoscalerResourceLimits()
		maxNodesTotal := state.ResourceLimits.Attributes()[maxNodesTotalAttribute].(types.Int64)
		if !maxNodesTotal.IsNull() && !maxNodesTotal.IsUnknown() {
			resourceLimitsBuilder.MaxNodesTotal(int(maxNodesTotal.ValueInt64()))
		}

		builder.ResourceLimits(resourceLimitsBuilder)
	}

	if !state.BalancingIgnoredLabels.IsNull() && !state.BalancingIgnoredLabels.IsUnknown() {
		builder.BalancingIgnoredLabels(common.OptionalList(state.BalancingIgnoredLabels)...)
	}

	if !state.ScaleDown.IsNull() && !state.ScaleDown.IsUnknown() {
		values := state.ScaleDown.Attributes()
		scaleDown := cmv1.NewAutoscalerScaleDownConfig()
		if value := values[enabledAttribute].(types.Bool); !value.IsNull() && !value.IsUnknown() {
			scaleDown.Enabled(value.ValueBool())
			if !value.ValueBool() {
				// Persisted tuning values must not accompany ScaleUpOnly requests.
				return builder.ScaleDown(scaleDown).Build()
			}
		}
		if value := values[unneededTimeAttribute].(types.String); !value.IsNull() && !value.IsUnknown() {
			scaleDown.UnneededTime(value.ValueString())
		}
		if value := values[utilizationThresholdAttribute].(types.String); !value.IsNull() && !value.IsUnknown() {
			scaleDown.UtilizationThreshold(value.ValueString())
		}
		if value := values[delayAfterAddAttribute].(types.String); !value.IsNull() && !value.IsUnknown() {
			scaleDown.DelayAfterAdd(value.ValueString())
		}
		if value := values[delayAfterDeleteAttribute].(types.String); !value.IsNull() && !value.IsUnknown() {
			scaleDown.DelayAfterDelete(value.ValueString())
		}
		if value := values[delayAfterFailureAttribute].(types.String); !value.IsNull() && !value.IsUnknown() {
			scaleDown.DelayAfterFailure(value.ValueString())
		}
		builder.ScaleDown(scaleDown)
	}

	return builder.Build()
}
