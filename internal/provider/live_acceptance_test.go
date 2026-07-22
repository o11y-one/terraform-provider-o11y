package provider

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	terraformresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveProviderAlertingLifecycle(t *testing.T) {
	if os.Getenv("O11Y_ACC") != "1" {
		t.Skip("set O11Y_ACC=1 to run the disposable live alerting lifecycle")
	}
	for _, name := range []string{"O11Y_ENDPOINT", "O11Y_TOKEN", "O11Y_TENANT_ID", "O11Y_ORG_ID", "O11Y_ACC_WEBHOOK_URL"} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			t.Fatalf("%s is required for live acceptance", name)
		}
	}
	t.Setenv("TF_ACC", "1")
	runID := fmt.Sprintf("tf-live-%d", time.Now().UTC().UnixNano())

	terraformresource.Test(t, terraformresource.TestCase{
		ProtoV6ProviderFactories: protocolProviderFactories(),
		Steps: []terraformresource.TestStep{
			{
				Config: liveAlertingConfig(runID, "initial"),
				Check: terraformresource.ComposeAggregateTestCheckFunc(
					terraformresource.TestCheckResourceAttrSet("o11y_alert_destination.live", "id"),
					terraformresource.TestCheckResourceAttr("o11y_alert_destination.live", "name", "Terraform live initial"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_notification_policy.live", "id"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_maintenance_window.live", "id"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_silence.live", "id"),
					terraformresource.TestCheckResourceAttrSet("o11y_agent_quality_alert.live", "id"),
					terraformresource.TestCheckResourceAttrSet("data.o11y_alert_preview.live", "preview_id"),
				),
			},
			{
				Config: liveAlertingConfig(runID, "updated"),
				Check: terraformresource.ComposeAggregateTestCheckFunc(
					terraformresource.TestCheckResourceAttr("o11y_alert_destination.live", "name", "Terraform live updated"),
					terraformresource.TestCheckResourceAttr("o11y_agent_quality_alert.live", "name", "Terraform live updated"),
				),
			},
			{ResourceName: "o11y_alert_destination.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_notification_policy.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_maintenance_window.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_silence.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_agent_quality_alert.live", ImportState: true, ImportStateVerify: true},
		},
	})
}

func liveAlertingConfig(runID, suffix string) string {
	return fmt.Sprintf(`
provider "o11y" {
  insecure_skip_verify = true
}

resource "o11y_alert_destination" "live" {
  destination_key = %q
  name = %q
  kind = "webhook"
  enabled = true
  config_json = jsonencode({ url = %q })
  secret_refs_json = jsonencode({})
}

resource "o11y_alert_notification_policy" "live" {
  policy_key = %q
  name = %q
  enabled = true
  config_json = jsonencode({
    routes = [{
      route_key = %q
      target = { destination_id = o11y_alert_destination.live.id }
      matcher = []
      priority = 10
      enabled = true
      behavior = "ALERT_NOTIFICATION_ROUTE_BEHAVIOR_V1_STOP"
    }]
  })
}

resource "o11y_alert_maintenance_window" "live" {
  window_key = %q
  name = "Terraform live maintenance"
  scope_json = jsonencode({ service_names = [%q] })
  starts_at = "2030-01-01T00:00:00Z"
  ends_at = "2030-01-01T01:00:00Z"
}

resource "o11y_alert_silence" "live" {
  silence_key = %q
  matcher_json = jsonencode({ clauses = [{ field = "service", values = [{ string_value = %q }] }] })
  reason = "Terraform live acceptance"
  starts_at = "2030-01-01T00:00:00Z"
  ends_at = "2030-01-01T01:00:00Z"
}

resource "o11y_agent_quality_alert" "live" {
  slug = %q
  name = %q
  description = "Terraform live lifecycle acceptance"
  severity = "warning"
  scope_json = jsonencode({ service_names = [%q] })
  owner_json = jsonencode({ display_name = "Terraform" })
  action_json = jsonencode({ external_runbook_url = "https://runbooks.example.test/terraform-live" })
  evaluation_settings_json = jsonencode({})
  sample_guard_json = jsonencode({ minimum_events = "20" })
  recipe_config_json = jsonencode({ max_bad_outcome_rate = 0.1 })
  paused = false
  notify = false
}

data "o11y_alert_preview" "live" {
  definition_id = o11y_agent_quality_alert.live.id
}
`,
		runID+"-destination",
		"Terraform live "+suffix,
		os.Getenv("O11Y_ACC_WEBHOOK_URL"),
		runID+"-policy",
		"Terraform live policy "+suffix,
		runID+"-route",
		runID+"-window",
		runID,
		runID+"-silence",
		runID,
		runID+"-quality",
		"Terraform live "+suffix,
		runID,
	)
}
