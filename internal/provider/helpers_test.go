package provider

import (
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
