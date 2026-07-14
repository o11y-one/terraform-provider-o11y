package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
)

func TestNotificationGroupMembersAreBoundedAndTyped(t *testing.T) {
	members, err := decodeNotificationGroupMembers(`[
		{"kind":"contact","member_id":"019f7aa2-6c7f-7000-8000-000000000001","position":0},
		{"kind":"destination","member_id":"019f7aa2-6c7f-7000-8000-000000000002","position":1,"enabled":false}
	]`)
	if err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if members[0].Kind != alertsv1.AlertNotificationGroupMemberKindV1_ALERT_NOTIFICATION_GROUP_MEMBER_KIND_V1_CONTACT {
		t.Fatalf("unexpected first member kind: %s", members[0].Kind)
	}
	if members[1].Enabled {
		t.Fatal("explicit disabled member must remain disabled")
	}
	if _, err := decodeNotificationGroupMembers(`[{"kind":"contact","member_id":"019f7aa2-6c7f-7000-8000-000000000001","position":0},{"kind":"contact","member_id":"019f7aa2-6c7f-7000-8000-000000000001","position":1}]`); err == nil {
		t.Fatal("duplicate member must fail")
	}
}

func TestContactAndGroupSchemasExposeRevisionSafeResources(t *testing.T) {
	for name, instance := range map[string]resource.Resource{"contact": NewContactResource(), "group": NewNotificationGroupResource()} {
		var response resource.SchemaResponse
		instance.Schema(context.Background(), resource.SchemaRequest{}, &response)
		if response.Schema.Attributes["revision"] == nil {
			t.Fatalf("%s resource must expose server revision", name)
		}
	}
}
