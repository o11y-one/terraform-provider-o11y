package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type alertRecipe int

const (
	recipeAgentQuality alertRecipe = iota
	recipeCost
	recipeSLO
	recipeSymptom
)

type alertResource struct {
	client   *client.Client
	recipe   alertRecipe
	typeName string
}
type alertModel struct {
	ID                 types.String `tfsdk:"id"`
	Slug               types.String `tfsdk:"slug"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Severity           types.String `tfsdk:"severity"`
	Scope              types.String `tfsdk:"scope_json"`
	Owner              types.String `tfsdk:"owner_json"`
	Action             types.String `tfsdk:"action_json"`
	EvaluationSettings types.String `tfsdk:"evaluation_settings_json"`
	EvaluationInterval types.Int64  `tfsdk:"evaluation_interval_seconds"`
	SampleGuard        types.String `tfsdk:"sample_guard_json"`
	RecipeConfig       types.String `tfsdk:"recipe_config_json"`
	Paused             types.Bool   `tfsdk:"paused"`
	Notify             types.Bool   `tfsdk:"notify"`
	Mode               types.String `tfsdk:"mode"`
	RevisionID         types.String `tfsdk:"revision_id"`
}

var _ resource.ResourceWithConfigure = &alertResource{}
var _ resource.ResourceWithImportState = &alertResource{}
var _ resource.ResourceWithValidateConfig = &alertResource{}
var _ resource.ResourceWithModifyPlan = &alertResource{}

func NewAgentQualityAlertResource() resource.Resource {
	return &alertResource{recipe: recipeAgentQuality, typeName: "agent_quality_alert"}
}
func NewCostAlertResource() resource.Resource {
	return &alertResource{recipe: recipeCost, typeName: "cost_per_success_alert"}
}
func NewSLOAlertResource() resource.Resource {
	return &alertResource{recipe: recipeSLO, typeName: "slo_burn_alert"}
}
func NewSymptomAlertResource() resource.Resource {
	return &alertResource{recipe: recipeSymptom, typeName: "advanced_signal_alert"}
}
func (r *alertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.typeName
}
func (r *alertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	canonical := []planmodifier.String{canonicalJSONPlanModifier{}}
	canonicalReplace := []planmodifier.String{canonicalJSONPlanModifier{}, stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{Description: "A shadow-only O11y.one alert definition. Notify activation is intentionally unsupported and fails closed.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}, "slug": schema.StringAttribute{Required: true, PlanModifiers: replace}, "name": schema.StringAttribute{Required: true}, "description": schema.StringAttribute{Required: true},
		"severity": schema.StringAttribute{Required: true, PlanModifiers: replace}, "scope_json": schema.StringAttribute{Required: true, PlanModifiers: canonicalReplace},
		"owner_json": schema.StringAttribute{Required: true, PlanModifiers: canonical}, "action_json": schema.StringAttribute{Required: true, PlanModifiers: canonical}, "evaluation_settings_json": schema.StringAttribute{Required: true, PlanModifiers: canonical, Description: "Detector evaluation settings as JSON."},
		"evaluation_interval_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(60), Description: "Evaluation schedule interval in seconds."},
		"sample_guard_json":           schema.StringAttribute{Required: true, PlanModifiers: canonical}, "recipe_config_json": schema.StringAttribute{Optional: true, PlanModifiers: canonicalReplace},
		"paused": schema.BoolAttribute{Required: true}, "notify": schema.BoolAttribute{Required: true, Description: "Must be false. Notify activation requires an out-of-band, audited API workflow."},
		"mode": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}, "revision_id": schema.StringAttribute{Computed: true},
	}}
}
func (r *alertResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var data alertModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fields := []*types.String{&data.Scope, &data.Owner, &data.Action, &data.SampleGuard, &data.RecipeConfig}
	for _, field := range fields {
		canonical, err := canonicalJSONString(*field)
		if err != nil {
			resp.Diagnostics.AddError("Invalid alert JSON", err.Error())
			return
		}
		*field = canonical
	}
	canonicalEvaluation, err := canonicalJSONString(data.EvaluationSettings)
	if err != nil {
		resp.Diagnostics.AddError("Invalid alert evaluation settings JSON", err.Error())
		return
	}
	data.EvaluationSettings = canonicalEvaluation
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &data)...)
}
func (r *alertResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *alertResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data alertModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !data.Notify.IsUnknown() && !data.Notify.IsNull() && data.Notify.ValueBool() {
		resp.Diagnostics.AddAttributeError(path.Root("notify"), "Notify activation is not supported", validationutil.NotifyActivation(true).Error()+". ActivateNotifyMode is intentionally not called by this provider.")
	}
	for name, value := range map[string]types.String{"slug": data.Slug, "name": data.Name, "description": data.Description} {
		if !value.IsUnknown() && !value.IsNull() {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	if !data.Severity.IsUnknown() && !data.Severity.IsNull() {
		if _, ok := alertSeverity(data.Severity.ValueString()); !ok {
			resp.Diagnostics.AddAttributeError(path.Root("severity"), "Invalid severity", "severity must be info, warning, or critical")
		}
	}
	for name, value := range map[string]types.String{"scope_json": data.Scope, "owner_json": data.Owner, "action_json": data.Action, "evaluation_settings_json": data.EvaluationSettings, "sample_guard_json": data.SampleGuard, "recipe_config_json": data.RecipeConfig} {
		if !value.IsNull() && !value.IsUnknown() && value.ValueString() != "" {
			if err := validationutil.JSONDocument(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid JSON object", err.Error())
			}
		}
	}
}
func (r *alertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data alertModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	base, recipeConfig, err := r.createPayload(&data)
	if err != nil {
		addRPCError(&resp.Diagnostics, "build alert request", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	var result *alertsv1.CreateAlertDefinitionResponse
	switch r.recipe {
	case recipeAgentQuality:
		result, err = r.client.Definitions.CreateAgentQualityRegressionAlert(rpcCtx, &alertsv1.CreateAgentQualityRegressionAlertRequest{Base: base, RecipeConfig: recipeConfig})
	case recipeCost:
		result, err = r.client.Definitions.CreateCostPerSuccessAlert(rpcCtx, &alertsv1.CreateCostPerSuccessAlertRequest{Base: base, RecipeConfig: recipeConfig})
	case recipeSLO:
		result, err = r.client.Definitions.CreateSloBurnAlert(rpcCtx, &alertsv1.CreateSloBurnAlertRequest{Base: base, RecipeConfig: recipeConfig})
	case recipeSymptom:
		result, err = r.client.Definitions.CreateAdvancedSignalAlert(rpcCtx, &alertsv1.CreateAdvancedSignalAlertRequest{Base: base, Condition: recipeConfig})
	}
	if err != nil {
		addRPCError(&resp.Diagnostics, "create shadow alert", err)
		return
	}
	desiredPaused := data.Paused.ValueBool()
	setAlert(&data, result.Definition)
	data.RevisionID = types.StringValue(result.RevisionId)
	if desiredPaused {
		if err := r.pause(ctx, data.ID.ValueString()); err != nil {
			addRPCError(&resp.Diagnostics, "pause newly created alert", err)
			return
		}
		data.Mode = types.StringValue("disabled")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *alertResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data alertModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Definitions.GetDefinition(rpcCtx, &alertsv1.GetAlertDefinitionRequest{Id: data.ID.ValueString()})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		addRPCError(&resp.Diagnostics, "get alert definition", err)
		return
	}
	setAlert(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *alertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state alertModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	plan.RevisionID = state.RevisionID
	owner, err := structFromJSON(plan.Owner)
	if err != nil {
		addRPCError(&resp.Diagnostics, "build owner", err)
		return
	}
	action, _ := structFromJSON(plan.Action)
	evaluation, _ := structFromJSON(plan.EvaluationSettings)
	guard, _ := structFromJSON(plan.SampleGuard)
	rpcCtx, cancel := r.client.Context(ctx)
	message := &alertsv1.UpdateShadowAlertRequest{DefinitionId: state.ID.ValueString(), Name: plan.Name.ValueString(), Description: plan.Description.ValueString(), Owner: owner, Action: action, EvaluationSettings: evaluation, SampleGuard: guard, EvaluationIntervalSeconds: proto.Int64(plan.EvaluationInterval.ValueInt64())}
	payload, marshalErr := protojson.Marshal(message)
	if marshalErr != nil {
		addRPCError(&resp.Diagnostics, "marshal update payload", marshalErr)
		return
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), r.typeName, "update", state.ID.ValueString(), string(payload))
	_, err = r.client.Definitions.UpdateShadow(rpcCtx, message)
	if err != nil {
		cancel()
		addRPCError(&resp.Diagnostics, "update shadow alert", err)
		return
	}
	item, err := r.client.Definitions.GetDefinition(rpcCtx, &alertsv1.GetAlertDefinitionRequest{Id: state.ID.ValueString()})
	cancel()
	if err != nil {
		addRPCError(&resp.Diagnostics, "read updated shadow alert", err)
		return
	}
	desiredPaused := plan.Paused.ValueBool()
	setAlert(&plan, item)
	if desiredPaused != state.Paused.ValueBool() {
		if desiredPaused {
			err = r.pause(ctx, plan.ID.ValueString())
			plan.Mode = types.StringValue("disabled")
		} else {
			err = r.resume(ctx, plan.ID.ValueString(), plan.RevisionID.ValueString())
			plan.Mode = types.StringValue("shadow")
		}
		if err != nil {
			addRPCError(&resp.Diagnostics, "change alert pause state", err)
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *alertResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data alertModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	message := &alertsv1.ArchiveAlertDefinitionV2Request{
		DefinitionId:       data.ID.ValueString(),
		ExpectedRevisionId: data.RevisionID.ValueString(),
		Reason:             "Terraform resource removed from configuration",
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), r.typeName, "archive", data.ID.ValueString(), data.RevisionID.ValueString())
	result, err := r.client.Definitions.ArchiveDefinitionV2(rpcCtx, message)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return
		}
		addRPCError(&resp.Diagnostics, "archive alert definition", err)
		return
	}
	if !result.Ok {
		resp.Diagnostics.AddError("Alert archive rejected", result.Message)
	}
}
func (r *alertResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
func (r *alertResource) createPayload(data *alertModel) (*alertsv1.AlertRecipeBaseV1, *structpb.Struct, error) {
	scope, err := structFromJSON(data.Scope)
	if err != nil {
		return nil, nil, err
	}
	owner, err := structFromJSON(data.Owner)
	if err != nil {
		return nil, nil, err
	}
	action, err := structFromJSON(data.Action)
	if err != nil {
		return nil, nil, err
	}
	evaluation, err := structFromJSON(data.EvaluationSettings)
	if err != nil {
		return nil, nil, err
	}
	guard, err := structFromJSON(data.SampleGuard)
	if err != nil {
		return nil, nil, err
	}
	config, err := structFromJSON(data.RecipeConfig)
	if err != nil {
		return nil, nil, err
	}
	severity, _ := alertSeverity(data.Severity.ValueString())
	base := &alertsv1.AlertRecipeBaseV1{OrgId: r.client.OrgID(), Slug: data.Slug.ValueString(), Name: data.Name.ValueString(), Description: data.Description.ValueString(), Severity: severity, Scope: scope, Owner: owner, Action: action, EvaluationSettings: evaluation, SampleGuard: guard, EvaluationIntervalSeconds: data.EvaluationInterval.ValueInt64()}
	payload, err := protojson.Marshal(&alertsv1.CreateAgentQualityRegressionAlertRequest{Base: base, RecipeConfig: config})
	if err != nil {
		return nil, nil, err
	}
	base.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), r.typeName, "create", data.Slug.ValueString(), string(payload))
	return base, config, nil
}
func (r *alertResource) pause(ctx context.Context, id string) error {
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Runtime.Pause(rpcCtx, &alertsv1.PauseAlertRequest{DefinitionId: id, Reason: "Terraform managed pause", IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), r.typeName, "pause", id)})
	if err == nil && !result.Ok {
		return fmt.Errorf("pause rejected: %s", result.Message)
	}
	return err
}
func (r *alertResource) resume(ctx context.Context, id, revision string) error {
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	result, err := r.client.Runtime.Resume(rpcCtx, &alertsv1.ResumeAlertRequest{DefinitionId: id, RevisionId: revision, Reason: "Terraform managed resume", IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), r.typeName, "resume", id)})
	if err == nil && !result.Ok {
		return fmt.Errorf("resume rejected: %s", result.Message)
	}
	return err
}
func setAlert(data *alertModel, item *alertsv1.AlertDefinitionV1) {
	data.ID = types.StringValue(item.Id)
	data.Slug = types.StringValue(item.Slug)
	data.Name = types.StringValue(item.Name)
	data.Description = types.StringValue(item.Description)
	data.Severity = types.StringValue(strings.ToLower(strings.TrimPrefix(item.Severity.String(), "ALERT_SEVERITY_V1_")))
	data.Scope = jsonFromStruct(item.Scope)
	data.Owner = jsonFromStruct(item.Owner)
	data.Action = jsonFromStruct(item.Action)
	data.EvaluationSettings = jsonFromStruct(item.EvaluationSettings)
	data.EvaluationInterval = types.Int64Value(item.EvaluationIntervalSeconds)
	data.SampleGuard = jsonFromStruct(item.SampleGuard)
	data.RecipeConfig = jsonFromStruct(item.RecipeConfig)
	data.Mode = types.StringValue(strings.ToLower(strings.TrimPrefix(item.Mode.String(), "ALERT_MODE_V1_")))
	data.Paused = types.BoolValue(item.Mode == alertsv1.AlertModeV1_ALERT_MODE_V1_DISABLED)
	data.Notify = types.BoolValue(item.Mode == alertsv1.AlertModeV1_ALERT_MODE_V1_NOTIFY)
	data.RevisionID = types.StringValue(item.CurrentRevisionId)
}
func alertSeverity(value string) (alertsv1.AlertSeverityV1, bool) {
	values := map[string]alertsv1.AlertSeverityV1{"info": alertsv1.AlertSeverityV1_ALERT_SEVERITY_V1_INFO, "warning": alertsv1.AlertSeverityV1_ALERT_SEVERITY_V1_WARNING, "critical": alertsv1.AlertSeverityV1_ALERT_SEVERITY_V1_CRITICAL}
	result, ok := values[value]
	return result, ok
}
