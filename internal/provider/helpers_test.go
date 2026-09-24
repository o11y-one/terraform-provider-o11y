package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
)

func TestJSONFromProtoPreservesEquivalentConfiguredRepresentation(t *testing.T) {
	configured := types.StringValue(`{"minimum_events":50}`)
	value := &alertsv1.AlertSampleGuardV1{MinimumEvents: 50}

	result := jsonFromProtoPreserving(configured, value)
	if result.ValueString() != configured.ValueString() {
		t.Fatalf("equivalent uint64 JSON changed from %s to %s", configured.ValueString(), result.ValueString())
	}
}

func TestJSONFromProtoPreservesExplicitEmptyCollections(t *testing.T) {
	configured := types.StringValue(`{"firing":{"blocks":[],"title_template":"Alert"},"schema_version":1}`)
	value := &alertsv1.NotificationTemplateDocumentV1{SchemaVersion: 1, Firing: &alertsv1.NotificationTemplateVariantV1{TitleTemplate: "Alert"}}

	result := jsonFromProtoPreserving(configured, value)
	if result.ValueString() != configured.ValueString() {
		t.Fatalf("equivalent empty collection JSON changed from %s to %s", configured.ValueString(), result.ValueString())
	}
}

func TestJSONFromProtoUsesAuthorityWhenValuesDiffer(t *testing.T) {
	configured := types.StringValue(`{"minimum_events":25}`)
	value := &alertsv1.AlertSampleGuardV1{MinimumEvents: 50}

	result := jsonFromProtoPreserving(configured, value)
	if result.ValueString() != `{"minimum_events":"50"}` {
		t.Fatalf("got %s, want authoritative protobuf JSON", result.ValueString())
	}
}

func TestJSONFromProtoPreservesScopeFilterValuesEchoedAsTyped(t *testing.T) {
	str := func(v string) *alertsv1.AlertScalarValueV1 {
		return &alertsv1.AlertScalarValueV1{Value: &alertsv1.AlertScalarValueV1_StringValue{StringValue: v}}
	}
	echo := func(values []string, typed ...*alertsv1.AlertScalarValueV1) *alertsv1.AlertScopeV1 {
		return &alertsv1.AlertScopeV1{TelemetryAttributeFilters: []*alertsv1.SliTelemetryAttributeFilterV1{{
			Field: "http.request.method", Operator: alertsv1.SliTelemetryFilterOperatorV1_SLI_TELEMETRY_FILTER_OPERATOR_V1_EQUALS,
			Source: alertsv1.SliTelemetryAttributeSourceV1_SLI_TELEMETRY_ATTRIBUTE_SOURCE_V1_SPAN, Values: values, TypedValues: typed,
		}}}
	}
	filter := `{"telemetry_attribute_filters":[{"field":"http.request.method","operator":"SLI_TELEMETRY_FILTER_OPERATOR_V1_EQUALS","source":"SLI_TELEMETRY_ATTRIBUTE_SOURCE_V1_SPAN",%s}]}`
	for _, tc := range []struct {
		configured string
		server     *alertsv1.AlertScopeV1
		preserved  bool
	}{
		{`"values":["POST"]`, echo([]string{"POST"}, str("POST")), true},
		{`"typed_values":[{"string_value":"POST"},{"int_value":"42"}]`, echo([]string{"POST"}, str("POST"), &alertsv1.AlertScalarValueV1{Value: &alertsv1.AlertScalarValueV1_IntValue{IntValue: 42}}), true},
		{`"values":["POST"]`, echo([]string{"PUT"}, str("PUT")), false},
	} {
		configured := types.StringValue(fmt.Sprintf(filter, tc.configured))
		result := jsonFromProtoPreserving(configured, tc.server)
		if (result.ValueString() == configured.ValueString()) != tc.preserved {
			t.Errorf("configured %s, server echo %v: got %s, want preserved=%t", configured.ValueString(), tc.server, result.ValueString(), tc.preserved)
		}
	}
}
