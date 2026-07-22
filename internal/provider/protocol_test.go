package provider

import (
	"fmt"
	"net"
	"regexp"
	"strings"
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
			if len(service.destinations) != 0 || len(service.policies) != 0 || len(service.definitions) != 0 || len(service.windows) != 0 || len(service.silences) != 0 || len(service.slos) != 0 {
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
					terraformresource.TestCheckResourceAttr("o11y_agent_quality_alert.quality", "recipe_config_json", `{"max_bad_outcome_rate":0.1}`),
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

func TestProviderProtocolSLOCalendarLifecycle(t *testing.T) {
	endpoint, service, cleanup := startProtocolTestServer(t)
	defer cleanup()

	terraformresource.UnitTest(t, terraformresource.TestCase{
		ProtoV6ProviderFactories: protocolProviderFactories(),
		CheckDestroy: func(_ *terraform.State) error {
			service.mu.Lock()
			defer service.mu.Unlock()
			if len(service.slos) != 0 {
				return fmt.Errorf("remote SLO resources remain after Terraform destroy")
			}
			return nil
		},
		Steps: []terraformresource.TestStep{
			{
				Config: protocolSLOConfig(endpoint, "calendar"),
				Check: terraformresource.ComposeAggregateTestCheckFunc(
					terraformresource.TestCheckResourceAttrSet("o11y_slo.checkout", "id"),
					terraformresource.TestCheckResourceAttr("o11y_slo.checkout", "window_mode", "calendar"),
					terraformresource.TestCheckResourceAttr("o11y_slo.checkout", "rolling_window_seconds", "0"),
					terraformresource.TestCheckResourceAttr("o11y_slo.checkout", "calendar_period", "month"),
					terraformresource.TestCheckResourceAttr("o11y_slo.checkout", "calendar_timezone", "America/New_York"),
					terraformresource.TestCheckResourceAttr("o11y_slo.checkout", "revision_number", "1"),
					terraformresource.TestCheckResourceAttr("o11y_slo.checkout", "maximum_window_seconds", "8640000"),
				),
			},
			{
				Config: protocolSLOConfig(endpoint, "rolling"),
				Check: terraformresource.ComposeAggregateTestCheckFunc(
					terraformresource.TestCheckResourceAttr("o11y_slo.checkout", "window_mode", "rolling"),
					terraformresource.TestCheckResourceAttr("o11y_slo.checkout", "rolling_window_seconds", "2419200"),
					terraformresource.TestCheckNoResourceAttr("o11y_slo.checkout", "calendar_period"),
					terraformresource.TestCheckNoResourceAttr("o11y_slo.checkout", "calendar_timezone"),
					terraformresource.TestCheckResourceAttr("o11y_slo.checkout", "revision_number", "2"),
				),
			},
			{ResourceName: "o11y_slo.checkout", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestProviderProtocolSLILifecycle(t *testing.T) {
	endpoint, service, cleanup := startProtocolTestServer(t)
	defer cleanup()

	terraformresource.UnitTest(t, terraformresource.TestCase{
		ProtoV6ProviderFactories: protocolProviderFactories(),
		CheckDestroy: func(_ *terraform.State) error {
			service.mu.Lock()
			defer service.mu.Unlock()
			if len(service.slis) != 0 {
				return fmt.Errorf("remote SLI remains after Terraform destroy")
			}
			return nil
		},
		Steps: []terraformresource.TestStep{
			{Config: protocolSLIConfig(endpoint, "Checkout availability"), Check: terraformresource.ComposeAggregateTestCheckFunc(
				terraformresource.TestCheckResourceAttrSet("o11y_sli.checkout", "id"),
				terraformresource.TestCheckResourceAttrSet("o11y_sli.checkout", "current_revision_id"),
				terraformresource.TestCheckResourceAttr("o11y_sli.checkout", "revision_number", "1"),
				terraformresource.TestCheckResourceAttr("o11y_sli.checkout", "indicator_kind", "availability"),
			)},
			{Config: protocolSLIConfig(endpoint, "Checkout availability updated"), Check: terraformresource.ComposeAggregateTestCheckFunc(
				terraformresource.TestCheckResourceAttr("o11y_sli.checkout", "name", "Checkout availability updated"),
				terraformresource.TestCheckResourceAttr("o11y_sli.checkout", "revision_number", "2"),
			)},
			{ResourceName: "o11y_sli.checkout", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestProviderProtocolNotificationTemplateLifecycle(t *testing.T) {
	endpoint, service, cleanup := startProtocolTestServer(t)
	defer cleanup()

	terraformresource.UnitTest(t, terraformresource.TestCase{
		ProtoV6ProviderFactories: protocolProviderFactories(),
		CheckDestroy: func(_ *terraform.State) error {
			service.mu.Lock()
			defer service.mu.Unlock()
			for _, item := range service.templates {
				if item.ArchivedAt == nil {
					return fmt.Errorf("notification template was not archived on destroy")
				}
			}
			return nil
		},
		Steps: []terraformresource.TestStep{
			{Config: protocolNotificationTemplateConfig(endpoint, "Customer impact", "Investigate {{ hypothesis }}"), Check: terraformresource.ComposeAggregateTestCheckFunc(
				terraformresource.TestCheckResourceAttrSet("o11y_alert_notification_template.customer_impact", "id"),
				terraformresource.TestCheckResourceAttrSet("o11y_alert_notification_template.customer_impact", "published_revision_id"),
				terraformresource.TestCheckResourceAttr("o11y_alert_notification_template.customer_impact", "published", "true"),
			)},
			{Config: protocolNotificationTemplateConfig(endpoint, "Customer impact updated", "Act on {{ hypothesis }}"), Check: terraformresource.ComposeAggregateTestCheckFunc(
				terraformresource.TestCheckResourceAttr("o11y_alert_notification_template.customer_impact", "name", "Customer impact updated"),
				terraformresource.TestCheckResourceAttr("o11y_alert_notification_template.customer_impact", "published", "true"),
			)},
			{ResourceName: "o11y_alert_notification_template.customer_impact", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestProviderProtocolSLORejectsInvalidTimezone(t *testing.T) {
	endpoint, _, cleanup := startProtocolTestServer(t)
	defer cleanup()

	config := strings.Replace(protocolSLOConfig(endpoint, "calendar"), "America/New_York", "Mars/Olympus", 1)
	terraformresource.UnitTest(t, terraformresource.TestCase{
		ProtoV6ProviderFactories: protocolProviderFactories(),
		Steps: []terraformresource.TestStep{{
			Config: config, PlanOnly: true,
			ExpectError: regexp.MustCompile(`Invalid IANA timezone`),
		}},
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
	alertsv1.RegisterAlertSloServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	return "http://" + listener.Addr().String(), service, func() {
		server.Stop()
		_ = listener.Close()
	}
}

func protocolSLOConfig(endpoint, mode string) string {
	window := `
  window_mode = "calendar"
  calendar_period = "month"
  calendar_timezone = "America/New_York"`
	if mode == "rolling" {
		window = `
  window_mode = "rolling"
  rolling_window_seconds = 2419200`
	}
	return protocolProviderConfig(endpoint) + fmt.Sprintf(`
resource "o11y_slo" "checkout" {
  slo_key = "checkout-availability"
  name = "Checkout availability"
  description = "Calendar-aware error budget"
  sli_id = "019f7aa2-6c7f-7000-8000-000000000001"
  sli_revision_id = "019f7aa2-6c7f-7000-8000-000000000002"
  target_ratio = 0.999
  labels_json = jsonencode({ service = "checkout" })
  %s
}
`, window)
}

func protocolSLIConfig(endpoint, name string) string {
	return protocolProviderConfig(endpoint) + fmt.Sprintf(`
resource "o11y_sli" "checkout" {
  sli_key = "checkout-availability"
  name = %q
  description = "Successful checkout server spans"
  indicator_kind = "availability"
  scope_json = jsonencode({
    service_names = ["checkout"]
    span_kinds = ["SLI_SPAN_KIND_V1_SERVER"]
  })
  eligible_events = "matching server spans"
  good_events = "matching spans without error"
  excluded_events = ""
  aggregation = "event_ratio"
  missing_data_behavior = "unknown"
}
`, name)
}

func protocolNotificationTemplateConfig(endpoint, name, summary string) string {
	return protocolProviderConfig(endpoint) + fmt.Sprintf(`
resource "o11y_alert_notification_template" "customer_impact" {
  template_key = "customer-impact"
  name = %q
  description = "Rich customer-impact notification"
  change_reason = "Terraform protocol test"
  published = true
  document_json = jsonencode({
    schema_version = 1
    subject_template = "[{{ incident.severity }}] {{ incident.title }}"
    firing = {
      title_template = "{{ incident.title }}"
      summary_template = %q
    }
    resolved = {
      title_template = "Resolved: {{ incident.title }}"
      summary_template = "Customer impact has recovered"
    }
  })
}
`, name, summary)
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
        node_kind = "ALERT_POLICY_TREE_NODE_KIND_V1_BRANCH"
        matcher = [{ field = "severity", values = [{ string_value = "critical" }] }]
        priority = 10
        behavior = "ALERT_NOTIFICATION_ROUTE_BEHAVIOR_V1_STOP"
      },
      {
        node_key = "primary"
        parent_node_key = "critical"
        node_kind = "ALERT_POLICY_TREE_NODE_KIND_V1_ROUTE"
        target = { destination_id = o11y_alert_destination.primary.id }
        priority = 10
        behavior = "ALERT_NOTIFICATION_ROUTE_BEHAVIOR_V1_CONTINUE"
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
      steps = [{ delay_seconds = 600, target = { destination_id = o11y_alert_destination.primary.id } }]
    }
    max_pages_per_day = 5
    max_pages_per_week = 20
  })
}

resource "o11y_alert_maintenance_window" "deploy" {
  window_key = "deploy-freeze"
  name = "Deploy freeze"
  scope_json = jsonencode({ service_names = ["checkout"] })
  starts_at = "2030-01-01T00:00:00Z"
  ends_at = "2030-01-01T01:00:00Z"
}

resource "o11y_alert_silence" "provider" {
  silence_key = "provider-maintenance"
  matcher_json = jsonencode({ clauses = [{ field = "provider", values = [{ string_value = "openai" }] }] })
  reason = "Provider maintenance"
  starts_at = "2030-01-01T00:00:00Z"
  ends_at = "2030-01-01T01:00:00Z"
}

resource "o11y_agent_quality_alert" "quality" {
  slug = "quality"
  name = %q
  description = "Terraform protocol acceptance"
  severity = "warning"
  scope_json = jsonencode({ service_names = ["checkout"] })
  owner_json = jsonencode({ team_id = "platform" })
  action_json = jsonencode({ external_runbook_url = "https://runbooks.example.test/quality" })
  evaluation_settings_json = jsonencode({})
  sample_guard_json = jsonencode({ minimum_events = "20" })
  recipe_config_json = jsonencode({ max_bad_outcome_rate = 0.1 })
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
