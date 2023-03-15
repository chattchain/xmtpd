locals {
  grafana_hostnames       = [for hostname in var.hostnames : "grafana.${hostname}"]
  grafana_public_hostname = local.grafana_hostnames[0]
}

resource "kubernetes_config_map" "xmtp-dashboards" {
  depends_on = [kubernetes_namespace.tools]
  metadata {
    name      = "xmtp-dashboards"
    namespace = var.namespace
  }
  data = {
    "xmtp-network-api.json" = "${file("${path.module}/grafana/dashboards/xmtp-network-api.json")}"
  }
}

resource "argocd_application" "grafana" {
  count      = var.enable_monitoring ? 1 : 0
  depends_on = [argocd_project.tools]
  wait       = var.wait_for_ready
  metadata {
    name      = "grafana"
    namespace = var.argocd_namespace
  }
  spec {
    project = argocd_project.tools.metadata[0].name
    source {
      repo_url        = "https://grafana.github.io/helm-charts"
      chart           = "grafana"
      target_revision = "6.51.2"
      helm {
        release_name = "grafana"
        values       = <<EOT
          nodeSelector:
            node-pool: ${var.node_pool}
          persistence:
            enabled: false
          ingress:
            enabled: true
            hosts:
              - ${local.grafana_public_hostname}
          grafana.ini:
            auth.anonymous:
              enabled: true
              org_name: "Main Org."
              # Role for unauthenticated users, other valid values are `Editor` and `Admin`
              org_role: "Admin"
          datasources:
            datasources.yaml:
              apiVersion: 1
              datasources:
              - name: Prometheus
                uid: xmtpd-metrics
                type: prometheus
                url: http://${local.prometheus_server_endpoint}
                editable: true
                isDefault: true
                jsonData:
                  exemplarTraceIdDestinations:
                    - datasourceUid: xmtpd-traces
                      name: trace_id
                    - url: ${local.jaegar_public_url}/jaeger/ui/trace/$${__value.raw}
                      name: trace_id
                      urlDisplayLabel: View in Jaeger UI
              - name: Jaeger
                uid: xmtpd-traces
                type: jaeger
                url: http://${local.jaeger_query_endpoint}/jaeger/ui
                editable: true
                isDefault: false
          ${indent(10, file("${path.module}/grafana/dashboards-helm-values.yaml"))}
        EOT
      }
    }

    destination {
      server    = "https://kubernetes.default.svc"
      namespace = var.namespace
    }

    sync_policy {
      automated = {
        prune       = true
        self_heal   = true
        allow_empty = false
      }
    }
  }
}
