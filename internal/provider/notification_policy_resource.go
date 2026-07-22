package provider

import (
	"context"
	"fmt"

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
	"google.golang.org/protobuf/encoding/protojson"
)

var _ resource.ResourceWithConfigure = &notificationPolicyResource{}
var _ resource.ResourceWithImportState = &notificationPolicyResource{}
var _ resource.ResourceWithValidateConfig = &notificationPolicyResource{}
var _ resource.ResourceWithModifyPlan = &notificationPolicyResource{}

type notificationPolicyResource struct{ client *client.Client }
type notificationPolicyModel struct {
	ID        types.String `tfsdk:"id"`
	PolicyKey types.String `tfsdk:"policy_key"`
	Name      types.String `tfsdk:"name"`
	Enabled   types.Bool   `tfsdk:"enabled"`
	Config    types.String `tfsdk:"config_json"`
	Revision  types.Int64  `tfsdk:"revision"`
}

func NewNotificationPolicyResource() resource.Resource { return &notificationPolicyResource{} }
func (r *notificationPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_notification_policy"
}
func (r *notificationPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "An O11y.one alert notification policy with bounded route trees, grouping, repeats, escalation, revision-pinned templates, and notification budgets.", Attributes: map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}, "policy_key": schema.StringAttribute{Required: true}, "name": schema.StringAttribute{Required: true}, "enabled": schema.BoolAttribute{Required: true}, "config_json": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{canonicalJSONPlanModifier{}}, Description: "Exact AlertNotificationPolicyConfigV1 protobuf JSON. Route targets, matcher values, behavior, template revision bindings, grouping, timing, escalation, inhibition, and page budgets are typed."}, "revision": schema.Int64Attribute{Computed: true, Description: "Monotonic server revision used to fence concurrent updates."}}}
}
func (r *notificationPolicyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var data notificationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var err error
	data.Config, err = canonicalJSONString(data.Config)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("config_json"), "Invalid JSON", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &data)...)
}
func (r *notificationPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *notificationPolicyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data notificationPolicyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	for name, value := range map[string]types.String{"policy_key": data.PolicyKey, "name": data.Name} {
		if !value.IsUnknown() && !value.IsNull() {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	if !data.Config.IsUnknown() && !data.Config.IsNull() {
		if err := validationutil.JSONDocument(data.Config.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("config_json"), "Invalid policy config", err.Error())
		}
		config := &alertsv1.AlertNotificationPolicyConfigV1{}
		if err := protoFromJSON(data.Config, config); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("config_json"), "Invalid typed policy config", err.Error())
		}
	}
}
func (r *notificationPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data notificationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	message, err := r.request(&data, "create", data.PolicyKey.ValueString())
	if err != nil {
		addRPCError(&resp.Diagnostics, "build policy request", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Notifications.CreateNotificationPolicy(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "create notification policy", err)
		return
	}
	setNotificationPolicy(&data, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *notificationPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data notificationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.GetNotificationPolicy(rpcCtx, &alertsv1.GetAlertResourceRequest{Id: data.ID.ValueString(), OrgId: r.client.OrgID()})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		addRPCError(&resp.Diagnostics, "get notification policy", err)
		return
	}
	setNotificationPolicy(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *notificationPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state notificationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = state.ID
	message, err := r.request(&data, "update", state.ID.ValueString())
	if err != nil {
		addRPCError(&resp.Diagnostics, "build policy request", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Notifications.UpdateNotificationPolicy(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "update notification policy", err)
		return
	}
	setNotificationPolicy(&data, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *notificationPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data notificationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Notifications.DeleteNotificationPolicy(rpcCtx, &alertsv1.DeleteOperatorResourceRequest{OrgId: r.client.OrgID(), Id: data.ID.ValueString(), IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "notification_policy", "delete", data.ID.ValueString())})
	if err != nil {
		addRPCError(&resp.Diagnostics, "delete notification policy", err)
		return
	}
	if !result.Ok {
		resp.Diagnostics.AddError("Policy deletion rejected", result.Message)
	}
}
func (r *notificationPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
func (r *notificationPolicyResource) request(data *notificationPolicyModel, operation, identity string) (*alertsv1.UpsertNotificationPolicyRequest, error) {
	config := &alertsv1.AlertNotificationPolicyConfigV1{}
	if err := protoFromJSON(data.Config, config); err != nil {
		return nil, err
	}
	message := &alertsv1.UpsertNotificationPolicyRequest{Id: data.ID.ValueString(), PolicyKey: data.PolicyKey.ValueString(), Name: data.Name.ValueString(), Enabled: data.Enabled.ValueBool(), Config: config}
	if operation == "update" {
		expectedRevision := data.Revision.ValueInt64()
		message.ExpectedRevision = &expectedRevision
	}
	payload, err := protojson.Marshal(message)
	if err != nil {
		return nil, err
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "notification_policy", operation, identity, string(payload))
	return message, nil
}
func setNotificationPolicy(data *notificationPolicyModel, item *alertsv1.AlertNotificationPolicyV1) {
	data.ID = types.StringValue(item.Id)
	data.PolicyKey = types.StringValue(item.PolicyKey)
	data.Name = types.StringValue(item.Name)
	data.Enabled = types.BoolValue(item.Enabled)
	data.Config = jsonFromProtoPreserving(data.Config, item.Config)
	data.Revision = types.Int64Value(item.Revision)
}
