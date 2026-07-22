package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	"google.golang.org/protobuf/types/known/durationpb"
)

var _ resource.ResourceWithConfigure = &sliResource{}
var _ resource.ResourceWithImportState = &sliResource{}
var _ resource.ResourceWithValidateConfig = &sliResource{}

type sliResource struct{ client *client.Client }

type sliModel struct {
	ID                types.String `tfsdk:"id"`
	SLIKey            types.String `tfsdk:"sli_key"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	IndicatorKind     types.String `tfsdk:"indicator_kind"`
	Scope             types.String `tfsdk:"scope_json"`
	OwnerUserID       types.String `tfsdk:"owner_user_id"`
	OwnerTeamID       types.String `tfsdk:"owner_team_id"`
	OwnerDisplayName  types.String `tfsdk:"owner_display_name"`
	EligibleEvents    types.String `tfsdk:"eligible_events"`
	GoodEvents        types.String `tfsdk:"good_events"`
	ExcludedEvents    types.String `tfsdk:"excluded_events"`
	Aggregation       types.String `tfsdk:"aggregation"`
	LatencyThreshold  types.String `tfsdk:"latency_threshold"`
	MissingData       types.String `tfsdk:"missing_data_behavior"`
	CurrentRevisionID types.String `tfsdk:"current_revision_id"`
	RevisionNumber    types.Int64  `tfsdk:"revision_number"`
	ConfigHash        types.String `tfsdk:"config_hash"`
	Archived          types.Bool   `tfsdk:"archived"`
	CreatedAt         types.String `tfsdk:"created_at"`
	UpdatedAt         types.String `tfsdk:"updated_at"`
}

func NewSLIResource() resource.Resource { return &sliResource{} }

func (r *sliResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sli"
}

func (r *sliResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "A revisioned O11y.one service-level indicator evaluated from canonical minute-level telemetry facts.", Attributes: map[string]schema.Attribute{
		"id":                    schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"sli_key":               schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, Description: "Stable immutable indicator key."},
		"name":                  schema.StringAttribute{Required: true},
		"description":           schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
		"indicator_kind":        schema.StringAttribute{Required: true, Description: "availability, latency, agent_outcome, eval_quality, tool_success, or cost_per_successful_outcome."},
		"scope_json":            schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{canonicalJSONPlanModifier{}}, Description: "Typed AlertScopeV1 protobuf JSON, including telemetry attribute filters and span kinds."},
		"owner_user_id":         schema.StringAttribute{Optional: true, Description: "Optional owner user UUID. Mutually exclusive with owner_team_id."},
		"owner_team_id":         schema.StringAttribute{Optional: true, Description: "Optional owner team UUID. Mutually exclusive with owner_user_id."},
		"owner_display_name":    schema.StringAttribute{Optional: true},
		"eligible_events":       schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("all matching events"), Description: "Human-readable event-set definition recorded with the SLI revision."},
		"good_events":           schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("events satisfying the indicator"), Description: "Human-readable good-event definition recorded with the SLI revision."},
		"excluded_events":       schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Human-readable excluded-event definition."},
		"aggregation":           schema.StringAttribute{Required: true, Description: "event_ratio or threshold_ratio."},
		"latency_threshold":     schema.StringAttribute{Optional: true, Description: "Go duration between 100us and 24h for latency indicators, for example 250ms or 2s."},
		"missing_data_behavior": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("unknown"), Description: "unknown, good, or bad."},
		"current_revision_id":   schema.StringAttribute{Computed: true},
		"revision_number":       schema.Int64Attribute{Computed: true},
		"config_hash":           schema.StringAttribute{Computed: true},
		"archived":              schema.BoolAttribute{Computed: true},
		"created_at":            schema.StringAttribute{Computed: true},
		"updated_at":            schema.StringAttribute{Computed: true},
	}}
}

func (r *sliResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *sliResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data sliModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, value := range map[string]types.String{"sli_key": data.SLIKey, "name": data.Name} {
		if knownNonEmpty(value) {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	if knownNonEmpty(data.OwnerUserID) && knownNonEmpty(data.OwnerTeamID) {
		resp.Diagnostics.AddError("Invalid SLI owner", "owner_user_id and owner_team_id are mutually exclusive")
	}
	for name, value := range map[string]types.String{"owner_user_id": data.OwnerUserID, "owner_team_id": data.OwnerTeamID} {
		if knownNonEmpty(value) {
			if err := validationutil.UUID(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid owner UUID", err.Error())
			}
		}
	}
	if knownNonEmpty(data.IndicatorKind) {
		if _, ok := sliIndicatorKind(data.IndicatorKind.ValueString()); !ok {
			resp.Diagnostics.AddAttributeError(path.Root("indicator_kind"), "Invalid indicator kind", "use availability, latency, agent_outcome, eval_quality, tool_success, or cost_per_successful_outcome")
		}
	}
	if knownNonEmpty(data.Aggregation) {
		if _, ok := sliAggregation(data.Aggregation.ValueString()); !ok {
			resp.Diagnostics.AddAttributeError(path.Root("aggregation"), "Invalid aggregation", "use event_ratio or threshold_ratio")
		}
	}
	if knownNonEmpty(data.MissingData) {
		if _, ok := sliMissingData(data.MissingData.ValueString()); !ok {
			resp.Diagnostics.AddAttributeError(path.Root("missing_data_behavior"), "Invalid missing-data behavior", "use unknown, good, or bad")
		}
	}
	if knownNonEmpty(data.Scope) {
		scope := &alertsv1.AlertScopeV1{}
		if err := protoFromJSON(data.Scope, scope); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("scope_json"), "Invalid typed SLI scope", err.Error())
		}
	}
	if knownNonEmpty(data.LatencyThreshold) {
		value, err := time.ParseDuration(data.LatencyThreshold.ValueString())
		if err != nil || value < 100*time.Microsecond || value > 24*time.Hour {
			resp.Diagnostics.AddAttributeError(path.Root("latency_threshold"), "Invalid latency threshold", "latency_threshold must be a duration from 100us through 24h")
		}
	}
	if knownNonEmpty(data.IndicatorKind) && data.IndicatorKind.ValueString() == "latency" && !knownNonEmpty(data.LatencyThreshold) {
		resp.Diagnostics.AddAttributeError(path.Root("latency_threshold"), "Latency threshold required", "latency indicators require latency_threshold")
	}
}

func (r *sliResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data sliModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	revision, err := sliRevisionInput(&data)
	if err != nil {
		addRPCError(&resp.Diagnostics, "build SLI", err)
		return
	}
	message := &alertsv1.CreateSliRequest{SliKey: data.SLIKey.ValueString(), Name: data.Name.ValueString(), Description: stringValue(data.Description), Revision: revision}
	payload, _ := protojson.Marshal(message)
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "sli", "create", data.SLIKey.ValueString(), string(payload))
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.SLOs.CreateSli(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "create SLI", err)
		return
	}
	setSLI(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *sliResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data sliModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.SLOs.GetSli(rpcCtx, &alertsv1.GetSliRequest{SliId: data.ID.ValueString()})
	if status.Code(err) == codes.NotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addRPCError(&resp.Diagnostics, "get SLI", err)
		return
	}
	setSLI(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *sliResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state sliModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	revision, err := sliRevisionInput(&data)
	if err != nil {
		addRPCError(&resp.Diagnostics, "build SLI", err)
		return
	}
	message := &alertsv1.UpdateSliRequest{SliId: state.ID.ValueString(), ExpectedRevisionId: state.CurrentRevisionID.ValueString(), Name: data.Name.ValueString(), Description: stringValue(data.Description), Revision: revision}
	payload, _ := protojson.Marshal(message)
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "sli", "update", state.ID.ValueString(), state.CurrentRevisionID.ValueString(), string(payload))
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.SLOs.UpdateSli(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "update SLI", err)
		return
	}
	setSLI(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *sliResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data sliModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	_, err := r.client.SLOs.ArchiveSli(rpcCtx, &alertsv1.ArchiveSliRequest{SliId: data.ID.ValueString(), IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "sli", "archive", data.ID.ValueString(), data.CurrentRevisionID.ValueString())})
	if err != nil && status.Code(err) != codes.NotFound {
		addRPCError(&resp.Diagnostics, "archive SLI", err)
	}
}

func (r *sliResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func sliRevisionInput(data *sliModel) (*alertsv1.SliRevisionInputV1, error) {
	scope := &alertsv1.AlertScopeV1{}
	if err := protoFromJSON(data.Scope, scope); err != nil {
		return nil, fmt.Errorf("scope_json: %w", err)
	}
	kind, _ := sliIndicatorKind(data.IndicatorKind.ValueString())
	aggregation, _ := sliAggregation(data.Aggregation.ValueString())
	missingData, _ := sliMissingData(data.MissingData.ValueString())
	input := &alertsv1.SliRevisionInputV1{
		IndicatorKind: kind, Scope: scope, Owner: sloOwnerProto(data.OwnerUserID, data.OwnerTeamID, data.OwnerDisplayName),
		EligibleEvents: &alertsv1.SliEventDescriptionV1{Count: stringValue(data.EligibleEvents)}, GoodEvents: &alertsv1.SliEventDescriptionV1{Count: stringValue(data.GoodEvents)},
		ExcludedEvents: &alertsv1.SliEventDescriptionV1{Count: stringValue(data.ExcludedEvents)}, Aggregation: aggregation, MissingDataBehavior: missingData,
	}
	if knownNonEmpty(data.LatencyThreshold) {
		value, err := time.ParseDuration(data.LatencyThreshold.ValueString())
		if err != nil {
			return nil, err
		}
		input.LatencyThreshold = durationpb.New(value)
	}
	return input, nil
}

func setSLI(data *sliModel, item *alertsv1.AlertSliV1) {
	data.ID = types.StringValue(item.Id)
	data.SLIKey = types.StringValue(item.SliKey)
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
	data.IndicatorKind = types.StringValue(strings.ToLower(strings.TrimPrefix(revision.IndicatorKind.String(), "SLI_INDICATOR_KIND_V1_")))
	data.Scope = jsonFromProtoPreserving(data.Scope, revision.Scope)
	setOwner(&data.OwnerUserID, &data.OwnerTeamID, revision.Owner)
	if revision.Owner == nil || revision.Owner.DisplayName == "" {
		data.OwnerDisplayName = types.StringNull()
	} else {
		data.OwnerDisplayName = types.StringValue(revision.Owner.DisplayName)
	}
	data.EligibleEvents = eventDescriptionString(revision.EligibleEvents)
	data.GoodEvents = eventDescriptionString(revision.GoodEvents)
	data.ExcludedEvents = eventDescriptionString(revision.ExcludedEvents)
	data.Aggregation = types.StringValue(strings.ToLower(strings.TrimPrefix(revision.Aggregation.String(), "SLI_AGGREGATION_V1_")))
	if revision.LatencyThreshold == nil {
		data.LatencyThreshold = types.StringNull()
	} else {
		serverDuration := revision.LatencyThreshold.AsDuration()
		if configured, err := time.ParseDuration(stringValue(data.LatencyThreshold)); err == nil && configured == serverDuration {
			// Preserve equivalent user formatting such as 0.1ms across refreshes.
		} else {
			data.LatencyThreshold = types.StringValue(strings.ReplaceAll(serverDuration.String(), "µs", "us"))
		}
	}
	data.MissingData = types.StringValue(strings.ToLower(strings.TrimPrefix(revision.MissingDataBehavior.String(), "SLI_MISSING_DATA_BEHAVIOR_V1_")))
	data.RevisionNumber = types.Int64Value(int64(revision.RevisionNumber))
	data.ConfigHash = types.StringValue(revision.ConfigHash)
}

func eventDescriptionString(value *alertsv1.SliEventDescriptionV1) types.String {
	if value == nil {
		return types.StringValue("")
	}
	return types.StringValue(value.Count)
}

func sliIndicatorKind(value string) (alertsv1.SliIndicatorKindV1, bool) {
	items := map[string]alertsv1.SliIndicatorKindV1{
		"availability": alertsv1.SliIndicatorKindV1_SLI_INDICATOR_KIND_V1_AVAILABILITY, "latency": alertsv1.SliIndicatorKindV1_SLI_INDICATOR_KIND_V1_LATENCY,
		"agent_outcome": alertsv1.SliIndicatorKindV1_SLI_INDICATOR_KIND_V1_AGENT_OUTCOME, "eval_quality": alertsv1.SliIndicatorKindV1_SLI_INDICATOR_KIND_V1_EVAL_QUALITY,
		"tool_success": alertsv1.SliIndicatorKindV1_SLI_INDICATOR_KIND_V1_TOOL_SUCCESS, "cost_per_successful_outcome": alertsv1.SliIndicatorKindV1_SLI_INDICATOR_KIND_V1_COST_PER_SUCCESSFUL_OUTCOME,
	}
	result, ok := items[value]
	return result, ok
}

func sliAggregation(value string) (alertsv1.SliAggregationV1, bool) {
	items := map[string]alertsv1.SliAggregationV1{"event_ratio": alertsv1.SliAggregationV1_SLI_AGGREGATION_V1_EVENT_RATIO, "threshold_ratio": alertsv1.SliAggregationV1_SLI_AGGREGATION_V1_THRESHOLD_RATIO}
	result, ok := items[value]
	return result, ok
}

func sliMissingData(value string) (alertsv1.SliMissingDataBehaviorV1, bool) {
	items := map[string]alertsv1.SliMissingDataBehaviorV1{"unknown": alertsv1.SliMissingDataBehaviorV1_SLI_MISSING_DATA_BEHAVIOR_V1_UNKNOWN, "good": alertsv1.SliMissingDataBehaviorV1_SLI_MISSING_DATA_BEHAVIOR_V1_GOOD, "bad": alertsv1.SliMissingDataBehaviorV1_SLI_MISSING_DATA_BEHAVIOR_V1_BAD}
	result, ok := items[value]
	return result, ok
}
