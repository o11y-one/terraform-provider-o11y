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

var _ resource.ResourceWithConfigure = &contactResource{}
var _ resource.ResourceWithImportState = &contactResource{}
var _ resource.ResourceWithValidateConfig = &contactResource{}

type contactResource struct{ client *client.Client }
type contactModel struct {
	ID                  types.String `tfsdk:"id"`
	ContactKey          types.String `tfsdk:"contact_key"`
	DisplayName         types.String `tfsdk:"display_name"`
	Email               types.String `tfsdk:"email"`
	RequestVerification types.Bool   `tfsdk:"request_verification"`
	Status              types.String `tfsdk:"status"`
	Generation          types.Int64  `tfsdk:"generation"`
	Revision            types.Int64  `tfsdk:"revision"`
}

func NewContactResource() resource.Resource { return &contactResource{} }
func (r *contactResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_contact"
}
func (r *contactResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "A verified human email contact for reusable alert notification groups. Verification tokens are never stored in Terraform state.", Attributes: map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"contact_key": schema.StringAttribute{Required: true}, "display_name": schema.StringAttribute{Required: true}, "email": schema.StringAttribute{Required: true},
		"request_verification": schema.BoolAttribute{Optional: true, Description: "Send a verification email for each new contact generation. Confirmation remains an out-of-band user action."},
		"status":               schema.StringAttribute{Computed: true}, "generation": schema.Int64Attribute{Computed: true}, "revision": schema.Int64Attribute{Computed: true},
	}}
}
func (r *contactResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *contactResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data contactModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	for name, value := range map[string]types.String{"contact_key": data.ContactKey, "display_name": data.DisplayName, "email": data.Email} {
		if !value.IsUnknown() && !value.IsNull() {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	if !data.Email.IsUnknown() && !data.Email.IsNull() && (!strings.Contains(data.Email.ValueString(), "@") || len(data.Email.ValueString()) > 320) {
		resp.Diagnostics.AddAttributeError(path.Root("email"), "Invalid contact email", "email must be a valid address of at most 320 characters")
	}
}
func (r *contactResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data contactModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	message := r.request(&data, "create", data.ContactKey.ValueString(), nil)
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.CreateContact(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "create contact", err)
		return
	}
	setContact(&data, item)
	r.maybeVerify(ctx, &data, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	}
}
func (r *contactResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data contactModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.GetContact(rpcCtx, &alertsv1.GetAlertResourceRequest{Id: data.ID.ValueString(), OrgId: r.client.OrgID()})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		addRPCError(&resp.Diagnostics, "get contact", err)
		return
	}
	requestVerification := data.RequestVerification
	setContact(&data, item)
	data.RequestVerification = requestVerification
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *contactResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state contactModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = state.ID
	message := r.request(&data, "update", state.ID.ValueString(), proto.Int64(state.Revision.ValueInt64()))
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.UpdateContact(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "update contact", err)
		return
	}
	setContact(&data, item)
	r.maybeVerify(ctx, &data, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	}
}
func (r *contactResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data contactModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	_, err := r.client.Notifications.SetContactArchived(rpcCtx, &alertsv1.SetAlertContactArchivedRequest{OrgId: r.client.OrgID(), ContactId: data.ID.ValueString(), ExpectedRevision: data.Revision.ValueInt64(), Archived: true, Reason: "removed from Terraform configuration", IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "contact", "archive", data.ID.ValueString())})
	if err != nil {
		addRPCError(&resp.Diagnostics, "archive contact", err)
	}
}
func (r *contactResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
func (r *contactResource) request(data *contactModel, operation, identity string, expected *int64) *alertsv1.UpsertAlertContactRequest {
	message := &alertsv1.UpsertAlertContactRequest{Id: data.ID.ValueString(), ContactKey: data.ContactKey.ValueString(), DisplayName: data.DisplayName.ValueString(), Email: data.Email.ValueString(), ExpectedRevision: expected, OrgId: r.client.OrgID()}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "contact", operation, identity, message.ContactKey, message.DisplayName, message.Email)
	return message
}
func (r *contactResource) maybeVerify(ctx context.Context, data *contactModel, diags interface{ AddError(string, string) }) {
	if data.RequestVerification.IsNull() || data.RequestVerification.IsUnknown() || !data.RequestVerification.ValueBool() || data.Status.ValueString() == "verified" {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	_, err := r.client.Notifications.BeginContactVerification(rpcCtx, &alertsv1.BeginAlertContactVerificationRequest{OrgId: r.client.OrgID(), ContactId: data.ID.ValueString(), ExpectedGeneration: data.Generation.ValueInt64(), IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "contact", "verify", data.ID.ValueString(), fmt.Sprint(data.Generation.ValueInt64()))})
	if err != nil {
		diags.AddError("Unable to request contact verification", err.Error())
	}
}
func setContact(data *contactModel, item *alertsv1.AlertContactV1) {
	data.ID = types.StringValue(item.Id)
	data.ContactKey = types.StringValue(item.ContactKey)
	data.DisplayName = types.StringValue(item.DisplayName)
	data.Email = types.StringValue(item.Email)
	data.Status = types.StringValue(strings.ToLower(strings.TrimPrefix(item.Status.String(), "ALERT_CONTACT_STATUS_V1_")))
	data.Generation = types.Int64Value(item.Generation)
	data.Revision = types.Int64Value(item.Revision)
}
