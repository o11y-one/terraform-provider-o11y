package provider

import (
	"fmt"
	"net"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	terraformresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
	"google.golang.org/grpc"
)

func TestProviderProtocolAlertingLifecycle(t *testing.T) {
	endpoint, service, cleanup := startProtocolTestServer(t)
	defer cleanup()

	terraformresource.UnitTest(t, terraformresource.TestCase{
		ProtoV6ProviderFactories: protocolProviderFactories(),
		CheckDestroy: func(_ *terraform.State) error {
			service.mu.Lock()
			defer service.mu.Unlock()
			if len(service.destinations) != 0 || len(service.policies) != 0 || len(service.definitions) != 0 || len(service.windows) != 0 || len(service.silences) != 0 {
				return fmt.Errorf("remote alert resources remain after Terraform destroy")
			}
			return nil
		},
		Steps: []terraformresource.TestStep{
			{
				Config: protocolAlertingConfig(endpoint, "Primary", "Quality"),
				Check: terraformresource.ComposeAggregateTestCheckFunc(
					terraformresource.TestCheckResourceAttr("o11y_alert_destination.primary", "name", "Primary"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_destination.primary", "id"),
					terraformresource.TestCheckResourceAttr("o11y_alert_notification_policy.default", "name", "Default"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_notification_policy.default", "id"),
					terraformresource.TestCheckResourceAttr("o11y_alert_notification_policy.default", "revision", "1"),
					terraformresource.TestCheckResourceAttr("o11y_alert_maintenance_window.deploy", "name", "Deploy freeze"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_maintenance_window.deploy", "id"),
					terraformresource.TestCheckResourceAttr("o11y_alert_silence.provider", "reason", "Provider maintenance"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_silence.provider", "id"),
					terraformresource.TestCheckResourceAttr("o11y_agent_quality_alert.quality", "name", "Quality"),
					terraformresource.TestCheckResourceAttr("o11y_agent_quality_alert.quality", "evaluation_settings_json", `{}`),
					terraformresource.TestCheckResourceAttr("o11y_agent_quality_alert.quality", "evaluation_interval_seconds", "60"),
					terraformresource.TestCheckResourceAttr("o11y_agent_quality_alert.quality", "recipe_config_json", `{"maximum_failure_rate":0.1}`),
					terraformresource.TestCheckResourceAttr("data.o11y_alert_preview.quality", "predicted_firing_count", "2"),
					terraformresource.TestCheckResourceAttr("data.o11y_alert_preview.quality", "predicted_notification_count", "0"),
				),
			},
			{
				Config: protocolAlertingConfig(endpoint, "Primary updated", "Quality updated"),
				Check: terraformresource.ComposeAggregateTestCheckFunc(
					terraformresource.TestCheckResourceAttr("o11y_alert_destination.primary", "name", "Primary updated"),
					terraformresource.TestCheckResourceAttr("o11y_agent_quality_alert.quality", "name", "Quality updated"),
				),
			},
			{ResourceName: "o11y_alert_destination.primary", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_notification_policy.default", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_maintenance_window.deploy", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_silence.provider", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_agent_quality_alert.quality", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestProviderProtocolNotifyFailsClosed(t *testing.T) {
	endpoint, _, cleanup := startProtocolTestServer(t)
	defer cleanup()

	terraformresource.UnitTest(t, terraformresource.TestCase{
		ProtoV6ProviderFactories: protocolProviderFactories(),
		Steps: []terraformresource.TestStep{{
			Config:      protocolAlertConfig(endpoint, true),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`Notify activation is not supported`),
		}},
	})
}

func protocolProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"o11y": providerserver.NewProtocol6WithError(New("protocol-test")()),
	}
}

func startProtocolTestServer(t *testing.T) (string, *alertTestServer, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := newAlertTestServer()
	server := grpc.NewServer()
	alertsv1.RegisterAlertDefinitionServiceServer(server, service)
	alertsv1.RegisterAlertRuntimeServiceServer(server, service)
	alertsv1.RegisterAlertNotificationServiceServer(server, service)
	alertsv1.RegisterAlertPreviewServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	return "http://" + listener.Addr().String(), service, func() {
		server.Stop()
		_ = listener.Close()
	}
}

func protocolAlertingConfig(endpoint, destinationName, alertName string) string {
	return protocolProviderConfig(endpoint) + fmt.Sprintf(`
resource "o11y_alert_destination" "primary" {
  destination_key = "primary"
  name = %q
  kind = "webhook"
  enabled = true
  config_json = jsonencode({ url = "https://hooks.example.test" })
  secret_refs_json = jsonencode({ authorization = "secret:webhook" })
}

resource "o11y_alert_notification_policy" "default" {
  policy_key = "default"
  name = "Default"
  enabled = true
  config_json = jsonencode({
    tree = { nodes = [
      {
        node_key = "critical"
        node_kind = "branch"
        matcher = { severity = ["critical"] }
        priority = 10
        continue_evaluation = false
      },
      {
        node_key = "primary"
        parent_node_key = "critical"
        node_kind = "route"
        destination_id = o11y_alert_destination.primary.id
        priority = 10
        continue_evaluation = true
      }
    ] }
    grouping = {
      group_by = ["service.name", "environment"]
      group_wait_seconds = 30
      group_interval_seconds = 300
    }
    notifications = {
      repeat_interval_seconds = 900
      repeat_limit = 2
      notify_resolved = true
    }
    escalation = {
      schedule_key = "critical"
      steps = [{ delay_seconds = 600, destination_id = o11y_alert_destination.primary.id }]
    }
    noise_budget = { max_pages_per_day = 5, max_pages_per_week = 20 }
  })
}

resource "o11y_alert_maintenance_window" "deploy" {
  window_key = "deploy-freeze"
  name = "Deploy freeze"
  scope_json = jsonencode({ service = "checkout" })
  starts_at = "2030-01-01T00:00:00Z"
  ends_at = "2030-01-01T01:00:00Z"
}

resource "o11y_alert_silence" "provider" {
  silence_key = "provider-maintenance"
  matcher_json = jsonencode({ provider = "openai" })
  reason = "Provider maintenance"
  starts_at = "2030-01-01T00:00:00Z"
  ends_at = "2030-01-01T01:00:00Z"
}

resource "o11y_agent_quality_alert" "quality" {
  slug = "quality"
  name = %q
  description = "Terraform protocol acceptance"
  severity = "warning"
  scope_json = jsonencode({ service = "checkout" })
  owner_json = jsonencode({ team = "platform" })
  action_json = jsonencode({ runbook = "https://runbooks.example.test/quality" })
  evaluation_settings_json = jsonencode({})
  sample_guard_json = jsonencode({ minimum_runs = 20 })
  recipe_config_json = jsonencode({ maximum_failure_rate = 0.1 })
  paused = false
  notify = false
}

data "o11y_alert_preview" "quality" {
  definition_id = o11y_agent_quality_alert.quality.id
}
`, destinationName, alertName)
}

func protocolAlertConfig(endpoint string, notify bool) string {
	return protocolProviderConfig(endpoint) + fmt.Sprintf(`
resource "o11y_agent_quality_alert" "quality" {
  slug = "quality"
  name = "Quality"
  description = "Terraform protocol acceptance"
  severity = "warning"
  scope_json = jsonencode({})
  owner_json = jsonencode({})
  action_json = jsonencode({})
  evaluation_settings_json = jsonencode({})
  sample_guard_json = jsonencode({})
  recipe_config_json = jsonencode({})
  paused = false
  notify = %t
}
`, notify)
}

func protocolProviderConfig(endpoint string) string {
	return fmt.Sprintf(`
provider "o11y" {
  endpoint = %q
  token = "test-token"
  tenant_id = %q
  org_id = %q
  insecure_skip_verify = true
}
`, endpoint, testTenantID, testOrgID)
}
