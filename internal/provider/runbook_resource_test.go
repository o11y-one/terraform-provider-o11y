package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRunbookOwnerUsesExactlyOneProtoAuthority(t *testing.T) {
	user := ownerProto(types.StringValue("019f7aa2-6c7f-7000-8000-000000000001"), types.StringNull())
	if user.GetUserId() == "" || user.GetTeamId() != "" {
		t.Fatalf("unexpected user owner: %#v", user)
	}
	team := ownerProto(types.StringNull(), types.StringValue("019f7aa2-6c7f-7000-8000-000000000002"))
	if team.GetTeamId() == "" || team.GetUserId() != "" {
		t.Fatalf("unexpected team owner: %#v", team)
	}
}

func TestRunbookResourceAndDataSourcesExposeRevisionSafeContracts(t *testing.T) {
	var resourceResponse resource.SchemaResponse
	NewRunbookResource().Schema(context.Background(), resource.SchemaRequest{}, &resourceResponse)
	for _, attribute := range []string{"revision", "current_revision_id", "content_hash", "rendered_html", "archived"} {
		if resourceResponse.Schema.Attributes[attribute] == nil {
			t.Fatalf("runbook resource must expose %s", attribute)
		}
	}

	for name, instance := range map[string]datasource.DataSource{
		"current": NewRunbookDataSource(), "revisions": NewRunbookRevisionsDataSource(),
		"preview": NewRunbookPreviewDataSource(), "usage": NewRunbookUsageDataSource(),
	} {
		var response datasource.SchemaResponse
		instance.Schema(context.Background(), datasource.SchemaRequest{}, &response)
		if len(response.Schema.Attributes) == 0 {
			t.Fatalf("%s runbook data source must expose attributes", name)
		}
	}
}

func TestRunbookContentChangesIgnoreLifecycleOnlyFields(t *testing.T) {
	base := runbookModel{
		RunbookKey: types.StringValue("checkout"), Title: types.StringValue("Checkout"),
		OwnerUserID: types.StringValue("019f7aa2-6c7f-7000-8000-000000000001"), OwnerTeamID: types.StringNull(),
		Markdown: types.StringValue("# Recover"), ProvenanceRef: types.StringValue("module.alerts"),
		Archived: types.BoolValue(false), ChangeReason: types.StringValue("initial"),
	}
	lifecycleOnly := base
	lifecycleOnly.Archived = types.BoolValue(true)
	lifecycleOnly.ChangeReason = types.StringValue("retired")
	if runbookContentChanged(lifecycleOnly, base) {
		t.Fatal("archive and audit reason changes must not create a content revision")
	}
	changed := base
	changed.Markdown = types.StringValue("# Recover\n\nInspect traces")
	if !runbookContentChanged(changed, base) {
		t.Fatal("markdown change must create a content revision")
	}
}
