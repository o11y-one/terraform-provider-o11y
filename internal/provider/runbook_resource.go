package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/o11y-one/terraform-provider-o11y/internal/client"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
	"github.com/o11y-one/terraform-provider-o11y/internal/idempotency"
	validationutil "github.com/o11y-one/terraform-provider-o11y/internal/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var _ resource.ResourceWithConfigure = &runbookResource{}
var _ resource.ResourceWithImportState = &runbookResource{}
var _ resource.ResourceWithModifyPlan = &runbookResource{}
var _ resource.ResourceWithValidateConfig = &runbookResource{}

type runbookResource struct{ client *client.Client }

type runbookModel struct {
	ID                types.String `tfsdk:"id"`
	RunbookKey        types.String `tfsdk:"runbook_key"`
	Title             types.String `tfsdk:"title"`
	OwnerUserID       types.String `tfsdk:"owner_user_id"`
	OwnerTeamID       types.String `tfsdk:"owner_team_id"`
	Markdown          types.String `tfsdk:"markdown"`
	ChangeReason      types.String `tfsdk:"change_reason"`
	ProvenanceRef     types.String `tfsdk:"provenance_ref"`
	CurrentRevisionID types.String `tfsdk:"current_revision_id"`
	Revision          types.Int64  `tfsdk:"revision"`
	ContentHash       types.String `tfsdk:"content_hash"`
	RenderedHTML      types.String `tfsdk:"rendered_html"`
	Archived          types.Bool   `tfsdk:"archived"`
	CreatedAt         types.String `tfsdk:"created_at"`
	UpdatedAt         types.String `tfsdk:"updated_at"`
}

func NewRunbookResource() resource.Resource { return &runbookResource{} }

func (r *runbookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_runbook"
}

func (r *runbookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A versioned managed alert runbook. Content is sanitized by O11y.one and alert/incident links retain the exact revision they were created with.",
		Attributes: map[string]schema.Attribute{
			"id":                  schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"runbook_key":         schema.StringAttribute{Required: true, Description: "Stable immutable key. Archive the runbook rather than reusing this key."},
			"title":               schema.StringAttribute{Required: true},
			"owner_user_id":       schema.StringAttribute{Optional: true, Description: "Exactly one of owner_user_id or owner_team_id is required."},
			"owner_team_id":       schema.StringAttribute{Optional: true, Description: "Exactly one of owner_user_id or owner_team_id is required."},
			"markdown":            schema.StringAttribute{Required: true, Description: "Runbook Markdown. Raw HTML, unsafe links, and active content are rejected."},
			"change_reason":       schema.StringAttribute{Required: true, Description: "Audited reason for the current desired content."},
			"provenance_ref":      schema.StringAttribute{Optional: true, Description: "Optional source repository, module, or control-plane reference."},
			"current_revision_id": schema.StringAttribute{Computed: true},
			"revision":            schema.Int64Attribute{Computed: true},
			"content_hash":        schema.StringAttribute{Computed: true},
			"rendered_html":       schema.StringAttribute{Computed: true, Description: "Sanitized server-rendered HTML."},
			"archived":            schema.BoolAttribute{Optional: true, Computed: true, Description: "Archive or restore the runbook without deleting immutable revisions."},
			"created_at":          schema.StringAttribute{Computed: true},
			"updated_at":          schema.StringAttribute{Computed: true},
		},
	}
}

func (r *runbookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *runbookResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var data runbookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !data.Markdown.IsUnknown() && !data.Markdown.IsNull() {
		data.Markdown = types.StringValue(canonicalRunbookMarkdown(data.Markdown.ValueString()))
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &data)...)
}

func (r *runbookResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data runbookModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, value := range map[string]types.String{
		"runbook_key":   data.RunbookKey,
		"title":         data.Title,
		"markdown":      data.Markdown,
		"change_reason": data.ChangeReason,
	} {
		if !value.IsUnknown() && !value.IsNull() {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	ownerUser := knownNonEmpty(data.OwnerUserID)
	ownerTeam := knownNonEmpty(data.OwnerTeamID)
	if !data.OwnerUserID.IsUnknown() && !data.OwnerTeamID.IsUnknown() && ownerUser == ownerTeam {
		resp.Diagnostics.AddError("Invalid runbook owner", "exactly one of owner_user_id or owner_team_id must be configured")
	}
	for name, value := range map[string]types.String{"owner_user_id": data.OwnerUserID, "owner_team_id": data.OwnerTeamID} {
		if knownNonEmpty(value) {
			if err := validationutil.UUID(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid owner reference", err.Error())
			}
		}
	}
}

func (r *runbookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data runbookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Runbooks.CreateRunbook(rpcCtx, r.request(&data, "create", data.RunbookKey.ValueString(), nil))
	if err != nil {
		addRPCError(&resp.Diagnostics, "create runbook", err)
		return
	}
	if !data.Archived.IsNull() && !data.Archived.IsUnknown() && data.Archived.ValueBool() {
		item, err = r.setArchived(ctx, item, true, data.ChangeReason.ValueString(), "create-archive")
		if err != nil {
			addRPCError(&resp.Diagnostics, "archive new runbook", err)
			return
		}
	}
	setRunbook(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *runbookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data runbookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Runbooks.GetRunbook(rpcCtx, &alertsv1.GetAlertRunbookRequest{OrgId: r.client.OrgID(), RunbookId: data.ID.ValueString()})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		addRPCError(&resp.Diagnostics, "get runbook", err)
		return
	}
	changeReason := data.ChangeReason
	setRunbook(&data, item)
	data.ChangeReason = changeReason
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *runbookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state runbookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = state.ID
	stateArchived := !state.Archived.IsNull() && state.Archived.ValueBool()
	planArchived := !data.Archived.IsNull() && data.Archived.ValueBool()
	if stateArchived && planArchived && runbookContentChanged(data, state) {
		resp.Diagnostics.AddError("Archived runbook is immutable", "set archived = false and apply before changing runbook content")
		return
	}
	currentRevision := state.Revision.ValueInt64()
	var item *alertsv1.AlertRunbookV1
	var err error
	if stateArchived && !planArchived {
		item, err = r.setArchived(ctx, runbookProtoFromState(state), false, data.ChangeReason.ValueString(), "restore")
		if err != nil {
			addRPCError(&resp.Diagnostics, "restore runbook", err)
			return
		}
		currentRevision = item.Revision
	}
	if runbookContentChanged(data, state) {
		rpcCtx, cancel := r.client.Context(ctx)
		item, err = r.client.Runbooks.UpdateRunbook(rpcCtx, r.request(&data, "update", state.ID.ValueString(), proto.Int64(currentRevision)))
		cancel()
		if err != nil {
			addRPCError(&resp.Diagnostics, "update runbook", err)
			return
		}
		currentRevision = item.Revision
	}
	if !stateArchived && planArchived {
		if item == nil {
			item = runbookProtoFromState(state)
			item.Revision = currentRevision
		}
		item, err = r.setArchived(ctx, item, true, data.ChangeReason.ValueString(), "archive")
		if err != nil {
			addRPCError(&resp.Diagnostics, "archive runbook", err)
			return
		}
	}
	if item == nil {
		rpcCtx, cancel := r.client.Context(ctx)
		item, err = r.client.Runbooks.GetRunbook(rpcCtx, &alertsv1.GetAlertRunbookRequest{OrgId: r.client.OrgID(), RunbookId: state.ID.ValueString()})
		cancel()
		if err != nil {
			addRPCError(&resp.Diagnostics, "refresh runbook", err)
			return
		}
	}
	setRunbook(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *runbookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data runbookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.setArchived(ctx, runbookProtoFromState(data), true, "removed from Terraform configuration", "delete-archive")
	if err != nil && status.Code(err) != codes.NotFound {
		addRPCError(&resp.Diagnostics, "archive runbook", err)
	}
}

func (r *runbookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *runbookResource) request(data *runbookModel, operation, identity string, expected *int64) *alertsv1.UpsertAlertRunbookRequest {
	message := &alertsv1.UpsertAlertRunbookRequest{
		Id: data.ID.ValueString(), OrgId: r.client.OrgID(), RunbookKey: data.RunbookKey.ValueString(),
		Title: data.Title.ValueString(), Owner: ownerProto(data.OwnerUserID, data.OwnerTeamID), Markdown: canonicalRunbookMarkdown(data.Markdown.ValueString()),
		ExpectedRevision: expected, ChangeReason: data.ChangeReason.ValueString(), Provenance: "terraform", ProvenanceRef: stringValue(data.ProvenanceRef),
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "runbook", operation, identity, message.RunbookKey, message.Title, message.Markdown, message.ChangeReason, message.ProvenanceRef)
	return message
}

func (r *runbookResource) setArchived(ctx context.Context, item *alertsv1.AlertRunbookV1, archived bool, reason, operation string) (*alertsv1.AlertRunbookV1, error) {
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	return r.client.Runbooks.SetRunbookArchived(rpcCtx, &alertsv1.SetAlertRunbookArchivedRequest{
		OrgId: r.client.OrgID(), RunbookId: item.Id, ExpectedRevision: item.Revision, Archived: archived, Reason: reason,
		IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "runbook", operation, item.Id, fmt.Sprint(item.Revision), fmt.Sprint(archived)),
	})
}

func setRunbook(data *runbookModel, item *alertsv1.AlertRunbookV1) {
	data.ID = types.StringValue(item.Id)
	data.RunbookKey = types.StringValue(item.RunbookKey)
	data.Title = types.StringValue(item.Title)
	setOwner(&data.OwnerUserID, &data.OwnerTeamID, item.Owner)
	data.CurrentRevisionID = types.StringValue(item.CurrentRevisionId)
	data.Revision = types.Int64Value(item.Revision)
	data.Archived = types.BoolValue(item.ArchivedAt != nil)
	data.CreatedAt = timestampString(item.CreatedAt)
	data.UpdatedAt = timestampString(item.UpdatedAt)
	if revision := item.CurrentRevision; revision != nil {
		data.Markdown = types.StringValue(revision.Markdown)
		data.ContentHash = types.StringValue(revision.ContentHash)
		data.RenderedHTML = types.StringValue(revision.RenderedHtml)
	} else {
		data.Markdown = types.StringNull()
		data.ContentHash = types.StringNull()
		data.RenderedHTML = types.StringNull()
	}
	if item.ProvenanceRef == "" {
		data.ProvenanceRef = types.StringNull()
	} else {
		data.ProvenanceRef = types.StringValue(item.ProvenanceRef)
	}
}

func runbookContentChanged(plan, state runbookModel) bool {
	return plan.RunbookKey.ValueString() != state.RunbookKey.ValueString() ||
		plan.Title.ValueString() != state.Title.ValueString() ||
		stringValue(plan.OwnerUserID) != stringValue(state.OwnerUserID) ||
		stringValue(plan.OwnerTeamID) != stringValue(state.OwnerTeamID) ||
		canonicalRunbookMarkdown(plan.Markdown.ValueString()) != canonicalRunbookMarkdown(state.Markdown.ValueString()) ||
		stringValue(plan.ProvenanceRef) != stringValue(state.ProvenanceRef)
}

func canonicalRunbookMarkdown(markdown string) string {
	return strings.TrimSpace(markdown)
}

func runbookProtoFromState(data runbookModel) *alertsv1.AlertRunbookV1 {
	return &alertsv1.AlertRunbookV1{Id: data.ID.ValueString(), Revision: data.Revision.ValueInt64()}
}

func ownerProto(user, team types.String) *alertsv1.AlertOwnerRefV1 {
	if value := stringValue(user); value != "" {
		return &alertsv1.AlertOwnerRefV1{Owner: &alertsv1.AlertOwnerRefV1_UserId{UserId: value}}
	}
	return &alertsv1.AlertOwnerRefV1{Owner: &alertsv1.AlertOwnerRefV1_TeamId{TeamId: stringValue(team)}}
}

func setOwner(user, team *types.String, owner *alertsv1.AlertOwnerRefV1) {
	*user, *team = types.StringNull(), types.StringNull()
	if owner == nil {
		return
	}
	if owner.GetUserId() != "" {
		*user = types.StringValue(owner.GetUserId())
	}
	if owner.GetTeamId() != "" {
		*team = types.StringValue(owner.GetTeamId())
	}
}

func knownNonEmpty(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueString() != ""
}

func stringValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}
