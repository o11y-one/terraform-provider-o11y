resource "o11y_sli" "checkout_availability" {
  sli_key               = "checkout-availability"
  name                  = "Checkout availability"
  description           = "Share of checkout server spans that complete without an error status."
  indicator_kind        = "availability"
  aggregation           = "event_ratio"
  missing_data_behavior = "unknown"
  eligible_events       = "checkout server spans"
  good_events           = "checkout server spans without an error status"

  # Availability and latency SLIs need span_kinds: omitted, the server stores SERVER
  # and the next plan shows a change.
  scope_json = jsonencode({
    service_names = ["checkout"]
    environments  = ["production"]
    span_kinds    = ["SLI_SPAN_KIND_V1_SERVER"]
  })
}

resource "o11y_sli" "checkout_latency" {
  sli_key               = "checkout-latency"
  name                  = "Checkout latency"
  description           = "Share of checkout POST requests that complete within 750ms."
  indicator_kind        = "latency"
  aggregation           = "threshold_ratio"
  latency_threshold     = "750ms"
  missing_data_behavior = "unknown"
  eligible_events       = "checkout POST server spans"
  good_events           = "checkout POST server spans completed within 750ms"

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
}
