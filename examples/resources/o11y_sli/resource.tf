resource "o11y_sli" "checkout_latency" {
  sli_key        = "checkout-latency"
  name           = "Checkout latency"
  description    = "Share of checkout server spans completed within 750ms."
  indicator_kind = "latency"
  aggregation    = "threshold_ratio"

  scope_json = jsonencode({
    service_names = ["checkout"]
    span_kinds    = ["SLI_SPAN_KIND_V1_SERVER"]
    telemetry_attribute_filters = [{
      field    = "http.request.method"
      operator = "SLI_TELEMETRY_FILTER_OPERATOR_V1_EQUALS"
      values   = ["POST"]
      source   = "SLI_TELEMETRY_ATTRIBUTE_SOURCE_V1_SPAN"
    }]
  })

  eligible_events       = "matching checkout server spans"
  good_events           = "matching spans completed within 750ms"
  excluded_events       = ""
  latency_threshold     = "750ms"
  missing_data_behavior = "unknown"
}

resource "o11y_slo" "checkout_latency" {
  slo_key         = "checkout-latency"
  name            = "Checkout latency"
  sli_id          = o11y_sli.checkout_latency.id
  sli_revision_id = o11y_sli.checkout_latency.current_revision_id
  target_ratio    = 0.95

  window_mode            = "rolling"
  rolling_window_seconds = 604800
  labels_json            = jsonencode({ service = "checkout" })
}
