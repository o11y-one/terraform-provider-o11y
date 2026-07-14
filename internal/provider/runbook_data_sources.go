package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/o11y-one/terraform-provider-o11y/internal/client"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
)

var _ datasource.DataSourceWithConfigure = &runbookDataSource{}
var _ datasource.DataSourceWithConfigure = &runbookRevisionsDataSource{}
var _ datasource.DataSourceWithConfigure = &runbookPreviewDataSource{}
var _ datasource.DataSourceWithConfigure = &runbookUsageDataSource{}

type runbookDataSource struct{ client *client.Client }
type runbookRevisionsDataSource struct{ client *client.Client }
type runbookPreviewDataSource struct{ client *client.Client }
type runbookUsageDataSource struct{ client *client.Client }

type runbookDataModel struct {
	RunbookID           types.String `tfsdk:"runbook_id"`
	RunbookKey          types.String `tfsdk:"runbook_key"`
	Title               types.String `tfsdk:"title"`
	OwnerUserID         types.String `tfsdk:"owner_user_id"`
	OwnerTeamID         types.String `tfsdk:"owner_team_id"`
	CurrentRevisionID   types.String `tfsdk:"current_revision_id"`
	Revision            types.Int64  `tfsdk:"revision"`
	Markdown            types.String `tfsdk:"markdown"`
	RenderedHTML        types.String `tfsdk:"rendered_html"`
	ContentHash         types.String `tfsdk:"content_hash"`
	Provenance          types.String `tfsdk:"provenance"`
	ProvenanceRef       types.String `tfsdk:"provenance_ref"`
	Archived            types.Bool   `tfsdk:"archived"`
	CurrentChangeReason types.String `tfsdk:"current_change_reason"`
}

type runbookRevisionsModel struct {
	RunbookID     types.String `tfsdk:"runbook_id"`
	Limit         types.Int64  `tfsdk:"limit"`
	RevisionsJSON types.String `tfsdk:"revisions_json"`
}

type runbookPreviewModel struct {
	Markdown           types.String `tfsdk:"markdown"`
	NormalizedMarkdown types.String `tfsdk:"normalized_markdown"`
	RenderedHTML       types.String `tfsdk:"rendered_html"`
}

type runbookUsageModel struct {
	RunbookID             types.String `tfsdk:"runbook_id"`
	DefinitionIDs         types.List   `tfsdk:"definition_ids"`
	DefinitionRevisionIDs types.List   `tfsdk:"definition_revision_ids"`
	IncidentIDs           types.List   `tfsdk:"incident_ids"`
}

func NewRunbookDataSource() datasource.DataSource          { return &runbookDataSource{} }
func NewRunbookRevisionsDataSource() datasource.DataSource { return &runbookRevisionsDataSource{} }
func NewRunbookPreviewDataSource() datasource.DataSource   { return &runbookPreviewDataSource{} }
func NewRunbookUsageDataSource() datasource.DataSource     { return &runbookUsageDataSource{} }

func (d *runbookDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_runbook"
}

func (d *runbookDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads the current authoritative revision of a managed alert runbook.", Attributes: map[string]schema.Attribute{
		"runbook_id": schema.StringAttribute{Required: true}, "runbook_key": schema.StringAttribute{Computed: true}, "title": schema.StringAttribute{Computed: true},
		"owner_user_id": schema.StringAttribute{Computed: true}, "owner_team_id": schema.StringAttribute{Computed: true}, "current_revision_id": schema.StringAttribute{Computed: true},
		"revision": schema.Int64Attribute{Computed: true}, "markdown": schema.StringAttribute{Computed: true}, "rendered_html": schema.StringAttribute{Computed: true},
		"content_hash": schema.StringAttribute{Computed: true}, "provenance": schema.StringAttribute{Computed: true}, "provenance_ref": schema.StringAttribute{Computed: true},
		"archived": schema.BoolAttribute{Computed: true}, "current_change_reason": schema.StringAttribute{Computed: true},
	}}
}

func (d *runbookDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredDataSourceClient(req, &resp.Diagnostics)
}

func (d *runbookDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data runbookDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := d.client.Context(ctx)
	defer cancel()
	item, err := d.client.Runbooks.GetRunbook(rpcCtx, &alertsv1.GetAlertRunbookRequest{OrgId: d.client.OrgID(), RunbookId: data.RunbookID.ValueString()})
	if err != nil {
		addRPCError(&resp.Diagnostics, "get runbook", err)
		return
	}
	data.RunbookKey = types.StringValue(item.RunbookKey)
	data.Title = types.StringValue(item.Title)
	setOwner(&data.OwnerUserID, &data.OwnerTeamID, item.Owner)
	data.CurrentRevisionID = types.StringValue(item.CurrentRevisionId)
	data.Revision = types.Int64Value(item.Revision)
	data.Provenance = types.StringValue(item.Provenance)
	data.Archived = types.BoolValue(item.ArchivedAt != nil)
	if item.ProvenanceRef == "" {
		data.ProvenanceRef = types.StringNull()
	} else {
		data.ProvenanceRef = types.StringValue(item.ProvenanceRef)
	}
	if current := item.CurrentRevision; current != nil {
		data.Markdown = types.StringValue(current.Markdown)
		data.RenderedHTML = types.StringValue(current.RenderedHtml)
		data.ContentHash = types.StringValue(current.ContentHash)
		data.CurrentChangeReason = types.StringValue(current.ChangeReason)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *runbookRevisionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_runbook_revisions"
}

func (d *runbookRevisionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Lists immutable revision metadata and content for a managed alert runbook as canonical JSON.", Attributes: map[string]schema.Attribute{
		"runbook_id": schema.StringAttribute{Required: true}, "limit": schema.Int64Attribute{Optional: true, Description: "Maximum revisions, from 1 to 200. Defaults to 50."},
		"revisions_json": schema.StringAttribute{Computed: true, Description: "Revision history ordered newest first."},
	}}
}

func (d *runbookRevisionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredDataSourceClient(req, &resp.Diagnostics)
}

func (d *runbookRevisionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data runbookRevisionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit := int32(50)
	if !data.Limit.IsNull() && !data.Limit.IsUnknown() {
		if data.Limit.ValueInt64() < 1 || data.Limit.ValueInt64() > 200 {
			resp.Diagnostics.AddError("Invalid revision limit", "limit must be between 1 and 200")
			return
		}
		limit = int32(data.Limit.ValueInt64())
	}
	rpcCtx, cancel := d.client.Context(ctx)
	defer cancel()
	result, err := d.client.Runbooks.ListRunbookRevisions(rpcCtx, &alertsv1.ListAlertRunbookRevisionsRequest{OrgId: d.client.OrgID(), RunbookId: data.RunbookID.ValueString(), Limit: limit})
	if err != nil {
		addRPCError(&resp.Diagnostics, "list runbook revisions", err)
		return
	}
	revisions := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		revisions = append(revisions, revisionDocument(item))
	}
	raw, err := json.Marshal(revisions)
	if err != nil {
		resp.Diagnostics.AddError("Unable to encode runbook revisions", err.Error())
		return
	}
	data.Limit = types.Int64Value(int64(limit))
	data.RevisionsJSON = types.StringValue(string(raw))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *runbookPreviewDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_runbook_preview"
}

func (d *runbookPreviewDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Validates and safely renders runbook Markdown without persisting it.", Attributes: map[string]schema.Attribute{
		"markdown": schema.StringAttribute{Required: true}, "normalized_markdown": schema.StringAttribute{Computed: true}, "rendered_html": schema.StringAttribute{Computed: true},
	}}
}

func (d *runbookPreviewDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredDataSourceClient(req, &resp.Diagnostics)
}

func (d *runbookPreviewDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data runbookPreviewModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := d.client.Context(ctx)
	defer cancel()
	result, err := d.client.Runbooks.PreviewRunbook(rpcCtx, &alertsv1.PreviewAlertRunbookRequest{Markdown: data.Markdown.ValueString()})
	if err != nil {
		addRPCError(&resp.Diagnostics, "preview runbook", err)
		return
	}
	data.NormalizedMarkdown = types.StringValue(result.NormalizedMarkdown)
	data.RenderedHTML = types.StringValue(result.RenderedHtml)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *runbookUsageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_runbook_usage"
}

func (d *runbookUsageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads definitions, exact definition revisions, and incidents linked to a managed runbook.", Attributes: map[string]schema.Attribute{
		"runbook_id":              schema.StringAttribute{Required: true},
		"definition_ids":          schema.ListAttribute{Computed: true, ElementType: types.StringType},
		"definition_revision_ids": schema.ListAttribute{Computed: true, ElementType: types.StringType},
		"incident_ids":            schema.ListAttribute{Computed: true, ElementType: types.StringType},
	}}
}

func (d *runbookUsageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredDataSourceClient(req, &resp.Diagnostics)
}

func (d *runbookUsageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data runbookUsageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := d.client.Context(ctx)
	defer cancel()
	result, err := d.client.Runbooks.GetRunbookUsage(rpcCtx, &alertsv1.GetAlertRunbookUsageRequest{OrgId: d.client.OrgID(), RunbookId: data.RunbookID.ValueString()})
	if err != nil {
		addRPCError(&resp.Diagnostics, "get runbook usage", err)
		return
	}
	var listDiags diag.Diagnostics
	data.DefinitionIDs, listDiags = types.ListValueFrom(ctx, types.StringType, result.DefinitionIds)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.DefinitionRevisionIDs, listDiags = types.ListValueFrom(ctx, types.StringType, result.DefinitionRevisionIds)
	resp.Diagnostics.Append(listDiags...)
	data.IncidentIDs, listDiags = types.ListValueFrom(ctx, types.StringType, result.IncidentIds)
	resp.Diagnostics.Append(listDiags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	}
}

func configuredDataSourceClient(req datasource.ConfigureRequest, diagnostics *diag.Diagnostics) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return nil
	}
	return c
}

func revisionDocument(item *alertsv1.AlertRunbookRevisionV1) map[string]any {
	ownerUser, ownerTeam := "", ""
	if item.Owner != nil {
		ownerUser, ownerTeam = item.Owner.GetUserId(), item.Owner.GetTeamId()
	}
	return map[string]any{
		"id": item.Id, "runbook_id": item.RunbookId, "revision_number": item.RevisionNumber,
		"title": item.Title, "owner_user_id": ownerUser, "owner_team_id": ownerTeam,
		"markdown": item.Markdown, "rendered_html": item.RenderedHtml, "content_hash": item.ContentHash,
		"change_reason": item.ChangeReason, "created_by": item.CreatedBy, "created_at": timestampString(item.CreatedAt).ValueString(),
	}
}
