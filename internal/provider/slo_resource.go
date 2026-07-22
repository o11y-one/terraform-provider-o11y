package provider

import (
	"context"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
)

const defaultRollingSLOWindowSeconds int64 = 30 * 24 * 60 * 60
const maximumSLOWindowSeconds int64 = 100 * 24 * 60 * 60

var _ resource.ResourceWithConfigure = &sloResource{}
var _ resource.ResourceWithImportState = &sloResource{}
var _ resource.ResourceWithValidateConfig = &sloResource{}
var _ resource.ResourceWithModifyPlan = &sloResource{}

type sloResource struct{ client *client.Client }

type sloModel struct {
	ID                   types.String  `tfsdk:"id"`
	SLOKey               types.String  `tfsdk:"slo_key"`
	Name                 types.String  `tfsdk:"name"`
	Description          types.String  `tfsdk:"description"`
	SLIID                types.String  `tfsdk:"sli_id"`
	SLIRevisionID        types.String  `tfsdk:"sli_revision_id"`
	TargetRatio          types.Float64 `tfsdk:"target_ratio"`
	WindowMode           types.String  `tfsdk:"window_mode"`
	RollingWindowSeconds types.Int64   `tfsdk:"rolling_window_seconds"`
	CalendarPeriod       types.String  `tfsdk:"calendar_period"`
	CalendarTimezone     types.String  `tfsdk:"calendar_timezone"`
	OwnerUserID          types.String  `tfsdk:"owner_user_id"`
	OwnerTeamID          types.String  `tfsdk:"owner_team_id"`
	OwnerDisplayName     types.String  `tfsdk:"owner_display_name"`
	Labels               types.String  `tfsdk:"labels_json"`
	CurrentRevisionID    types.String  `tfsdk:"current_revision_id"`
	RevisionNumber       types.Int64   `tfsdk:"revision_number"`
	ConfigHash           types.String  `tfsdk:"config_hash"`
	EffectiveFrom        types.String  `tfsdk:"effective_from"`
	MaximumWindowSeconds types.Int64   `tfsdk:"maximum_window_seconds"`
	Archived             types.Bool    `tfsdk:"archived"`
	CreatedAt            types.String  `tfsdk:"created_at"`
	UpdatedAt            types.String  `tfsdk:"updated_at"`
}

func NewSLOResource() resource.Resource { return &sloResource{} }

func (r *sloResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_slo"
}

func (r *sloResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A revisioned O11y.one service-level objective with an exact rolling or timezone-aware calendar error-budget window.",
		Attributes: map[string]schema.Attribute{
			"id":                     schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"slo_key":                schema.StringAttribute{Required: true, Description: "Stable objective key.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":                   schema.StringAttribute{Required: true},
			"description":            schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"sli_id":                 schema.StringAttribute{Required: true, Description: "Durable SLI resource UUID."},
			"sli_revision_id":        schema.StringAttribute{Required: true, Description: "Immutable SLI revision UUID evaluated by this objective."},
			"target_ratio":           schema.Float64Attribute{Required: true, Description: "Target as a ratio strictly between 0 and 1."},
			"window_mode":            schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("rolling"), Description: "rolling or calendar."},
			"rolling_window_seconds": schema.Int64Attribute{Optional: true, Computed: true, Description: "Whole-minute rolling window. Defaults to 30 days in rolling mode and is zero in calendar mode."},
			"calendar_period":        schema.StringAttribute{Optional: true, Description: "day, week, month, or quarter. Required only in calendar mode."},
			"calendar_timezone":      schema.StringAttribute{Optional: true, Description: "IANA timezone used for calendar reset boundaries. Required only in calendar mode."},
			"owner_user_id":          schema.StringAttribute{Optional: true, Description: "Optional owner user UUID. Mutually exclusive with owner_team_id."},
			"owner_team_id":          schema.StringAttribute{Optional: true, Description: "Optional owner team UUID. Mutually exclusive with owner_user_id."},
			"owner_display_name":     schema.StringAttribute{Optional: true},
			"labels_json":            schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("{}"), PlanModifiers: []planmodifier.String{canonicalJSONPlanModifier{}}},
			"current_revision_id":    schema.StringAttribute{Computed: true},
			"revision_number":        schema.Int64Attribute{Computed: true},
			"config_hash":            schema.StringAttribute{Computed: true},
			"effective_from":         schema.StringAttribute{Computed: true, Description: "Minute-aligned boundary from which the current revision owns the error budget."},
			"maximum_window_seconds": schema.Int64Attribute{Computed: true, Description: "Maximum fact-retention horizon required by this window contract."},
			"archived":               schema.BoolAttribute{Computed: true},
			"created_at":             schema.StringAttribute{Computed: true},
			"updated_at":             schema.StringAttribute{Computed: true},
		},
	}
}

func (r *sloResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *sloResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data sloModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, value := range map[string]types.String{"slo_key": data.SLOKey, "name": data.Name} {
		if knownNonEmpty(value) {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	for name, value := range map[string]types.String{
		"sli_id": data.SLIID, "sli_revision_id": data.SLIRevisionID,
		"owner_user_id": data.OwnerUserID, "owner_team_id": data.OwnerTeamID,
	} {
		if knownNonEmpty(value) {
			if err := validationutil.UUID(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid UUID", err.Error())
			}
		}
	}
	if knownNonEmpty(data.OwnerUserID) && knownNonEmpty(data.OwnerTeamID) {
		resp.Diagnostics.AddError("Invalid SLO owner", "owner_user_id and owner_team_id are mutually exclusive")
	}
	if !data.TargetRatio.IsNull() && !data.TargetRatio.IsUnknown() {
		value := data.TargetRatio.ValueFloat64()
		if value <= 0 || value >= 1 {
			resp.Diagnostics.AddAttributeError(path.Root("target_ratio"), "Invalid SLO target", "target_ratio must be strictly between 0 and 1")
		}
	}
	if data.WindowMode.IsUnknown() {
		return
	}
	mode := stringValue(data.WindowMode)
	if mode == "" {
		mode = "rolling"
	}
	switch mode {
	case "rolling":
		if knownNonEmpty(data.CalendarPeriod) || knownNonEmpty(data.CalendarTimezone) {
			resp.Diagnostics.AddError("Invalid rolling window", "calendar_period and calendar_timezone must be omitted in rolling mode")
		}
		if !data.RollingWindowSeconds.IsNull() && !data.RollingWindowSeconds.IsUnknown() {
			validateRollingSLOWindow(&resp.Diagnostics, data.RollingWindowSeconds.ValueInt64())
		}
	case "calendar":
		period := stringValue(data.CalendarPeriod)
		if period != "day" && period != "week" && period != "month" && period != "quarter" {
			resp.Diagnostics.AddAttributeError(path.Root("calendar_period"), "Invalid calendar period", "calendar_period must be day, week, month, or quarter")
		}
		timezone := stringValue(data.CalendarTimezone)
		if timezone == "" {
			resp.Diagnostics.AddAttributeError(path.Root("calendar_timezone"), "Missing calendar timezone", "calendar_timezone is required in calendar mode")
		} else if _, err := time.LoadLocation(timezone); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("calendar_timezone"), "Invalid IANA timezone", err.Error())
		}
		if !data.RollingWindowSeconds.IsNull() && !data.RollingWindowSeconds.IsUnknown() && data.RollingWindowSeconds.ValueInt64() != 0 {
			resp.Diagnostics.AddAttributeError(path.Root("rolling_window_seconds"), "Invalid calendar window", "rolling_window_seconds must be omitted or zero in calendar mode")
		}
	default:
		resp.Diagnostics.AddAttributeError(path.Root("window_mode"), "Invalid window mode", "window_mode must be rolling or calendar")
	}
}

func validateRollingSLOWindow(diags interface {
	AddAttributeError(path.Path, string, string)
}, seconds int64) {
	if seconds < 300 || seconds > maximumSLOWindowSeconds || seconds%60 != 0 {
		diags.AddAttributeError(path.Root("rolling_window_seconds"), "Invalid rolling SLO window", "rolling_window_seconds must be a whole-minute window between five minutes and 100 days")
	}
}

func (r *sloResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan, config sloModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.WindowMode.IsUnknown() {
		return
	}
	if stringValue(config.WindowMode) == "calendar" {
		plan.RollingWindowSeconds = types.Int64Value(0)
	} else if config.RollingWindowSeconds.IsNull() {
		plan.RollingWindowSeconds = types.Int64Value(defaultRollingSLOWindowSeconds)
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (r *sloResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data sloModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	request, err := r.createRequest(&data)
	if err != nil {
		resp.Diagnostics.AddError("Invalid SLO configuration", err.Error())
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.SLOs.CreateSlo(rpcCtx, request)
	if err != nil {
		addRPCError(&resp.Diagnostics, "create SLO", err)
		return
	}
	setSLO(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *sloResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data sloModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.SLOs.GetSlo(rpcCtx, &alertsv1.GetSloRequest{SloId: data.ID.ValueString()})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		addRPCError(&resp.Diagnostics, "get SLO", err)
		return
	}
	setSLO(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *sloResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state sloModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	revision, err := sloRevisionInput(&plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid SLO configuration", err.Error())
		return
	}
	message := &alertsv1.UpdateSloRequest{
		SloId: state.ID.ValueString(), ExpectedRevisionId: state.CurrentRevisionID.ValueString(),
		Name: plan.Name.ValueString(), Description: stringValue(plan.Description), Revision: revision,
	}
	payload, err := protojson.Marshal(message)
	if err != nil {
		resp.Diagnostics.AddError("Unable to encode SLO update", err.Error())
		return
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "slo", "update", state.ID.ValueString(), string(payload))
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.SLOs.UpdateSlo(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "update SLO", err)
		return
	}
	setSLO(&plan, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sloResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data sloModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	_, err := r.client.SLOs.ArchiveSlo(rpcCtx, &alertsv1.ArchiveSloRequest{
		SloId:          data.ID.ValueString(),
		IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "slo", "archive", data.ID.ValueString(), data.CurrentRevisionID.ValueString()),
	})
	if err != nil && status.Code(err) != codes.NotFound {
		addRPCError(&resp.Diagnostics, "archive SLO", err)
	}
}

func (r *sloResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *sloResource) createRequest(data *sloModel) (*alertsv1.CreateSloRequest, error) {
	revision, err := sloRevisionInput(data)
	if err != nil {
		return nil, err
	}
	message := &alertsv1.CreateSloRequest{
		SloKey: data.SLOKey.ValueString(), Name: data.Name.ValueString(), Description: stringValue(data.Description), Revision: revision,
	}
	payload, err := protojson.Marshal(message)
	if err != nil {
		return nil, err
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "slo", "create", data.SLOKey.ValueString(), string(payload))
	return message, nil
}

func sloRevisionInput(data *sloModel) (*alertsv1.SloRevisionInputV1, error) {
	labels, err := stringMapFromJSON(data.Labels)
	if err != nil {
		return nil, fmt.Errorf("labels_json: %w", err)
	}
	mode := alertsv1.SloWindowModeV1_SLO_WINDOW_MODE_V1_ROLLING
	period := alertsv1.SloCalendarPeriodV1_SLO_CALENDAR_PERIOD_V1_UNSPECIFIED
	rollingSeconds := defaultRollingSLOWindowSeconds
	timezone := ""
	if stringValue(data.WindowMode) == "calendar" {
		mode = alertsv1.SloWindowModeV1_SLO_WINDOW_MODE_V1_CALENDAR
		rollingSeconds = 0
		period = calendarPeriodProto(stringValue(data.CalendarPeriod))
		timezone = stringValue(data.CalendarTimezone)
	} else if !data.RollingWindowSeconds.IsNull() && !data.RollingWindowSeconds.IsUnknown() {
		rollingSeconds = data.RollingWindowSeconds.ValueInt64()
	}
	return &alertsv1.SloRevisionInputV1{
		SliId: data.SLIID.ValueString(), SliRevisionId: data.SLIRevisionID.ValueString(), TargetRatio: data.TargetRatio.ValueFloat64(),
		RollingWindowSeconds: rollingSeconds, WindowMode: mode, CalendarPeriod: period, CalendarTimezone: timezone,
		Owner: sloOwnerProto(data.OwnerUserID, data.OwnerTeamID, data.OwnerDisplayName), Labels: &alertsv1.AlertLabelsV1{Values: labels},
	}, nil
}

func sloOwnerProto(user, team, displayName types.String) *alertsv1.AlertOwnerRefV1 {
	owner := &alertsv1.AlertOwnerRefV1{DisplayName: stringValue(displayName), Active: true}
	if knownNonEmpty(user) {
		owner.Owner = &alertsv1.AlertOwnerRefV1_UserId{UserId: user.ValueString()}
		return owner
	}
	if knownNonEmpty(team) {
		owner.Owner = &alertsv1.AlertOwnerRefV1_TeamId{TeamId: team.ValueString()}
		return owner
	}
	return nil
}

func calendarPeriodProto(value string) alertsv1.SloCalendarPeriodV1 {
	return map[string]alertsv1.SloCalendarPeriodV1{
		"day":     alertsv1.SloCalendarPeriodV1_SLO_CALENDAR_PERIOD_V1_DAY,
		"week":    alertsv1.SloCalendarPeriodV1_SLO_CALENDAR_PERIOD_V1_WEEK,
		"month":   alertsv1.SloCalendarPeriodV1_SLO_CALENDAR_PERIOD_V1_MONTH,
		"quarter": alertsv1.SloCalendarPeriodV1_SLO_CALENDAR_PERIOD_V1_QUARTER,
	}[value]
}

func setSLO(data *sloModel, item *alertsv1.AlertSloV1) {
	data.ID = types.StringValue(item.Id)
	data.SLOKey = types.StringValue(item.SloKey)
	data.Name = types.StringValue(item.Name)
	data.Description = types.StringValue(item.Description)
	data.CurrentRevisionID = types.StringValue(item.CurrentRevisionId)
	data.Archived = types.BoolValue(item.ArchivedAt != nil)
	data.CreatedAt = timestampString(item.CreatedAt)
	data.UpdatedAt = timestampString(item.UpdatedAt)
	revision := item.CurrentRevision
	if revision == nil {
		return
	}
	data.SLIID = types.StringValue(revision.SliId)
	data.SLIRevisionID = types.StringValue(revision.SliRevisionId)
	data.TargetRatio = types.Float64Value(revision.TargetRatio)
	data.RollingWindowSeconds = types.Int64Value(revision.RollingWindowSeconds)
	data.WindowMode = types.StringValue(strings.ToLower(strings.TrimPrefix(revision.WindowMode.String(), "SLO_WINDOW_MODE_V1_")))
	if revision.CalendarPeriod == alertsv1.SloCalendarPeriodV1_SLO_CALENDAR_PERIOD_V1_UNSPECIFIED {
		data.CalendarPeriod = types.StringNull()
		data.CalendarTimezone = types.StringNull()
	} else {
		data.CalendarPeriod = types.StringValue(strings.ToLower(strings.TrimPrefix(revision.CalendarPeriod.String(), "SLO_CALENDAR_PERIOD_V1_")))
		data.CalendarTimezone = types.StringValue(revision.CalendarTimezone)
	}
	setOwner(&data.OwnerUserID, &data.OwnerTeamID, revision.Owner)
	if revision.Owner == nil || revision.Owner.DisplayName == "" {
		data.OwnerDisplayName = types.StringNull()
	} else {
		data.OwnerDisplayName = types.StringValue(revision.Owner.DisplayName)
	}
	if revision.Labels == nil {
		data.Labels = jsonFromStringMap(nil)
	} else {
		data.Labels = jsonFromStringMap(revision.Labels.Values)
	}
	data.RevisionNumber = types.Int64Value(int64(revision.RevisionNumber))
	data.ConfigHash = types.StringValue(revision.ConfigHash)
	data.EffectiveFrom = timestampString(revision.EffectiveFrom)
	data.MaximumWindowSeconds = types.Int64Value(revision.MaximumWindowSeconds)
}
