package provider

import (
	"context"
	"fmt"
	"time"

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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type maintenanceWindowResource struct{ client *client.Client }
type maintenanceWindowModel struct {
	ID        types.String `tfsdk:"id"`
	WindowKey types.String `tfsdk:"window_key"`
	Name      types.String `tfsdk:"name"`
	Scope     types.String `tfsdk:"scope_json"`
	StartsAt  types.String `tfsdk:"starts_at"`
	EndsAt    types.String `tfsdk:"ends_at"`
}

var _ resource.ResourceWithConfigure = &maintenanceWindowResource{}
var _ resource.ResourceWithImportState = &maintenanceWindowResource{}
var _ resource.ResourceWithValidateConfig = &maintenanceWindowResource{}

func NewMaintenanceWindowResource() resource.Resource { return &maintenanceWindowResource{} }
func (r *maintenanceWindowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_maintenance_window"
}
func (r *maintenanceWindowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "An O11y.one alert maintenance window.", Attributes: map[string]schema.Attribute{
		"id":         schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"window_key": schema.StringAttribute{Required: true}, "name": schema.StringAttribute{Required: true},
		"scope_json": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{canonicalJSONPlanModifier{}}},
		"starts_at":  schema.StringAttribute{Required: true}, "ends_at": schema.StringAttribute{Required: true},
	}}
}
func (r *maintenanceWindowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *maintenanceWindowResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data maintenanceWindowModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	for name, value := range map[string]types.String{"window_key": data.WindowKey, "name": data.Name} {
		if !value.IsNull() && !value.IsUnknown() {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	validateTimeRange(&resp.Diagnostics, data.StartsAt, data.EndsAt)
	if !data.Scope.IsNull() && !data.Scope.IsUnknown() {
		if err := validationutil.JSONDocument(data.Scope.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("scope_json"), "Invalid scope", err.Error())
		}
	}
}
func (r *maintenanceWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	message, err := r.request(&data, "create", data.WindowKey.ValueString())
	if err != nil {
		addRPCError(&resp.Diagnostics, "build maintenance window", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.CreateMaintenanceWindow(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "create maintenance window", err)
		return
	}
	setMaintenanceWindow(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *maintenanceWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.GetMaintenanceWindow(rpcCtx, &alertsv1.GetAlertResourceRequest{Id: data.ID.ValueString(), OrgId: r.client.OrgID()})
	if status.Code(err) == codes.NotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addRPCError(&resp.Diagnostics, "get maintenance window", err)
		return
	}
	setMaintenanceWindow(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *maintenanceWindowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = state.ID
	message, err := r.request(&data, "update", state.ID.ValueString())
	if err != nil {
		addRPCError(&resp.Diagnostics, "build maintenance window", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.UpdateMaintenanceWindow(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "update maintenance window", err)
		return
	}
	setMaintenanceWindow(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *maintenanceWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Notifications.DeleteMaintenanceWindow(rpcCtx, &alertsv1.DeleteOperatorResourceRequest{OrgId: r.client.OrgID(), Id: data.ID.ValueString(), IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "maintenance_window", "delete", data.ID.ValueString())})
	if status.Code(err) == codes.NotFound {
		return
	}
	if err != nil {
		addRPCError(&resp.Diagnostics, "delete maintenance window", err)
		return
	}
	if !result.Ok {
		resp.Diagnostics.AddError("Maintenance window deletion rejected", result.Message)
	}
}
func (r *maintenanceWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
func (r *maintenanceWindowResource) request(data *maintenanceWindowModel, operation, identity string) (*alertsv1.UpsertMaintenanceWindowRequest, error) {
	scope, err := structFromJSON(data.Scope)
	if err != nil {
		return nil, err
	}
	startsAt, err := time.Parse(time.RFC3339, data.StartsAt.ValueString())
	if err != nil {
		return nil, err
	}
	endsAt, err := time.Parse(time.RFC3339, data.EndsAt.ValueString())
	if err != nil {
		return nil, err
	}
	message := &alertsv1.UpsertMaintenanceWindowRequest{Id: data.ID.ValueString(), WindowKey: data.WindowKey.ValueString(), Name: data.Name.ValueString(), Scope: scope, StartsAt: timestamppb.New(startsAt), EndsAt: timestamppb.New(endsAt)}
	payload, err := protojson.Marshal(message)
	if err != nil {
		return nil, err
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "maintenance_window", operation, identity, string(payload))
	return message, nil
}
func setMaintenanceWindow(data *maintenanceWindowModel, item *alertsv1.AlertMaintenanceWindowV1) {
	data.ID = types.StringValue(item.Id)
	data.WindowKey = types.StringValue(item.WindowKey)
	data.Name = types.StringValue(item.Name)
	data.Scope = jsonFromStruct(item.Scope)
	data.StartsAt = timestampString(item.StartsAt)
	data.EndsAt = timestampString(item.EndsAt)
}
