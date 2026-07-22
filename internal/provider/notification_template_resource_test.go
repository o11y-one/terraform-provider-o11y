package provider

import (
	"strings"
	"testing"

	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
)

func TestValidateNotificationTemplateDocumentRequiresEveryLifecycleVariant(t *testing.T) {
	variant := validNotificationTemplateVariant()
	document := &alertsv1.NotificationTemplateDocumentV1{
		SchemaVersion: 1,
		Firing:        variant,
		Resolved:      variant,
	}
	if err := validateNotificationTemplateDocument(document); err == nil || !strings.Contains(err.Error(), "document.reminder") {
		t.Fatalf("missing reminder error = %v", err)
	}

	document.Reminder = variant
	if err := validateNotificationTemplateDocument(document); err != nil {
		t.Fatalf("complete lifecycle document rejected: %v", err)
	}
}

func TestValidateNotificationTemplateDocumentRequiresCompleteLocalizations(t *testing.T) {
	variant := validNotificationTemplateVariant()
	document := &alertsv1.NotificationTemplateDocumentV1{
		SchemaVersion: 1,
		Firing:        variant,
		Resolved:      variant,
		Reminder:      variant,
		Localizations: []*alertsv1.NotificationTemplateLocalizationV1{
			{Locale: "en-GB", Firing: variant, Resolved: variant},
		},
	}
	if err := validateNotificationTemplateDocument(document); err == nil || !strings.Contains(err.Error(), "localizations[0].reminder") {
		t.Fatalf("missing localized reminder error = %v", err)
	}

	document.Localizations[0].Reminder = variant
	if err := validateNotificationTemplateDocument(document); err != nil {
		t.Fatalf("complete localized document rejected: %v", err)
	}
}

func TestValidateNotificationTemplateDocumentRequiresBlocksForEveryVariant(t *testing.T) {
	valid := validNotificationTemplateVariant()
	document := &alertsv1.NotificationTemplateDocumentV1{
		SchemaVersion: 1,
		Firing:        valid,
		Resolved:      &alertsv1.NotificationTemplateVariantV1{TitleTemplate: "Resolved"},
		Reminder:      valid,
	}
	if err := validateNotificationTemplateDocument(document); err == nil || !strings.Contains(err.Error(), "document.resolved must contain 1..=32 blocks") {
		t.Fatalf("missing resolved blocks error = %v", err)
	}

	document.Resolved.Blocks = make([]*alertsv1.NotificationTemplateBlockV1, 33)
	if err := validateNotificationTemplateDocument(document); err == nil || !strings.Contains(err.Error(), "document.resolved must contain 1..=32 blocks") {
		t.Fatalf("excess resolved blocks error = %v", err)
	}
}

func validNotificationTemplateVariant() *alertsv1.NotificationTemplateVariantV1 {
	return &alertsv1.NotificationTemplateVariantV1{
		TitleTemplate:   "Alert",
		SummaryTemplate: "Customer impact",
		Blocks: []*alertsv1.NotificationTemplateBlockV1{
			{
				Key: "summary",
				Content: &alertsv1.NotificationTemplateBlockV1_Markdown{
					Markdown: &alertsv1.NotificationTemplateMarkdownBlockV1{TextTemplate: "Customer impact"},
				},
			},
		},
	}
}
