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
	"google.golang.org/protobuf/encoding/protojson"
)

var _ resource.ResourceWithConfigure = &destinationResource{}
var _ resource.ResourceWithImportState = &destinationResource{}
var _ resource.ResourceWithValidateConfig = &destinationResource{}
var _ resource.ResourceWithModifyPlan = &destinationResource{}

type destinationResource struct{ client *client.Client }
type destinationModel struct {
	ID             types.String `tfsdk:"id"`
	DestinationKey types.String `tfsdk:"destination_key"`
	Name           types.String `tfsdk:"name"`
	Kind           types.String `tfsdk:"kind"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Config         types.String `tfsdk:"config_json"`
	SecretRefs     types.String `tfsdk:"secret_refs_json"`
	LastTestedAt   types.String `tfsdk:"last_tested_at"`
}

func NewDestinationResource() resource.Resource { return &destinationResource{} }
func (r *destinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_destination"
}
func (r *destinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	canonical := []planmodifier.String{canonicalJSONPlanModifier{}}
	resp.Schema = schema.Schema{Description: "An O11y.one alert notification destination. Secret values are never accepted; only opaque secret references are managed.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}, "destination_key": schema.StringAttribute{Required: true}, "name": schema.StringAttribute{Required: true},
		"kind": schema.StringAttribute{Required: true, Description: "One of email, webhook, slack, pagerduty."}, "enabled": schema.BoolAttribute{Required: true},
		"config_json": schema.StringAttribute{Required: true, PlanModifiers: canonical}, "secret_refs_json": schema.StringAttribute{Required: true, Sensitive: true, PlanModifiers: canonical},
		"last_tested_at": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
	}}
}
func (r *destinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *destinationResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data destinationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !data.Kind.IsUnknown() && !data.Kind.IsNull() {
		if _, ok := destinationKind(data.Kind.ValueString()); !ok {
			resp.Diagnostics.AddAttributeError(path.Root("kind"), "Invalid destination kind", "kind must be one of email, webhook, slack, pagerduty")
		}
	}
	for name, value := range map[string]types.String{"destination_key": data.DestinationKey, "name": data.Name} {
		if !value.IsUnknown() && !value.IsNull() {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	if !data.Config.IsUnknown() && !data.Config.IsNull() {
		if err := validationutil.JSONDocument(data.Config.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("config_json"), "Invalid destination config", err.Error())
		}
		if err := validationutil.RejectSecretLikeConfig(data.Config.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("config_json"), "Inline secret material is forbidden", err.Error())
		}
	}
	if !data.SecretRefs.IsUnknown() && !data.SecretRefs.IsNull() {
		if err := validationutil.OpaqueSecretRefsJSON(data.SecretRefs.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("secret_refs_json"), "Invalid secret references", err.Error())
		}
	}
}
func (r *destinationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var data destinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var err error
	if data.Config, err = canonicalJSONString(data.Config); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("config_json"), "Invalid JSON", err.Error())
	}
	if data.SecretRefs, err = canonicalJSONString(data.SecretRefs); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("secret_refs_json"), "Invalid JSON", err.Error())
	}
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &data)...)
	}
}
func (r *destinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data destinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	message, err := r.request(&data, "create", data.DestinationKey.ValueString())
	if err != nil {
		addRPCError(&resp.Diagnostics, "build destination request", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Notifications.CreateDestination(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "create destination", err)
		return
	}
	setDestination(&data, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *destinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data destinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.GetDestination(rpcCtx, &alertsv1.GetAlertResourceRequest{Id: data.ID.ValueString(), OrgId: r.client.OrgID()})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		addRPCError(&resp.Diagnostics, "get destination", err)
		return
	}
	setDestination(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *destinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state destinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = state.ID
	message, err := r.request(&data, "update", state.ID.ValueString())
	if err != nil {
		addRPCError(&resp.Diagnostics, "build destination request", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Notifications.UpdateDestination(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "update destination", err)
		return
	}
	setDestination(&data, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *destinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data destinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Notifications.DeleteDestination(rpcCtx, &alertsv1.DeleteDestinationRequest{Id: data.ID.ValueString(), IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "destination", "delete", data.ID.ValueString())})
	if err != nil {
		addRPCError(&resp.Diagnostics, "delete destination", err)
		return
	}
	if !result.Ok {
		resp.Diagnostics.AddError("Destination deletion rejected", result.Message)
	}
}
func (r *destinationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
func (r *destinationResource) request(data *destinationModel, operation, identity string) (*alertsv1.UpsertDestinationRequest, error) {
	config, err := structFromJSON(data.Config)
	if err != nil {
		return nil, err
	}
	refs, err := structFromJSON(data.SecretRefs)
	if err != nil {
		return nil, err
	}
	kind, _ := destinationKind(data.Kind.ValueString())
	message := &alertsv1.UpsertDestinationRequest{Id: data.ID.ValueString(), DestinationKey: data.DestinationKey.ValueString(), Name: data.Name.ValueString(), Kind: kind, Enabled: data.Enabled.ValueBool(), Config: config, SecretRefs: refs}
	payload, err := protojson.Marshal(message)
	if err != nil {
		return nil, err
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "destination", operation, identity, string(payload))
	return message, nil
}
func setDestination(data *destinationModel, item *alertsv1.AlertDestinationV1) {
	data.ID = types.StringValue(item.Id)
	data.DestinationKey = types.StringValue(item.DestinationKey)
	data.Name = types.StringValue(item.Name)
	data.Kind = types.StringValue(strings.ToLower(strings.TrimPrefix(item.Kind.String(), "ALERT_DESTINATION_KIND_V1_")))
	data.Enabled = types.BoolValue(item.Enabled)
	data.Config = jsonFromStruct(item.Config)
	data.SecretRefs = jsonFromStruct(item.SecretRefs)
	if item.LastTestedAt == nil {
		data.LastTestedAt = types.StringNull()
	} else {
		data.LastTestedAt = types.StringValue(item.LastTestedAt.AsTime().UTC().Format("2006-01-02T15:04:05.999999999Z"))
	}
}
func destinationKind(value string) (alertsv1.AlertDestinationKindV1, bool) {
	kinds := map[string]alertsv1.AlertDestinationKindV1{"email": alertsv1.AlertDestinationKindV1_ALERT_DESTINATION_KIND_V1_EMAIL, "webhook": alertsv1.AlertDestinationKindV1_ALERT_DESTINATION_KIND_V1_WEBHOOK, "slack": alertsv1.AlertDestinationKindV1_ALERT_DESTINATION_KIND_V1_SLACK, "pagerduty": alertsv1.AlertDestinationKindV1_ALERT_DESTINATION_KIND_V1_PAGERDUTY}
	kind, ok := kinds[value]
	return kind, ok
}
