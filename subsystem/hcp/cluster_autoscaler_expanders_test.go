// Copyright Red Hat
// SPDX-License-Identifier: Apache-2.0

package hcp

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"                      // nolint
	. "github.com/onsi/gomega"                         // nolint
	. "github.com/onsi/gomega/ghttp"                   // nolint
	. "github.com/openshift-online/ocm-sdk-go/testing" // nolint

	. "github.com/terraform-redhat/terraform-provider-rhcs/subsystem/framework"
)

var _ = Describe("HCP autoscaler expanders", func() {
	const endpoint = "/api/clusters_mgmt/v1/clusters/123/autoscaler"
	const defaults = `{"max_node_provision_time":"15m","expanders":["Priority","LeastWaste"],"scale_down":{"enabled":true}}`

	DescribeTable("creates and refreshes ordered expanders without drift", func(config, patch, response string, expected []any) {
		TestServer.AppendHandlers(
			CombineHandlers(
				VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123"),
				RespondWithJSON(http.StatusOK, `{"id":"123","state":"ready"}`),
			),
			CombineHandlers(VerifyRequest(http.MethodPatch, endpoint), VerifyJSON(patch), RespondWithJSON(http.StatusOK, response)),
		)
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "test" {
			cluster = "123"
			` + config + `
		}`)
		Expect(Terraform.Apply().ExitCode).To(BeZero())
		attributes := Terraform.Resource("rhcs_hcp_cluster_autoscaler", "test").(map[string]any)["attributes"].(map[string]any)
		if expected == nil {
			Expect(attributes["expanders"]).To(BeNil())
		} else {
			Expect(attributes["expanders"]).To(Equal(expected))
		}
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, response))
		Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())
	},
		Entry("omitted, using API defaults", "", `{"kind":"ClusterAutoscaler"}`, defaults, []any{"Priority", "LeastWaste"}),
		Entry("older API omitting expanders", "", `{"kind":"ClusterAutoscaler"}`, `{}`, nil),
		Entry("explicit null, using API defaults", `expanders = null`, `{"kind":"ClusterAutoscaler"}`, defaults, []any{"Priority", "LeastWaste"}),
		Entry("single expander", `expanders = ["Random"]`, `{"kind":"ClusterAutoscaler","expanders":["Random"]}`,
			`{"expanders":["Random"]}`, []any{"Random"}),
		Entry("three expanders in nonalphabetical order", `expanders = ["Priority","LeastWaste","Random"]`,
			`{"kind":"ClusterAutoscaler","expanders":["Priority","LeastWaste","Random"]}`,
			`{"expanders":["Priority","LeastWaste","Random"]}`, []any{"Priority", "LeastWaste", "Random"}),
	)

	It("imports, reorders, disables scale-down and retains removed configuration", func() {
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, defaults))
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "test" { cluster = "123" }`)
		Expect(Terraform.Import("rhcs_hcp_cluster_autoscaler.test", "123").ExitCode).To(BeZero())
		attributes := Terraform.Resource("rhcs_hcp_cluster_autoscaler", "test").(map[string]any)["attributes"].(map[string]any)
		Expect(attributes["expanders"]).To(Equal([]any{"Priority", "LeastWaste"}))
		Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())

		for _, step := range []struct {
			config        string
			expected      []any
			response      string
			enabled       bool
			provisionTime string
		}{
			{`expanders = ["LeastWaste","Priority"]`, []any{"LeastWaste", "Priority"},
				`{"max_node_provision_time":"15m","expanders":["LeastWaste","Priority"],"scale_down":{"enabled":true}}`, true, "15m"},
			{`expanders = ["Priority","LeastWaste","Random"]`, []any{"Priority", "LeastWaste", "Random"},
				`{"max_node_provision_time":"15m","expanders":["Priority","LeastWaste","Random"],"scale_down":{"enabled":false}}`, false, "15m"},
			{"", []any{"Priority", "LeastWaste", "Random"},
				`{"max_node_provision_time":"20m","expanders":["Priority","LeastWaste","Random"],"scale_down":{"enabled":false}}`, false, "20m"},
		} {
			TestServer.AppendHandlers(CombineHandlers(
				VerifyRequest(http.MethodPatch, endpoint),
				VerifyJQ(".expanders", step.expected),
				VerifyJQ(".scale_down", map[string]any{"enabled": step.enabled}),
				VerifyJQ(".max_node_provision_time", step.provisionTime),
				RespondWithJSON(http.StatusOK, step.response),
			))
			Terraform.Source(fmt.Sprintf(`resource "rhcs_hcp_cluster_autoscaler" "test" {
				cluster = "123"
				%s
				max_node_provision_time = %q
				scale_down = { enabled = %t }
			}`, step.config, step.provisionTime, step.enabled))
			Expect(Terraform.Apply().ExitCode).To(BeZero())
			attributes = Terraform.Resource("rhcs_hcp_cluster_autoscaler", "test").(map[string]any)["attributes"].(map[string]any)
			Expect(attributes["expanders"]).To(Equal(step.expected))
			TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, step.response))
			Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())
		}
	})

	It("refreshes external order changes and clears API-omitted expanders", func() {
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, defaults))
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "test" { cluster = "123" }`)
		Expect(Terraform.Import("rhcs_hcp_cluster_autoscaler.test", "123").ExitCode).To(BeZero())
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, `{"expanders":["LeastWaste","Priority"]}`))
		Expect(Terraform.Apply().ExitCode).To(BeZero())
		attributes := Terraform.Resource("rhcs_hcp_cluster_autoscaler", "test").(map[string]any)["attributes"].(map[string]any)
		Expect(attributes["expanders"]).To(Equal([]any{"LeastWaste", "Priority"}))
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, `{}`))
		Expect(Terraform.Apply().ExitCode).To(BeZero())
		attributes = Terraform.Resource("rhcs_hcp_cluster_autoscaler", "test").(map[string]any)["attributes"].(map[string]any)
		Expect(attributes["expanders"]).To(BeNil())
		Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())

		By("accepting newly available API defaults on an unrelated update")
		response := `{"max_node_provision_time":"20m","expanders":["Priority","LeastWaste"]}`
		TestServer.AppendHandlers(CombineHandlers(
			VerifyRequest(http.MethodPatch, endpoint),
			VerifyJSON(`{"kind":"ClusterAutoscaler","max_node_provision_time":"20m"}`),
			RespondWithJSON(http.StatusOK, response),
		))
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "test" {
			cluster = "123"
			max_node_provision_time = "20m"
		}`)
		Expect(Terraform.Apply().ExitCode).To(BeZero())
		attributes = Terraform.Resource("rhcs_hcp_cluster_autoscaler", "test").(map[string]any)["attributes"].(map[string]any)
		Expect(attributes["expanders"]).To(Equal([]any{"Priority", "LeastWaste"}))
		TestServer.RouteToHandler(http.MethodGet, endpoint, RespondWithJSON(http.StatusOK, response))
		Expect(Terraform.Run("plan", "-detailed-exitcode").ExitCode).To(BeZero())
	})

	It("preserves order and surfaces API rejection when Random is not last", func() {
		const reason = "Random must be last when multiple expanders are configured."
		TestServer.AppendHandlers(
			CombineHandlers(
				VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123"),
				RespondWithJSON(http.StatusOK, `{"id":"123","state":"ready"}`),
			),
			CombineHandlers(
				VerifyRequest(http.MethodPatch, endpoint),
				VerifyJSON(`{"kind":"ClusterAutoscaler","expanders":["Random","Priority"]}`),
				RespondWithJSON(http.StatusBadRequest, fmt.Sprintf(`{"kind":"Error","reason":%q}`, reason)),
			),
		)
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "test" {
			cluster = "123"
			expanders = ["Random", "Priority"]
		}`)
		output := Terraform.Apply()
		Expect(output.ExitCode).NotTo(BeZero())
		output.VerifyErrorContainsSubstring(reason)
	})

	DescribeTable("rejects invalid expanders before any API request", func(config, message string) {
		Terraform.Source(`resource "rhcs_hcp_cluster_autoscaler" "test" {
			cluster = "123"
			expanders = ` + config + `
		}`)
		output := Terraform.Run("plan", "-refresh=false")
		Expect(output.ExitCode).NotTo(BeZero())
		output.VerifyErrorContainsSubstring(message)
		Expect(TestServer.ReceivedRequests()).To(BeEmpty())
	},
		Entry("empty", `[]`, "at least 1"),
		Entry("too many", `["Priority","LeastWaste","Random","Priority"]`, "at most 3"),
		Entry("duplicate", `["Priority","Priority"]`, "duplicate"),
		Entry("lowercase", `["priority"]`, "must be one of"),
		Entry("hyphenated", `["least-waste"]`, "must be one of"),
		Entry("unsupported", `["MostPods"]`, "must be one of"),
		Entry("null element", `[null]`, "null"),
	)
})
