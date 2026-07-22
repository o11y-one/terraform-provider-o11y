package provider

import (
	"testing"

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
