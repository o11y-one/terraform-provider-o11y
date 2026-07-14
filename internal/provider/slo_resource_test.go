package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
)

func TestSLOSchemaExposesExactWindowContract(t *testing.T) {
	var response resource.SchemaResponse
	NewSLOResource().Schema(context.Background(), resource.SchemaRequest{}, &response)

	for _, name := range []string{
		"window_mode", "rolling_window_seconds", "calendar_period", "calendar_timezone",
		"effective_from", "maximum_window_seconds", "current_revision_id", "revision_number",
	} {
		if response.Schema.Attributes[name] == nil {
			t.Fatalf("SLO resource is missing %s", name)
		}
	}
}

func TestSLORevisionInputPreservesCalendarContract(t *testing.T) {
	input, err := sloRevisionInput(&sloModel{
		SLIID:            types.StringValue("019f7aa2-6c7f-7000-8000-000000000001"),
		SLIRevisionID:    types.StringValue("019f7aa2-6c7f-7000-8000-000000000002"),
		TargetRatio:      types.Float64Value(0.999),
		WindowMode:       types.StringValue("calendar"),
		CalendarPeriod:   types.StringValue("quarter"),
		CalendarTimezone: types.StringValue("Asia/Kolkata"),
		Labels:           types.StringValue(`{"service":"checkout"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.WindowMode != alertsv1.SloWindowModeV1_SLO_WINDOW_MODE_V1_CALENDAR ||
		input.CalendarPeriod != alertsv1.SloCalendarPeriodV1_SLO_CALENDAR_PERIOD_V1_QUARTER ||
		input.CalendarTimezone != "Asia/Kolkata" || input.RollingWindowSeconds != 0 {
		t.Fatalf("calendar contract was not preserved: %+v", input)
	}
}

func TestSLORevisionInputDefaultsRollingWindow(t *testing.T) {
	input, err := sloRevisionInput(&sloModel{
		SLIID:         types.StringValue("019f7aa2-6c7f-7000-8000-000000000001"),
		SLIRevisionID: types.StringValue("019f7aa2-6c7f-7000-8000-000000000002"),
		TargetRatio:   types.Float64Value(0.995),
		WindowMode:    types.StringValue("rolling"),
		Labels:        types.StringValue(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.WindowMode != alertsv1.SloWindowModeV1_SLO_WINDOW_MODE_V1_ROLLING || input.RollingWindowSeconds != defaultRollingSLOWindowSeconds {
		t.Fatalf("unexpected rolling default: %+v", input)
	}
}
