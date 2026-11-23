resource "rhcs_hcp_cluster_autoscaler" "cluster_autoscaler" {
  cluster                 = "cluster-id-123"
  max_pod_grace_period    = 600
  pod_priority_threshold  = -10
  max_node_provision_time = "15m"

  resource_limits = {
    max_nodes_total = 5
  }

  # These settings require an HCP cluster running version 4.20 or newer.
  balancing_ignored_labels = ["custom.label/zone"]
  scale_down = {
    enabled               = true
    unneeded_time         = "10m"
    utilization_threshold = "0.5"
    delay_after_add       = "10m"
    delay_after_delete    = "0s"
    delay_after_failure   = "3m"
  }
}
