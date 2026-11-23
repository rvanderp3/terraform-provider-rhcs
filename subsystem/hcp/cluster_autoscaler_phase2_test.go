// Copyright Red Hat
// SPDX-License-Identifier: Apache-2.0

package hcp

import (
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"                      // nolint
	. "github.com/onsi/gomega"                         // nolint
	. "github.com/onsi/gomega/ghttp"                   // nolint
	. "github.com/openshift-online/ocm-sdk-go/testing" // nolint

	. "github.com/terraform-redhat/terraform-provider-rhcs/subsystem/framework"
)

var _ = Describe("HCP autoscaler ported fields", func() {
	const endpoint = "/api/clusters_mgmt/v1/clusters/123/autoscaler"
	const body = `{
		"max_pod_grace_period":600,"pod_priority_threshold":-10,"max_node_provision_time":"15m",
		"resource_limits":{"max_nodes_total":180},
		"balancing_ignored_labels":["custom.label/zone"],
		"scale_down":{"enabled":true,"unneeded_time":"10m","utilization_threshold":"0.5",
			"delay_after_add":"10m","delay_after_delete":"0s","delay_after_failure":"3m"}
	}`
	const allScaleDown = `scale_down = {
		enabled = true
		unneeded_time = "10m"
		utilization_threshold = "0.5"
		delay_after_add = "10m"
		delay_after_delete = "0s"
		delay_after_failure = "3m"
	}`

	DescribeTable("applies and refreshes without drift", func(config, patch, response string) {
		TestServer.AppendHandlers(
			CombineHandlers(
				VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123"),
				RespondWithJSON(http.StatusOK, `{"id":"123","state":"ready"}`),
			),
			CombineHandlers(
				VerifyRequest(http.MethodPatch, endpoint),
				VerifyJSON(patch),
				RespondWithJSON(http.StatusOK, response),
			),
		)
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, response))
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" {
			cluster = "123"
			` + config + `
		}`)
		Expect(Terraform.Apply().ExitCode).To(BeZero())
		resource := Terraform.Resource("rhcs_hcp_cluster_autoscaler", "cluster_autoscaler").(map[string]any)
		attributes := resource["attributes"].(map[string]any)
		Expect(attributes["scale_down"]).To(Equal(map[string]any{
			"enabled": true, "unneeded_time": "10m", "utilization_threshold": "0.5",
			"delay_after_add": "10m", "delay_after_delete": "0s", "delay_after_failure": "3m",
		}))
		if strings.Contains(config, "= []") {
			Expect(attributes["balancing_ignored_labels"]).To(BeEmpty())
		} else {
			Expect(attributes["balancing_ignored_labels"]).To(Equal([]any{"custom.label/zone"}))
		}
		Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())
		Expect(TestServer.ReceivedRequests()).To(HaveLen(3))
	},
		Entry("with no new configuration", "", `{"kind":"ClusterAutoscaler"}`, body),
		Entry("with labels only", `balancing_ignored_labels = ["custom.label/zone"]`,
			`{"kind":"ClusterAutoscaler","balancing_ignored_labels":["custom.label/zone"]}`, body),
		Entry("with partial scale-down configuration", `scale_down = { enabled = true }`,
			`{"kind":"ClusterAutoscaler","scale_down":{"enabled":true}}`, body),
		Entry("with empty scale-down configuration", `scale_down = {}`,
			`{"kind":"ClusterAutoscaler","scale_down":{}}`, body),
		Entry("with all ported fields and an existing scalar", allScaleDown+`
			balancing_ignored_labels = ["custom.label/zone"]
			max_node_provision_time = "15m"`,
			`{"kind":"ClusterAutoscaler","max_node_provision_time":"15m",
			"balancing_ignored_labels":["custom.label/zone"],
			"scale_down":{"enabled":true,"unneeded_time":"10m","utilization_threshold":"0.5",
			"delay_after_add":"10m","delay_after_delete":"0s","delay_after_failure":"3m"}}`, body),
		Entry("with explicit empty labels returned as absent", `balancing_ignored_labels = []`,
			`{"kind":"ClusterAutoscaler","balancing_ignored_labels":[]}`,
			strings.ReplaceAll(body, `"balancing_ignored_labels":["custom.label/zone"],`, "")),
	)

	It("imports and updates ported fields, including clearing labels", func() {
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, body))
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" { cluster = "123" }`)
		Expect(Terraform.Import("rhcs_hcp_cluster_autoscaler.cluster_autoscaler", "123").ExitCode).To(BeZero())
		Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())
		updated := strings.ReplaceAll(body, `"unneeded_time":"10m"`, `"unneeded_time":"20m"`)
		updated = strings.ReplaceAll(updated, `"balancing_ignored_labels":["custom.label/zone"],`, "")
		TestServer.AppendHandlers(CombineHandlers(
			VerifyRequest(http.MethodPatch, endpoint),
			VerifyJQ(".scale_down.unneeded_time", "20m"),
			VerifyJQ(".balancing_ignored_labels", []any{}),
			RespondWithJSON(http.StatusOK, updated),
		))
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" {
			cluster = "123"
			balancing_ignored_labels = []
			scale_down = { unneeded_time = "20m" }
		}`)
		Expect(Terraform.Apply().ExitCode).To(BeZero())
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, updated))
		Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())
	})

	It("creates with scale-down disabled and no tuning configuration", func() {
		response := strings.ReplaceAll(body, `"enabled":true`, `"enabled":false`)
		TestServer.AppendHandlers(
			CombineHandlers(
				VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123"),
				RespondWithJSON(http.StatusOK, `{"id":"123","state":"ready"}`),
			),
			CombineHandlers(
				VerifyRequest(http.MethodPatch, endpoint),
				VerifyJSON(`{"kind":"ClusterAutoscaler","scale_down":{"enabled":false}}`),
				RespondWithJSON(http.StatusOK, response),
			),
		)
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" {
			cluster = "123"
			scale_down = { enabled = false, unneeded_time = null }
		}`)
		Expect(Terraform.Apply().ExitCode).To(BeZero())
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, response))
		Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())
	})

	It("disables and re-enables scale-down with persisted computed defaults", func() {
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, body))
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" { cluster = "123" }`)
		Expect(Terraform.Import("rhcs_hcp_cluster_autoscaler.cluster_autoscaler", "123").ExitCode).To(BeZero())

		for _, step := range []struct {
			name           string
			enabled        bool
			provisionTime  string
			patchScaleDown map[string]any
		}{
			{"disable", false, "15m", map[string]any{"enabled": false}},
			{"re-enable", true, "15m", map[string]any{
				"enabled": true, "unneeded_time": "10m", "utilization_threshold": "0.5",
				"delay_after_add": "10m", "delay_after_delete": "0s", "delay_after_failure": "3m",
			}},
			{"disable and change an unrelated scalar", false, "20m", map[string]any{"enabled": false}},
			{"change an unrelated scalar while disabled", false, "15m", map[string]any{"enabled": false}},
		} {
			By(step.name)
			response := strings.ReplaceAll(body, `"enabled":true`, fmt.Sprintf(`"enabled":%t`, step.enabled))
			response = strings.ReplaceAll(response, `"max_node_provision_time":"15m"`,
				fmt.Sprintf(`"max_node_provision_time":%q`, step.provisionTime))
			TestServer.AppendHandlers(CombineHandlers(
				VerifyRequest(http.MethodPatch, endpoint),
				VerifyJQ(".scale_down", step.patchScaleDown),
				VerifyJQ(".max_node_provision_time", step.provisionTime),
				VerifyJQ(".balancing_ignored_labels", []any{"custom.label/zone"}),
				RespondWithJSON(http.StatusOK, response),
			))
			Terraform.Source(fmt.Sprintf(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" {
				cluster = "123"
				max_node_provision_time = %q
				scale_down = { enabled = %t }
			}`, step.provisionTime, step.enabled))
			Expect(Terraform.Apply().ExitCode).To(BeZero())
			resource := Terraform.Resource("rhcs_hcp_cluster_autoscaler", "cluster_autoscaler").(map[string]any)
			attributes := resource["attributes"].(map[string]any)
			Expect(attributes["scale_down"]).To(Equal(map[string]any{
				"enabled": step.enabled, "unneeded_time": "10m", "utilization_threshold": "0.5",
				"delay_after_add": "10m", "delay_after_delete": "0s", "delay_after_failure": "3m",
			}))
			Expect(attributes["max_node_provision_time"]).To(Equal(step.provisionTime))
			TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, response))
			Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())
		}
	})

	It("rejects tuning when enabled is omitted but prior state is disabled", func() {
		disabled := strings.ReplaceAll(body, `"enabled":true`, `"enabled":false`)
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, disabled))
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" { cluster = "123" }`)
		Expect(Terraform.Import("rhcs_hcp_cluster_autoscaler.cluster_autoscaler", "123").ExitCode).To(BeZero())
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" {
			cluster = "123"
			scale_down = { unneeded_time = "20m" }
		}`)
		output := Terraform.Run("plan")
		Expect(output.ExitCode).NotTo(BeZero())
		output.VerifyErrorContainsSubstring("cannot be configured when scale_down.enabled is false")
		for _, request := range TestServer.ReceivedRequests() {
			Expect(request.Method).To(Equal(http.MethodGet))
		}
	})

	DescribeTable("rejects explicit tuning when scale-down is disabled", func(field, value string) {
		Terraform.Source(fmt.Sprintf(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" {
			cluster = "123"
			scale_down = { enabled = false, %s = %q }
		}`, field, value))
		output := Terraform.Run("plan", "-refresh=false")
		Expect(output.ExitCode).NotTo(BeZero())
		output.VerifyErrorContainsSubstring("cannot be configured when scale_down.enabled is false")
		Expect(TestServer.ReceivedRequests()).To(BeEmpty())
	},
		Entry("unneeded time", "unneeded_time", "10m"),
		Entry("utilization threshold", "utilization_threshold", "0.5"),
		Entry("add delay", "delay_after_add", "10m"),
		Entry("delete delay", "delay_after_delete", "0s"),
		Entry("failure delay", "delay_after_failure", "3m"),
	)

	DescribeTable("surfaces authoritative API rejection", func(config, reason string) {
		TestServer.AppendHandlers(
			CombineHandlers(
				VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123"),
				RespondWithJSON(http.StatusOK, `{"id":"123","state":"ready"}`),
			),
			CombineHandlers(
				VerifyRequest(http.MethodPatch, endpoint),
				RespondWithJSON(http.StatusBadRequest, fmt.Sprintf(`{"kind":"Error","reason":%q}`, reason)),
			),
		)
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" {
			cluster = "123"
			` + config + `
		}`)
		output := Terraform.Apply()
		Expect(output.ExitCode).NotTo(BeZero())
		output.VerifyErrorContainsSubstring(reason)
		Expect(TestServer.ReceivedRequests()).To(HaveLen(2))
	},
		Entry("old cluster version", `scale_down = { enabled = true }`, "Attribute 'scale_down' requires hosted cluster version 4.20 or newer."),
		Entry("subsecond duration", `scale_down = { unneeded_time = "500ms" }`, "scale_down.unneeded_time must be a whole number of seconds."),
		Entry("excess ratio precision", `scale_down = { utilization_threshold = "0.555" }`, "scale_down.utilization_threshold supports at most 2 decimal places."),
	)

	DescribeTable("rejects invalid ported configuration before API calls", func(config, reason string) {
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" {
			cluster = "123"
			` + config + `
		}`)
		output := Terraform.Apply()
		Expect(output.ExitCode).NotTo(BeZero())
		output.VerifyErrorContainsSubstring(reason)
		Expect(TestServer.ReceivedRequests()).To(BeEmpty())
	},
		Entry("unneeded duration", `scale_down = { unneeded_time = "bad" }`, "cannot be parsed to a duration"),
		Entry("negative add delay", `scale_down = { delay_after_add = "-1s" }`, "Only positive durations are allowed"),
		Entry("delete duration", `scale_down = { delay_after_delete = "bad" }`, "cannot be parsed to a duration"),
		Entry("failure duration", `scale_down = { delay_after_failure = "bad" }`, "cannot be parsed to a duration"),
		Entry("low ratio", `scale_down = { utilization_threshold = "-0.1" }`, "out of range"),
		Entry("high ratio", `scale_down = { utilization_threshold = "1.1" }`, "out of range"),
		Entry("non-numeric ratio", `scale_down = { utilization_threshold = "bad" }`, "cannot be parsed to a float"),
		Entry("too many labels", fmt.Sprintf(`balancing_ignored_labels = [%s]`, strings.TrimSuffix(strings.Repeat(`"label",`, 65), ",")), "at most 64"),
		Entry("unsupported Classic setting", `balance_similar_node_groups = true`, "Unsupported argument"),
		Entry("unsupported verbosity", `log_verbosity = 4`, "Unsupported argument"),
		Entry("unsupported local storage setting", `skip_nodes_with_local_storage = true`, "Unsupported argument"),
		Entry("unsupported DaemonSet setting", `ignore_daemonsets_utilization = true`, "Unsupported argument"),
	)
})
