package provider

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	terraformresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAlertingConfigIsValidHCL(t *testing.T) {
	t.Setenv("O11Y_ACC_WEBHOOK_URL", "https://receiver.example.test/alerts")
	t.Setenv("O11Y_ACC_SLACK_CHANNEL_ID", "C0123456789")
	t.Setenv("O11Y_ACC_SLACK_TOKEN_REF", "secret:terraform-live-slack")
	t.Setenv("O11Y_ACC_CONTACT_EMAIL", "alerts@example.test")
	t.Setenv("O11Y_OWNER_TEAM_ID", "019f0000-0000-7000-8000-000000000001")

	_, diagnostics := hclsyntax.ParseConfig(
		[]byte(liveAlertingConfig("tf-live-parse", "initial")),
		"live_acceptance.tf",
		hcl.Pos{Line: 1, Column: 1},
	)
	if diagnostics.HasErrors() {
		t.Fatalf("live alerting Terraform config is invalid:\n%s", diagnostics.Error())
	}
}

func TestLiveAlertingConfigPlansAgainstProviderSchema(t *testing.T) {
	endpoint, _, cleanup := startProtocolTestServer(t)
	defer cleanup()
	t.Setenv("O11Y_ENDPOINT", endpoint)
	t.Setenv("O11Y_TOKEN", "test-token")
	t.Setenv("O11Y_TENANT_ID", testTenantID)
	t.Setenv("O11Y_ORG_ID", testOrgID)
	t.Setenv("O11Y_ACC_WEBHOOK_URL", "https://receiver.example.test/alerts")
	t.Setenv("O11Y_ACC_SLACK_CHANNEL_ID", "C0123456789")
	t.Setenv("O11Y_ACC_SLACK_TOKEN_REF", "secret:terraform-live-slack")
	t.Setenv("O11Y_ACC_CONTACT_EMAIL", "alerts@example.test")
	t.Setenv("O11Y_OWNER_TEAM_ID", "019f0000-0000-7000-8000-000000000001")

	terraformresource.UnitTest(t, terraformresource.TestCase{
		ProtoV6ProviderFactories: protocolProviderFactories(),
		Steps: []terraformresource.TestStep{{
			Config:             liveAlertingConfig("tf-live-plan", "initial"),
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		}},
	})
}

func TestLiveProviderAlertingLifecycle(t *testing.T) {
	if os.Getenv("O11Y_ACC") != "1" {
		t.Skip("set O11Y_ACC=1 to run the disposable live alerting lifecycle")
	}
	for _, name := range []string{
		"O11Y_ENDPOINT",
		"O11Y_TOKEN",
		"O11Y_TENANT_ID",
		"O11Y_ORG_ID",
		"O11Y_OWNER_TEAM_ID",
		"O11Y_ACC_WEBHOOK_URL",
		"O11Y_ACC_SLACK_CHANNEL_ID",
		"O11Y_ACC_SLACK_TOKEN_REF",
		"O11Y_ACC_CONTACT_EMAIL",
	} {
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
					terraformresource.TestCheckResourceAttrSet("o11y_alert_destination.webhook", "id"),
					terraformresource.TestCheckResourceAttr("o11y_alert_destination.webhook", "name", "Terraform live webhook initial"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_destination.slack", "id"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_contact.live", "id"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_notification_group.live", "id"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_notification_template.live", "published_revision_id"),
					terraformresource.TestCheckResourceAttr("o11y_alert_notification_template.live", "published", "true"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_runbook.live", "current_revision_id"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_notification_policy.live", "id"),
					terraformresource.TestCheckResourceAttr("o11y_alert_notification_policy.live", "enabled", "false"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_notification_policy.live", "config_json"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_maintenance_window.live", "id"),
					terraformresource.TestCheckResourceAttrSet("o11y_alert_silence.live", "id"),
					terraformresource.TestCheckResourceAttrSet("o11y_agent_quality_alert.live", "id"),
					terraformresource.TestCheckResourceAttrSet("o11y_agent_quality_alert.live", "action_json"),
					terraformresource.TestCheckResourceAttrSet("data.o11y_alert_preview.live", "preview_id"),
				),
			},
			{
				Config: liveAlertingConfig(runID, "updated"),
				Check: terraformresource.ComposeAggregateTestCheckFunc(
					terraformresource.TestCheckResourceAttr("o11y_alert_destination.webhook", "name", "Terraform live webhook updated"),
					terraformresource.TestCheckResourceAttr("o11y_alert_destination.slack", "name", "Terraform live Slack updated"),
					terraformresource.TestCheckResourceAttr("o11y_alert_contact.live", "display_name", "Terraform live contact updated"),
					terraformresource.TestCheckResourceAttr("o11y_alert_runbook.live", "title", "Terraform live runbook updated"),
					terraformresource.TestCheckResourceAttr("o11y_agent_quality_alert.live", "name", "Terraform live updated"),
				),
			},
			{ResourceName: "o11y_alert_destination.webhook", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_destination.slack", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_contact.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_notification_group.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_notification_template.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_runbook.live", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"change_reason"}},
			{ResourceName: "o11y_alert_notification_policy.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_maintenance_window.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_alert_silence.live", ImportState: true, ImportStateVerify: true},
			{ResourceName: "o11y_agent_quality_alert.live", ImportState: true, ImportStateVerify: true},
		},
	})
}

func liveAlertingConfig(runID, suffix string) string {
	return strings.NewReplacer(
		"__WEBHOOK_KEY__", fmt.Sprintf("%q", runID+"-webhook"),
		"__WEBHOOK_NAME__", fmt.Sprintf("%q", "Terraform live webhook "+suffix),
		"__WEBHOOK_URL__", fmt.Sprintf("%q", os.Getenv("O11Y_ACC_WEBHOOK_URL")),
		"__SLACK_KEY__", fmt.Sprintf("%q", runID+"-slack"),
		"__SLACK_NAME__", fmt.Sprintf("%q", "Terraform live Slack "+suffix),
		"__SLACK_CHANNEL__", fmt.Sprintf("%q", os.Getenv("O11Y_ACC_SLACK_CHANNEL_ID")),
		"__SLACK_TOKEN_REF__", fmt.Sprintf("%q", os.Getenv("O11Y_ACC_SLACK_TOKEN_REF")),
		"__CONTACT_KEY__", fmt.Sprintf("%q", runID+"-contact"),
		"__CONTACT_NAME__", fmt.Sprintf("%q", "Terraform live contact "+suffix),
		"__CONTACT_EMAIL__", fmt.Sprintf("%q", os.Getenv("O11Y_ACC_CONTACT_EMAIL")),
		"__GROUP_KEY__", fmt.Sprintf("%q", runID+"-human-oncall"),
		"__TEMPLATE_KEY__", fmt.Sprintf("%q", runID+"-template"),
		"__TEMPLATE_NAME__", fmt.Sprintf("%q", "Terraform live template "+suffix),
		"__RUNBOOK_KEY__", fmt.Sprintf("%q", runID+"-runbook"),
		"__RUNBOOK_TITLE__", fmt.Sprintf("%q", "Terraform live runbook "+suffix),
		"__OWNER_TEAM_ID__", fmt.Sprintf("%q", os.Getenv("O11Y_OWNER_TEAM_ID")),
		"__POLICY_KEY__", fmt.Sprintf("%q", runID+"-policy"),
		"__POLICY_NAME__", fmt.Sprintf("%q", "Terraform live policy "+suffix),
		"__WINDOW_KEY__", fmt.Sprintf("%q", runID+"-window"),
		"__SILENCE_KEY__", fmt.Sprintf("%q", runID+"-silence"),
		"__SERVICE__", fmt.Sprintf("%q", runID),
		"__ALERT_SLUG__", fmt.Sprintf("%q", runID+"-quality"),
		"__ALERT_NAME__", fmt.Sprintf("%q", "Terraform live "+suffix),
	).Replace(`
provider "o11y" {
  insecure_skip_verify = true
}

resource "o11y_alert_destination" "webhook" {
  destination_key = __WEBHOOK_KEY__
  name = __WEBHOOK_NAME__
  kind = "webhook"
  enabled = true
  config_json = jsonencode({
    url = __WEBHOOK_URL__
    method = "ALERT_WEBHOOK_METHOD_V1_POST"
  })
  secret_refs_json = jsonencode({})
}

resource "o11y_alert_destination" "slack" {
  destination_key = __SLACK_KEY__
  name = __SLACK_NAME__
  kind = "slack"
  enabled = true
  config_json = jsonencode({ channel_id = __SLACK_CHANNEL__ })
  secret_refs_json = jsonencode({ token = __SLACK_TOKEN_REF__ })
}

resource "o11y_alert_contact" "live" {
  contact_key = __CONTACT_KEY__
  display_name = __CONTACT_NAME__
  email = __CONTACT_EMAIL__
  request_verification = false
}

resource "o11y_alert_notification_group" "live" {
  group_key = __GROUP_KEY__
  name = "Terraform live human on-call"
  enabled = true
  members_json = jsonencode([
    {
      kind = "destination"
      member_id = o11y_alert_destination.slack.id
      position = 0
      enabled = true
    },
    {
      kind = "contact"
      member_id = o11y_alert_contact.live.id
      position = 1
      enabled = true
    }
  ])
}

resource "o11y_alert_notification_template" "live" {
  template_key = __TEMPLATE_KEY__
  name = __TEMPLATE_NAME__
  description = "Terraform live human-message template"
  change_reason = "Exercise the live provider template lifecycle"
  provenance_ref = "internal/provider/live_acceptance_test.go"
  published = true
  document_json = jsonencode({
    schema_version = 1
    subject_template = "[{{ incident.severity }}] {{ incident.title }}"
    firing = {
      title_template = "{{ incident.title }}"
      summary_template = "{{ incident.customer_impact }}"
      blocks = [{
        key = "summary"
        markdown = { text_template = "*Why this fired*\n{{ hypothesis }}" }
      }]
    }
    resolved = {
      title_template = "Resolved: {{ incident.title }}"
      summary_template = "Customer impact recovered."
      blocks = [{
        key = "recovery"
        markdown = { text_template = "*Resolved*\nCustomer impact has recovered." }
      }]
    }
    reminder = {
      title_template = "Reminder: {{ incident.title }}"
      summary_template = "Customer impact is still ongoing."
      blocks = [{
        key = "ongoing-impact"
        markdown = { text_template = "*Ongoing customer impact*\n{{ incident.customer_impact }}" }
      }]
    }
  })
}

resource "o11y_alert_runbook" "live" {
  runbook_key = __RUNBOOK_KEY__
  title = __RUNBOOK_TITLE__
  owner_team_id = __OWNER_TEAM_ID__
  markdown = "# Live acceptance\n\n1. Inspect linked evidence.\n2. Mitigate the regression."
  change_reason = "Exercise the live provider runbook lifecycle"
  provenance_ref = "internal/provider/live_acceptance_test.go"
}

resource "o11y_alert_notification_policy" "live" {
  policy_key = __POLICY_KEY__
  name = __POLICY_NAME__
  enabled = false
  config_json = jsonencode({
    tree = {
      nodes = [
        {
          node_key = "warning"
          node_kind = "ALERT_POLICY_TREE_NODE_KIND_V1_BRANCH"
          matcher = [{ field = "severity", values = [{ string_value = "warning" }] }]
          priority = 10
          enabled = true
          behavior = "ALERT_NOTIFICATION_ROUTE_BEHAVIOR_V1_STOP"
        },
        {
          node_key = "human"
          parent_node_key = "warning"
          node_kind = "ALERT_POLICY_TREE_NODE_KIND_V1_ROUTE"
          target = { group_id = o11y_alert_notification_group.live.id }
          priority = 10
          enabled = true
          behavior = "ALERT_NOTIFICATION_ROUTE_BEHAVIOR_V1_CONTINUE"
          notification_template_id = o11y_alert_notification_template.live.id
          requested_notification_template_revision_id = o11y_alert_notification_template.live.published_revision_id
        },
        {
          node_key = "webhook"
          parent_node_key = "warning"
          node_kind = "ALERT_POLICY_TREE_NODE_KIND_V1_ROUTE"
          target = { destination_id = o11y_alert_destination.webhook.id }
          priority = 20
          enabled = true
          behavior = "ALERT_NOTIFICATION_ROUTE_BEHAVIOR_V1_STOP"
        }
      ]
    }
    grouping = {
      group_by = ["service.name"]
      group_wait_seconds = 0
      group_interval_seconds = 60
    }
    notifications = {
      repeat_interval_seconds = 900
      repeat_limit = 1
      notify_resolved = true
    }
  })
}

resource "o11y_alert_maintenance_window" "live" {
  window_key = __WINDOW_KEY__
  name = "Terraform live maintenance"
  scope_json = jsonencode({ service_names = [__SERVICE__] })
  starts_at = "2030-01-01T00:00:00Z"
  ends_at = "2030-01-01T01:00:00Z"
}

resource "o11y_alert_silence" "live" {
  silence_key = __SILENCE_KEY__
  matcher_json = jsonencode({ clauses = [{ field = "service", values = [{ string_value = __SERVICE__ }] }] })
  reason = "Terraform live acceptance"
  starts_at = "2030-01-01T00:00:00Z"
  ends_at = "2030-01-01T01:00:00Z"
}

resource "o11y_agent_quality_alert" "live" {
  slug = __ALERT_SLUG__
  name = __ALERT_NAME__
  description = "Terraform live lifecycle acceptance"
  severity = "warning"
  scope_json = jsonencode({ service_names = [__SERVICE__] })
  owner_json = jsonencode({ team_id = __OWNER_TEAM_ID__, display_name = "Terraform", active = true })
  action_json = jsonencode({
    first_action = "Inspect current agent quality evidence."
    managed_runbook_id = o11y_alert_runbook.live.id
    managed_runbook_revision_id = o11y_alert_runbook.live.current_revision_id
  })
  evaluation_settings_json = jsonencode({})
  sample_guard_json = jsonencode({ minimum_events = "20" })
  recipe_config_json = jsonencode({ max_bad_outcome_rate = 0.1 })
  paused = false
  notify = false
}

data "o11y_alert_preview" "live" {
  definition_id = o11y_agent_quality_alert.live.id
}
`)
}
