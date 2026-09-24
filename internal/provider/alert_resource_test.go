package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
)

func validateRecipeConfig(t *testing.T, r resource.Resource, recipe string) string {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	objectType := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	values := map[string]tftypes.Value{}
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	values["recipe_config_json"] = tftypes.NewValue(tftypes.String, recipe)
	var resp resource.ValidateConfigResponse
	r.(resource.ResourceWithValidateConfig).ValidateConfig(ctx, resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, values)},
	}, &resp)
	var errors []string
	for _, d := range resp.Diagnostics.Errors() {
		errors = append(errors, d.Summary()+": "+d.Detail())
	}
	return strings.Join(errors, "\n")
}

func TestRecipeConfigFollowsServedContract(t *testing.T) {
	for _, key := range []string{"fact_family", "eligible_event_field", "good_event_field"} {
		errors := validateRecipeConfig(t, NewSLOAlertResource(), `{"target_percent":99.5,"`+key+`":"x"}`)
		if !strings.Contains(errors, `"`+key+`"`) {
			t.Errorf("recipe_config_json with reserved %s was not refused by name; errors: %q", key, errors)
		}
	}
	if errors := validateRecipeConfig(t, NewSLOAlertResource(), `{"slo_id":"x","slo_revision_id":"y","fast_burn_threshold":14.4}`); errors != "" {
		t.Errorf("valid SLO burn recipe refused: %s", errors)
	}
	derived := `{"slo_window_seconds":"3600","target_percent":99.5,"window_mode":"SLO_WINDOW_MODE_V1_CALENDAR","calendar_period":"SLO_CALENDAR_PERIOD_V1_MONTH","calendar_timezone":"UTC","revision_effective_from":"2030-01-01T00:00:00Z"}`
	errors := validateRecipeConfig(t, NewSLOAlertResource(), derived)
	for _, key := range []string{"slo_window_seconds", "target_percent", "window_mode", "calendar_period", "calendar_timezone", "revision_effective_from"} {
		if !strings.Contains(errors, key) {
			t.Errorf("recipe_config_json with server-derived %s was not refused by name; errors: %q", key, errors)
		}
	}
	if errors := validateRecipeConfig(t, NewAgentQualityAlertResource(), `{"use_run_quality_facts":true}`); errors != "" {
		t.Errorf("served agent-quality field refused: %s", errors)
	}
}

func TestSetAlertPreservesExternallyActivatedNotifyState(t *testing.T) {
	var data alertModel
	setAlert(&data, &alertsv1.AlertDefinitionV1{
		Mode:                      alertsv1.AlertModeV1_ALERT_MODE_V1_NOTIFY,
		Severity:                  alertsv1.AlertSeverityV1_ALERT_SEVERITY_V1_WARNING,
		EvaluationIntervalSeconds: 60,
	})

	if !data.Notify.ValueBool() {
		t.Fatal("notify-mode API state must not be relabeled as Observe in Terraform state")
	}
	if data.Mode.ValueString() != "notify" {
		t.Fatalf("unexpected mode %q", data.Mode.ValueString())
	}
}

func TestSetAlertPreservesConfiguredOwnerPresentationForSameIdentity(t *testing.T) {
	data := alertModel{Owner: types.StringValue(`{"active":true,"display_name":"Alert live QA","team_id":"team-1"}`)}
	setAlert(&data, &alertsv1.AlertDefinitionV1{
		Owner: &alertsv1.AlertOwnerRefV1{
			Owner:       &alertsv1.AlertOwnerRefV1_TeamId{TeamId: "team-1"},
			DisplayName: "Engineering",
			Active:      true,
		},
	})

	want := `{"active":true,"display_name":"Alert live QA","team_id":"team-1"}`
	if data.Owner.ValueString() != want {
		t.Fatalf("owner JSON changed from configured presentation: got %s, want %s", data.Owner.ValueString(), want)
	}
}

func TestSetAlertUsesRemoteOwnerWhenIdentityChanges(t *testing.T) {
	data := alertModel{Owner: types.StringValue(`{"display_name":"Alert live QA","team_id":"team-1"}`)}
	setAlert(&data, &alertsv1.AlertDefinitionV1{
		Owner: &alertsv1.AlertOwnerRefV1{
			Owner:       &alertsv1.AlertOwnerRefV1_TeamId{TeamId: "team-2"},
			DisplayName: "Operations",
			Active:      true,
		},
	})

	want := `{"active":true,"display_name":"Operations","team_id":"team-2"}`
	if data.Owner.ValueString() != want {
		t.Fatalf("remote owner identity was not adopted: got %s, want %s", data.Owner.ValueString(), want)
	}
}

func TestSetAlertUsesRemoteOwnerWhenConfiguredActiveStatusChanges(t *testing.T) {
	configured := `{"active":true,"display_name":"Alert live QA","team_id":"team-1"}`
	data := alertModel{Owner: types.StringValue(configured)}
	setAlert(&data, &alertsv1.AlertDefinitionV1{
		Owner: &alertsv1.AlertOwnerRefV1{
			Owner:       &alertsv1.AlertOwnerRefV1_TeamId{TeamId: "team-1"},
			DisplayName: "Engineering",
			Active:      false,
		},
	})

	if data.Owner.ValueString() == configured {
		t.Fatal("an authoritative owner active-status change must be visible in Terraform state")
	}
}

func TestSetAlertPreservesEvaluationSettingsEquivalentToBackendDefaults(t *testing.T) {
	want := `{"pending_for_seconds":0,"recovering_for_seconds":60}`
	data := alertModel{
		EvaluationSettings: types.StringValue(want),
		EvaluationInterval: types.Int64Value(60),
	}
	setAlert(&data, &alertsv1.AlertDefinitionV1{
		EvaluationSettings: &alertsv1.AlertEvaluationSettingsV1{
			IntervalSeconds:      60,
			RecoveringForSeconds: 60,
			NoDataBehavior:       alertsv1.AlertNoDataBehaviorV1_ALERT_NO_DATA_BEHAVIOR_V1_HOLD_STATE,
		},
	})
	if data.EvaluationSettings.ValueString() != want {
		t.Fatalf("backend defaults changed configured Terraform state: got %s, want %s", data.EvaluationSettings.ValueString(), want)
	}
}
