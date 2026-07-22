package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
)

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
