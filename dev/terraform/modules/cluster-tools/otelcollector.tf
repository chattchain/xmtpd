resource "argocd_application" "otelcollector" {
  depends_on = [argocd_project.tools]
  wait       = var.wait_for_ready
  metadata {
    name      = "otelcollector"
    namespace = var.argocd_namespace
  }
  spec {
    project = argocd_project.tools.metadata[0].name
    source {
      repo_url        = "https://open-telemetry.github.io/opentelemetry-helm-charts"
      chart           = "opentelemetry-collector"
      target_revision = "0.49.1"
      helm {
        release_name = "otelcollector"
        values       = <<EOT
          mode: daemonset
          podAnnotations:
            prometheus.io/scrape: "true"
            prometheus.io/path: /metrics
            prometheus.io/port: "9464"
          ports:
            metrics:
              enabled: true
          config:
            receivers:
              otlp:
                protocols:
                  grpc:
                    endpoint: 0.0.0.0:4317
                  http:
                    endpoint: 0.0.0.0:4318
                    cors:
                      allowed_origins:
                        - "http://*"
                        - "https://*"
            exporters:
              otlp:
                endpoint: "${local.jaeger_collector_endpoint}"
                tls:
                  insecure: true
              logging: {}
              prometheus:
                endpoint: "0.0.0.0:9464"
                resource_to_telemetry_conversion:
                  enabled: true
                enable_open_metrics: true
            processors:
              batch: {}
              spanmetrics:
                metrics_exporter: prometheus
              # temporary measure until description is fixed in .NET
              transform:
                metric_statements:
                  - context: metric
                    statements:
                      - set(description, "Measures the duration of inbound HTTP requests") where name == "http.server.duration"
            service:
              pipelines:
                traces:
                  receivers: [otlp]
                  processors: [spanmetrics, batch]
                  exporters: [logging, otlp]
                metrics:
                  receivers: [otlp, prometheus]
                  processors: [transform, batch]
                  exporters: [prometheus, logging]
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
