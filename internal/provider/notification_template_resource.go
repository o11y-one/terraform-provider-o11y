package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/o11y-one/terraform-provider-o11y/internal/client"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
	"github.com/o11y-one/terraform-provider-o11y/internal/idempotency"
	validationutil "github.com/o11y-one/terraform-provider-o11y/internal/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var _ resource.ResourceWithConfigure = &notificationTemplateResource{}
var _ resource.ResourceWithImportState = &notificationTemplateResource{}
var _ resource.ResourceWithValidateConfig = &notificationTemplateResource{}

type notificationTemplateResource struct{ client *client.Client }

type notificationTemplateModel struct {
	ID                  types.String `tfsdk:"id"`
	TemplateKey         types.String `tfsdk:"template_key"`
	Name                types.String `tfsdk:"name"`
	Description         types.String `tfsdk:"description"`
	Document            types.String `tfsdk:"document_json"`
	ChangeReason        types.String `tfsdk:"change_reason"`
	ProvenanceRef       types.String `tfsdk:"provenance_ref"`
	Publish             types.Bool   `tfsdk:"published"`
	Archived            types.Bool   `tfsdk:"archived"`
	Revision            types.Int64  `tfsdk:"revision"`
	CurrentRevisionID   types.String `tfsdk:"current_revision_id"`
	PublishedRevisionID types.String `tfsdk:"published_revision_id"`
	ContentHash         types.String `tfsdk:"content_hash"`
	CreatedAt           types.String `tfsdk:"created_at"`
	UpdatedAt           types.String `tfsdk:"updated_at"`
}

func NewNotificationTemplateResource() resource.Resource { return &notificationTemplateResource{} }

func (r *notificationTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_notification_template"
}

func (r *notificationTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "A revisioned reusable Email and Slack notification template. Publishing is explicit and policies bind immutable published revisions.", Attributes: map[string]schema.Attribute{
		"id":                    schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"template_key":          schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, Description: "Stable immutable user-template key."},
		"name":                  schema.StringAttribute{Required: true},
		"description":           schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
		"document_json":         schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{canonicalJSONPlanModifier{}}, Description: "Typed NotificationTemplateDocumentV1 protobuf JSON. Unknown variables or block shapes fail closed."},
		"change_reason":         schema.StringAttribute{Required: true, Description: "Audited reason for the desired revision or lifecycle change."},
		"provenance_ref":        schema.StringAttribute{Optional: true, Description: "Optional source repository, module, or control-plane reference."},
		"published":             schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Publish the current revision. Publishing never silently changes existing policy bindings."},
		"archived":              schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
		"revision":              schema.Int64Attribute{Computed: true, Description: "Monotonic optimistic-concurrency revision."},
		"current_revision_id":   schema.StringAttribute{Computed: true},
		"published_revision_id": schema.StringAttribute{Computed: true, Description: "Immutable revision currently available for new policy bindings."},
		"content_hash":          schema.StringAttribute{Computed: true},
		"created_at":            schema.StringAttribute{Computed: true},
		"updated_at":            schema.StringAttribute{Computed: true},
	}}
}

func (r *notificationTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *notificationTemplateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data notificationTemplateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, value := range map[string]types.String{"template_key": data.TemplateKey, "name": data.Name, "change_reason": data.ChangeReason} {
		if knownNonEmpty(value) {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	if knownNonEmpty(data.Document) {
		document := &alertsv1.NotificationTemplateDocumentV1{}
		if err := protoFromJSON(data.Document, document); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("document_json"), "Invalid typed notification template", err.Error())
		} else if err := validateNotificationTemplateDocument(document); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("document_json"), "Incomplete notification template", err.Error())
		}
	}
}

func (r *notificationTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data notificationTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	document, err := notificationTemplateDocument(data.Document)
	if err != nil {
		addRPCError(&resp.Diagnostics, "build notification template", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	item, err := r.client.Notifications.CreateNotificationTemplate(rpcCtx, r.upsertRequest(&data, document, "create", data.TemplateKey.ValueString(), nil))
	cancel()
	if err != nil {
		addRPCError(&resp.Diagnostics, "create notification template", err)
		return
	}
	if data.Publish.ValueBool() {
		item, err = r.publish(ctx, item, data.ChangeReason.ValueString(), "create-publish")
		if err != nil {
			addRPCError(&resp.Diagnostics, "publish notification template", err)
			return
		}
	}
	if data.Archived.ValueBool() {
		item, err = r.setArchived(ctx, item, true, data.ChangeReason.ValueString(), "create-archive")
		if err != nil {
			addRPCError(&resp.Diagnostics, "archive notification template", err)
			return
		}
	}
	configuredReason := data.ChangeReason
	setNotificationTemplate(&data, item)
	if knownNonEmpty(configuredReason) {
		data.ChangeReason = configuredReason
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *notificationTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data notificationTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.GetNotificationTemplate(rpcCtx, &alertsv1.GetAlertNotificationTemplateRequest{OrgId: r.client.OrgID(), NotificationTemplateId: data.ID.ValueString()})
	if status.Code(err) == codes.NotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addRPCError(&resp.Diagnostics, "get notification template", err)
		return
	}
	configuredReason := data.ChangeReason
	setNotificationTemplate(&data, item)
	if knownNonEmpty(configuredReason) {
		data.ChangeReason = configuredReason
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *notificationTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state notificationTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = state.ID
	stateArchived, planArchived := state.Archived.ValueBool(), data.Archived.ValueBool()
	contentChanged := notificationTemplateContentChanged(data, state)
	if stateArchived && planArchived && contentChanged {
		resp.Diagnostics.AddError("Archived notification template is immutable", "set archived = false and apply before changing template content")
		return
	}
	item := notificationTemplateProtoFromState(state)
	mutated := false
	var err error
	if stateArchived && !planArchived {
		item, err = r.setArchived(ctx, item, false, data.ChangeReason.ValueString(), "restore")
		if err != nil {
			addRPCError(&resp.Diagnostics, "restore notification template", err)
			return
		}
		mutated = true
	}
	if contentChanged {
		document, decodeErr := notificationTemplateDocument(data.Document)
		if decodeErr != nil {
			addRPCError(&resp.Diagnostics, "build notification template", decodeErr)
			return
		}
		rpcCtx, cancel := r.client.Context(ctx)
		item, err = r.client.Notifications.UpdateNotificationTemplate(rpcCtx, r.upsertRequest(&data, document, "update", state.ID.ValueString(), proto.Int64(item.Revision)))
		cancel()
		if err != nil {
			addRPCError(&resp.Diagnostics, "update notification template", err)
			return
		}
		mutated = true
	}
	currentPublished := item.CurrentRevisionId != "" && item.CurrentRevisionId == item.PublishedRevisionId
	if data.Publish.ValueBool() && !currentPublished {
		item, err = r.publish(ctx, item, data.ChangeReason.ValueString(), "publish")
		if err != nil {
			addRPCError(&resp.Diagnostics, "publish notification template", err)
			return
		}
		mutated = true
	} else if !data.Publish.ValueBool() && currentPublished && !contentChanged {
		resp.Diagnostics.AddError("Published revision cannot be unpublished", "change document_json to create a new draft or keep published = true")
		return
	}
	if !stateArchived && planArchived {
		item, err = r.setArchived(ctx, item, true, data.ChangeReason.ValueString(), "archive")
		if err != nil {
			addRPCError(&resp.Diagnostics, "archive notification template", err)
			return
		}
		mutated = true
	}
	if !mutated {
		rpcCtx, cancel := r.client.Context(ctx)
		item, err = r.client.Notifications.GetNotificationTemplate(rpcCtx, &alertsv1.GetAlertNotificationTemplateRequest{OrgId: r.client.OrgID(), NotificationTemplateId: state.ID.ValueString()})
		cancel()
		if err != nil {
			addRPCError(&resp.Diagnostics, "refresh notification template", err)
			return
		}
	}
	desiredChangeReason := data.ChangeReason
	setNotificationTemplate(&data, item)
	data.ChangeReason = desiredChangeReason
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *notificationTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data notificationTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.setArchived(ctx, notificationTemplateProtoFromState(data), true, "removed from Terraform configuration", "delete-archive")
	if err != nil && status.Code(err) != codes.NotFound {
		addRPCError(&resp.Diagnostics, "archive notification template", err)
	}
}

func (r *notificationTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *notificationTemplateResource) upsertRequest(data *notificationTemplateModel, document *alertsv1.NotificationTemplateDocumentV1, operation, identity string, expected *int64) *alertsv1.UpsertAlertNotificationTemplateRequest {
	message := &alertsv1.UpsertAlertNotificationTemplateRequest{Id: data.ID.ValueString(), OrgId: r.client.OrgID(), TemplateKey: data.TemplateKey.ValueString(), Name: data.Name.ValueString(), Description: stringValue(data.Description), Document: document, ExpectedRevision: expected, ChangeReason: data.ChangeReason.ValueString(), Provenance: "terraform", ProvenanceRef: stringValue(data.ProvenanceRef)}
	payload, _ := protojson.Marshal(message)
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "notification_template", operation, identity, string(payload))
	return message
}

func (r *notificationTemplateResource) publish(ctx context.Context, item *alertsv1.AlertNotificationTemplateV1, reason, operation string) (*alertsv1.AlertNotificationTemplateV1, error) {
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	return r.client.Notifications.PublishNotificationTemplate(rpcCtx, &alertsv1.PublishAlertNotificationTemplateRequest{OrgId: r.client.OrgID(), NotificationTemplateId: item.Id, ExpectedRevision: item.Revision, Reason: reason, IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "notification_template", operation, item.Id, fmt.Sprint(item.Revision))})
}

func (r *notificationTemplateResource) setArchived(ctx context.Context, item *alertsv1.AlertNotificationTemplateV1, archived bool, reason, operation string) (*alertsv1.AlertNotificationTemplateV1, error) {
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	return r.client.Notifications.SetNotificationTemplateArchived(rpcCtx, &alertsv1.SetAlertNotificationTemplateArchivedRequest{OrgId: r.client.OrgID(), NotificationTemplateId: item.Id, ExpectedRevision: item.Revision, Archived: archived, Reason: reason, IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "notification_template", operation, item.Id, fmt.Sprint(item.Revision), fmt.Sprint(archived))})
}

func notificationTemplateDocument(value types.String) (*alertsv1.NotificationTemplateDocumentV1, error) {
	document := &alertsv1.NotificationTemplateDocumentV1{}
	if err := protoFromJSON(value, document); err != nil {
		return nil, err
	}
	if err := validateNotificationTemplateDocument(document); err != nil {
		return nil, err
	}
	return document, nil
}

func validateNotificationTemplateDocument(document *alertsv1.NotificationTemplateDocumentV1) error {
	if document == nil {
		return fmt.Errorf("document is required")
	}
	if document.Firing == nil {
		return fmt.Errorf("document.firing is required")
	}
	if document.Resolved == nil {
		return fmt.Errorf("document.resolved is required")
	}
	if document.Reminder == nil {
		return fmt.Errorf("document.reminder is required")
	}
	for name, variant := range map[string]*alertsv1.NotificationTemplateVariantV1{
		"document.firing":   document.Firing,
		"document.resolved": document.Resolved,
		"document.reminder": document.Reminder,
	} {
		if err := validateNotificationTemplateVariant(name, variant); err != nil {
			return err
		}
	}
	for index, localization := range document.Localizations {
		if localization == nil {
			return fmt.Errorf("document.localizations[%d] is required", index)
		}
		if localization.Firing == nil {
			return fmt.Errorf("document.localizations[%d].firing is required", index)
		}
		if localization.Resolved == nil {
			return fmt.Errorf("document.localizations[%d].resolved is required", index)
		}
		if localization.Reminder == nil {
			return fmt.Errorf("document.localizations[%d].reminder is required", index)
		}
		for name, variant := range map[string]*alertsv1.NotificationTemplateVariantV1{
			fmt.Sprintf("document.localizations[%d].firing", index):   localization.Firing,
			fmt.Sprintf("document.localizations[%d].resolved", index): localization.Resolved,
			fmt.Sprintf("document.localizations[%d].reminder", index): localization.Reminder,
		} {
			if err := validateNotificationTemplateVariant(name, variant); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNotificationTemplateVariant(name string, variant *alertsv1.NotificationTemplateVariantV1) error {
	if count := len(variant.Blocks); count < 1 || count > 32 {
		return fmt.Errorf("%s must contain 1..=32 blocks", name)
	}
	return nil
}

func notificationTemplateContentChanged(plan, state notificationTemplateModel) bool {
	return plan.Name.ValueString() != state.Name.ValueString() || plan.Description.ValueString() != state.Description.ValueString() || plan.Document.ValueString() != state.Document.ValueString() || stringValue(plan.ProvenanceRef) != stringValue(state.ProvenanceRef)
}

func notificationTemplateProtoFromState(data notificationTemplateModel) *alertsv1.AlertNotificationTemplateV1 {
	document, _ := notificationTemplateDocument(data.Document)
	return &alertsv1.AlertNotificationTemplateV1{Id: data.ID.ValueString(), TemplateKey: data.TemplateKey.ValueString(), Name: data.Name.ValueString(), Description: data.Description.ValueString(), CurrentRevisionId: data.CurrentRevisionID.ValueString(), PublishedRevisionId: data.PublishedRevisionID.ValueString(), Revision: data.Revision.ValueInt64(), ArchivedAt: nil, CurrentRevision: &alertsv1.AlertNotificationTemplateRevisionV1{Id: data.CurrentRevisionID.ValueString(), NotificationTemplateId: data.ID.ValueString(), Document: document, ContentHash: data.ContentHash.ValueString()}}
}

func setNotificationTemplate(data *notificationTemplateModel, item *alertsv1.AlertNotificationTemplateV1) {
	data.ID = types.StringValue(item.Id)
	data.TemplateKey = types.StringValue(item.TemplateKey)
	data.Name = types.StringValue(item.Name)
	data.Description = types.StringValue(item.Description)
	if item.ProvenanceRef == "" {
		data.ProvenanceRef = types.StringNull()
	} else {
		data.ProvenanceRef = types.StringValue(item.ProvenanceRef)
	}
	data.CurrentRevisionID = types.StringValue(item.CurrentRevisionId)
	if item.PublishedRevisionId == "" {
		data.PublishedRevisionID = types.StringNull()
	} else {
		data.PublishedRevisionID = types.StringValue(item.PublishedRevisionId)
	}
	data.Publish = types.BoolValue(item.CurrentRevisionId != "" && item.CurrentRevisionId == item.PublishedRevisionId)
	data.Archived = types.BoolValue(item.ArchivedAt != nil)
	data.Revision = types.Int64Value(item.Revision)
	data.CreatedAt = timestampString(item.CreatedAt)
	data.UpdatedAt = timestampString(item.UpdatedAt)
	if item.CurrentRevision == nil {
		data.Document = types.StringValue("{}")
		data.ContentHash = types.StringValue("")
		return
	}
	data.Document = jsonFromProtoPreserving(data.Document, item.CurrentRevision.Document)
	data.ContentHash = types.StringValue(item.CurrentRevision.ContentHash)
	data.ChangeReason = types.StringValue(item.CurrentRevision.ChangeReason)
}
