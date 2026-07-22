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

type silenceResource struct{ client *client.Client }
type silenceModel struct {
	ID         types.String `tfsdk:"id"`
	SilenceKey types.String `tfsdk:"silence_key"`
	Matcher    types.String `tfsdk:"matcher_json"`
	Reason     types.String `tfsdk:"reason"`
	StartsAt   types.String `tfsdk:"starts_at"`
	EndsAt     types.String `tfsdk:"ends_at"`
}

var _ resource.ResourceWithConfigure = &silenceResource{}
var _ resource.ResourceWithImportState = &silenceResource{}
var _ resource.ResourceWithValidateConfig = &silenceResource{}

func NewSilenceResource() resource.Resource { return &silenceResource{} }
func (r *silenceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_silence"
}
func (r *silenceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "An O11y.one alert silence.", Attributes: map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"silence_key": schema.StringAttribute{Required: true}, "matcher_json": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{canonicalJSONPlanModifier{}}},
		"reason": schema.StringAttribute{Required: true}, "starts_at": schema.StringAttribute{Required: true}, "ends_at": schema.StringAttribute{Required: true},
	}}
}
func (r *silenceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *silenceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data silenceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if !data.SilenceKey.IsNull() && !data.SilenceKey.IsUnknown() {
		if err := validationutil.NonEmpty(data.SilenceKey.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("silence_key"), "Value must not be empty", err.Error())
		}
	}
	if !data.Matcher.IsNull() && !data.Matcher.IsUnknown() {
		if err := validationutil.JSONDocument(data.Matcher.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("matcher_json"), "Invalid matcher", err.Error())
		}
		matcher := &alertsv1.AlertMatcherV1{}
		if err := protoFromJSON(data.Matcher, matcher); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("matcher_json"), "Invalid typed matcher", err.Error())
		}
	}
	validateTimeRange(&resp.Diagnostics, data.StartsAt, data.EndsAt)
}
func (r *silenceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data silenceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	message, err := r.request(&data, "create", data.SilenceKey.ValueString())
	if err != nil {
		addRPCError(&resp.Diagnostics, "build silence", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.CreateSilence(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "create silence", err)
		return
	}
	setSilence(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *silenceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data silenceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.GetSilence(rpcCtx, &alertsv1.GetAlertResourceRequest{Id: data.ID.ValueString(), OrgId: r.client.OrgID()})
	if status.Code(err) == codes.NotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addRPCError(&resp.Diagnostics, "get silence", err)
		return
	}
	setSilence(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *silenceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state silenceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = state.ID
	message, err := r.request(&data, "update", state.ID.ValueString())
	if err != nil {
		addRPCError(&resp.Diagnostics, "build silence", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.UpdateSilence(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "update silence", err)
		return
	}
	setSilence(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *silenceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data silenceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Notifications.DeleteSilence(rpcCtx, &alertsv1.DeleteOperatorResourceRequest{OrgId: r.client.OrgID(), Id: data.ID.ValueString(), IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "silence", "delete", data.ID.ValueString())})
	if status.Code(err) == codes.NotFound {
		return
	}
	if err != nil {
		addRPCError(&resp.Diagnostics, "delete silence", err)
		return
	}
	if !result.Ok {
		resp.Diagnostics.AddError("Silence deletion rejected", result.Message)
	}
}
func (r *silenceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
func (r *silenceResource) request(data *silenceModel, operation, identity string) (*alertsv1.UpsertSilenceRequest, error) {
	matcher := &alertsv1.AlertMatcherV1{}
	if err := protoFromJSON(data.Matcher, matcher); err != nil {
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
	message := &alertsv1.UpsertSilenceRequest{Id: data.ID.ValueString(), SilenceKey: data.SilenceKey.ValueString(), Matcher: matcher, Reason: data.Reason.ValueString(), StartsAt: timestamppb.New(startsAt), EndsAt: timestamppb.New(endsAt), OrgId: r.client.OrgID()}
	payload, err := protojson.Marshal(message)
	if err != nil {
		return nil, err
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "silence", operation, identity, string(payload))
	return message, nil
}
func setSilence(data *silenceModel, item *alertsv1.AlertSilenceV1) {
	data.ID = types.StringValue(item.Id)
	data.SilenceKey = types.StringValue(item.SilenceKey)
	data.Matcher = jsonFromProtoPreserving(data.Matcher, item.Matcher)
	data.Reason = types.StringValue(item.Reason)
	data.StartsAt = timestampString(item.StartsAt)
	data.EndsAt = timestampString(item.EndsAt)
}
