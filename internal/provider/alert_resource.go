package provider

import (
	"context"
	"encoding/json"
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
	"google.golang.org/protobuf/reflect/protoreflect"
)

type alertRecipe int

const (
	recipeAgentQuality alertRecipe = iota
	recipeCost
	recipeSLO
	recipeSymptom
	recipeQueryThreshold
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
	AlertClass         types.String `tfsdk:"alert_class"`
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
func NewQueryThresholdAlertResource() resource.Resource {
	return &alertResource{recipe: recipeQueryThreshold, typeName: "query_threshold_alert"}
}
func (r *alertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.typeName
}
func (r *alertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	canonical := []planmodifier.String{canonicalJSONPlanModifier{}}
	canonicalReplace := []planmodifier.String{canonicalJSONPlanModifier{}, stringplanmodifier.RequiresReplace()}
	alertClass := schema.StringAttribute{Computed: true, Description: "Backend-owned outcome, budget, or symptom classification."}
	if r.recipe == recipeQueryThreshold {
		alertClass = schema.StringAttribute{Required: true, Description: "Query-threshold classification: outcome, budget, or symptom."}
	}
	scope := schema.StringAttribute{Required: true, PlanModifiers: canonicalReplace, Description: "Exact AlertScopeV1 protobuf JSON."}
	recipeConfig := "Exact recipe-specific detector protobuf JSON."
	if r.recipe == recipeSLO {
		scope = schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, Description: "AlertScopeV1 protobuf JSON the server copies from the referenced SLO's SLI revision."}
		recipeConfig = "Exact SloBurnConfigV1 protobuf JSON. Requires slo_id and slo_revision_id (an o11y_slo's id and current_revision_id); the server copies the target and window from that revision, so they are refused here."
	}
	resp.Schema = schema.Schema{Description: "An Observe-mode O11y.one alert definition. Notify activation is intentionally unsupported and fails closed.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}, "slug": schema.StringAttribute{Required: true, PlanModifiers: replace}, "name": schema.StringAttribute{Required: true}, "description": schema.StringAttribute{Required: true},
		"alert_class": alertClass,
		"severity":    schema.StringAttribute{Required: true, PlanModifiers: replace}, "scope_json": scope,
		"owner_json": schema.StringAttribute{Required: true, PlanModifiers: canonical, Description: "Exact AlertOwnerRefV1 protobuf JSON."}, "action_json": schema.StringAttribute{Required: true, PlanModifiers: canonical, Description: "Exact AlertActionV1 protobuf JSON."}, "evaluation_settings_json": schema.StringAttribute{Required: true, PlanModifiers: canonical, Description: "Exact AlertEvaluationSettingsV1 protobuf JSON."},
		"evaluation_interval_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(60), Description: "Evaluation schedule interval in seconds."},
		"sample_guard_json":           schema.StringAttribute{Required: true, PlanModifiers: canonical, Description: "Exact AlertSampleGuardV1 protobuf JSON."}, "recipe_config_json": schema.StringAttribute{Optional: true, PlanModifiers: canonicalReplace, Description: recipeConfig},
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
	if r.recipe == recipeQueryThreshold {
		if !data.AlertClass.IsUnknown() && (data.AlertClass.IsNull() || !validQueryAlertClass(data.AlertClass.ValueString())) {
			resp.Diagnostics.AddAttributeError(path.Root("alert_class"), "Invalid alert class", "query-threshold alerts require alert_class to be outcome, budget, or symptom")
		}
	}
	for name, value := range map[string]types.String{"scope_json": data.Scope, "owner_json": data.Owner, "action_json": data.Action, "evaluation_settings_json": data.EvaluationSettings, "sample_guard_json": data.SampleGuard, "recipe_config_json": data.RecipeConfig} {
		if !value.IsNull() && !value.IsUnknown() && value.ValueString() != "" {
			if err := validationutil.JSONDocument(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid JSON object", err.Error())
			}
		}
	}
	typedFields := map[string]struct {
		value  types.String
		target proto.Message
	}{
		"scope_json":               {data.Scope, &alertsv1.AlertScopeV1{}},
		"owner_json":               {data.Owner, &alertsv1.AlertOwnerRefV1{}},
		"action_json":              {data.Action, &alertsv1.AlertActionV1{}},
		"evaluation_settings_json": {data.EvaluationSettings, &alertsv1.AlertEvaluationSettingsV1{}},
		"sample_guard_json":        {data.SampleGuard, &alertsv1.AlertSampleGuardV1{}},
	}
	for name, field := range typedFields {
		if knownNonEmpty(field.value) {
			if err := protoFromJSON(field.value, field.target); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid typed alert configuration", err.Error())
			}
		}
	}
	if knownNonEmpty(data.RecipeConfig) {
		var target proto.Message
		switch r.recipe {
		case recipeAgentQuality:
			target = &alertsv1.AgentQualityRegressionConfigV1{}
		case recipeCost:
			target = &alertsv1.CostPerSuccessConfigV1{}
		case recipeSLO:
			target = &alertsv1.SloBurnConfigV1{}
		case recipeSymptom:
			target = &alertsv1.AdvancedSignalConfigV1{}
		case recipeQueryThreshold:
			target = &alertsv1.QueryThresholdConfigV1{}
		}
		if target != nil {
			if err := protoFromJSON(data.RecipeConfig, target); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root("recipe_config_json"), "Invalid typed detector configuration", err.Error())
			} else if slo, ok := target.(*alertsv1.SloBurnConfigV1); ok {
				if derived := clearSLOBurnDerived(slo); len(derived) > 0 {
					resp.Diagnostics.AddAttributeError(path.Root("recipe_config_json"), "Server-derived SLO burn fields", strings.Join(derived, ", ")+" are copied from the referenced SLO revision; configure them on the o11y_slo.")
				}
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
	base, err := r.createBase(&data)
	if err != nil {
		addRPCError(&resp.Diagnostics, "build alert request", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	var result *alertsv1.CreateAlertDefinitionResponse
	switch r.recipe {
	case recipeAgentQuality:
		config := &alertsv1.AgentQualityRegressionConfigV1{}
		if err = protoFromJSON(data.RecipeConfig, config); err == nil {
			request := &alertsv1.CreateAgentQualityRegressionAlertRequest{Base: base, RecipeConfig: config}
			r.setCreateIdempotency(base, request, data.Slug.ValueString())
			result, err = r.client.Definitions.CreateAgentQualityRegressionAlert(rpcCtx, request)
		}
	case recipeCost:
		config := &alertsv1.CostPerSuccessConfigV1{}
		if err = protoFromJSON(data.RecipeConfig, config); err == nil {
			request := &alertsv1.CreateCostPerSuccessAlertRequest{Base: base, RecipeConfig: config}
			r.setCreateIdempotency(base, request, data.Slug.ValueString())
			result, err = r.client.Definitions.CreateCostPerSuccessAlert(rpcCtx, request)
		}
	case recipeSLO:
		config := &alertsv1.SloBurnConfigV1{}
		if err = protoFromJSON(data.RecipeConfig, config); err == nil {
			request := &alertsv1.CreateSloBurnAlertRequest{Base: base, RecipeConfig: config}
			r.setCreateIdempotency(base, request, data.Slug.ValueString())
			result, err = r.client.Definitions.CreateSloBurnAlert(rpcCtx, request)
		}
	case recipeSymptom:
		config := &alertsv1.AdvancedSignalConfigV1{}
		if err = protoFromJSON(data.RecipeConfig, config); err == nil {
			request := &alertsv1.CreateAdvancedSignalAlertRequest{Base: base, Condition: config}
			r.setCreateIdempotency(base, request, data.Slug.ValueString())
			result, err = r.client.Definitions.CreateAdvancedSignalAlert(rpcCtx, request)
		}
	case recipeQueryThreshold:
		config := &alertsv1.QueryThresholdConfigV1{}
		if err = protoFromJSON(data.RecipeConfig, config); err == nil {
			request := &alertsv1.CreateQueryThresholdAlertRequest{Base: base, Query: config, AlertClass: queryAlertClass(data.AlertClass.ValueString())}
			r.setCreateIdempotency(base, request, data.Slug.ValueString())
			result, err = r.client.Definitions.CreateQueryThresholdAlert(rpcCtx, request)
		}
	}
	if err != nil {
		addRPCError(&resp.Diagnostics, "create Observe alert", err)
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
	owner := &alertsv1.AlertOwnerRefV1{}
	err := protoFromJSON(plan.Owner, owner)
	if err != nil {
		addRPCError(&resp.Diagnostics, "build owner", err)
		return
	}
	action := &alertsv1.AlertActionV1{}
	evaluation := &alertsv1.AlertEvaluationSettingsV1{}
	guard := &alertsv1.AlertSampleGuardV1{}
	for field, target := range map[string]proto.Message{
		"action":              action,
		"evaluation_settings": evaluation,
		"sample_guard":        guard,
	} {
		var value types.String
		switch field {
		case "action":
			value = plan.Action
		case "evaluation_settings":
			value = plan.EvaluationSettings
		default:
			value = plan.SampleGuard
		}
		if err = protoFromJSON(value, target); err != nil {
			addRPCError(&resp.Diagnostics, "build "+field, err)
			return
		}
	}
	rpcCtx, cancel := r.client.Context(ctx)
	message := &alertsv1.UpdateObserveAlertRequest{DefinitionId: state.ID.ValueString(), Name: plan.Name.ValueString(), Description: plan.Description.ValueString(), Owner: owner, Action: action, EvaluationSettings: evaluation, SampleGuard: guard, EvaluationIntervalSeconds: proto.Int64(plan.EvaluationInterval.ValueInt64())}
	payload, marshalErr := protojson.Marshal(message)
	if marshalErr != nil {
		addRPCError(&resp.Diagnostics, "marshal update payload", marshalErr)
		return
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), r.typeName, "update", state.ID.ValueString(), string(payload))
	_, err = r.client.Definitions.UpdateObserve(rpcCtx, message)
	if err != nil {
		cancel()
		addRPCError(&resp.Diagnostics, "update Observe alert", err)
		return
	}
	item, err := r.client.Definitions.GetDefinition(rpcCtx, &alertsv1.GetAlertDefinitionRequest{Id: state.ID.ValueString()})
	cancel()
	if err != nil {
		addRPCError(&resp.Diagnostics, "read updated Observe alert", err)
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
			plan.Mode = types.StringValue("observe")
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
func (r *alertResource) createBase(data *alertModel) (*alertsv1.AlertRecipeBaseV1, error) {
	scope := &alertsv1.AlertScopeV1{}
	owner := &alertsv1.AlertOwnerRefV1{}
	action := &alertsv1.AlertActionV1{}
	evaluation := &alertsv1.AlertEvaluationSettingsV1{}
	guard := &alertsv1.AlertSampleGuardV1{}
	fields := []struct {
		name   string
		value  types.String
		target proto.Message
	}{
		{"scope", data.Scope, scope},
		{"owner", data.Owner, owner},
		{"action", data.Action, action},
		{"evaluation_settings", data.EvaluationSettings, evaluation},
		{"sample_guard", data.SampleGuard, guard},
	}
	for _, field := range fields {
		if err := protoFromJSON(field.value, field.target); err != nil {
			return nil, fmt.Errorf("decode %s: %w", field.name, err)
		}
	}
	severity, _ := alertSeverity(data.Severity.ValueString())
	base := &alertsv1.AlertRecipeBaseV1{OrgId: r.client.OrgID(), Slug: data.Slug.ValueString(), Name: data.Name.ValueString(), Description: data.Description.ValueString(), Severity: severity, Scope: scope, Owner: owner, Action: action, EvaluationSettings: evaluation, SampleGuard: guard, EvaluationIntervalSeconds: data.EvaluationInterval.ValueInt64()}
	return base, nil
}

func (r *alertResource) setCreateIdempotency(base *alertsv1.AlertRecipeBaseV1, request proto.Message, slug string) {
	payload, _ := protojson.Marshal(request)
	base.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), r.typeName, "create", slug, string(payload))
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
	data.AlertClass = types.StringValue(strings.ToLower(strings.TrimPrefix(item.Class.String(), "ALERT_CLASS_V1_")))
	data.Severity = types.StringValue(strings.ToLower(strings.TrimPrefix(item.Severity.String(), "ALERT_SEVERITY_V1_")))
	data.Scope = jsonFromProtoPreserving(data.Scope, item.Scope)
	data.Owner = alertOwnerJSONFromProtoPreserving(data.Owner, item.Owner)
	data.Action = jsonFromProtoPreserving(data.Action, item.Action)
	data.EvaluationSettings = alertEvaluationSettingsJSONFromProtoPreserving(
		data.EvaluationSettings,
		data.EvaluationInterval,
		item.EvaluationSettings,
	)
	data.EvaluationInterval = types.Int64Value(item.EvaluationIntervalSeconds)
	data.SampleGuard = jsonFromProtoPreserving(data.SampleGuard, item.SampleGuard)
	data.RecipeConfig = jsonFromProtoPreserving(data.RecipeConfig, detectorRecipeConfig(item.DetectorConfig))
	data.Mode = types.StringValue(strings.ToLower(strings.TrimPrefix(item.Mode.String(), "ALERT_MODE_V1_")))
	data.Paused = types.BoolValue(item.Mode == alertsv1.AlertModeV1_ALERT_MODE_V1_DISABLED)
	data.Notify = types.BoolValue(item.Mode == alertsv1.AlertModeV1_ALERT_MODE_V1_NOTIFY)
	data.RevisionID = types.StringValue(item.CurrentRevisionId)
}

func alertOwnerJSONFromProtoPreserving(configured types.String, value *alertsv1.AlertOwnerRefV1) types.String {
	if value != nil && knownNonEmpty(configured) {
		candidate := &alertsv1.AlertOwnerRefV1{}
		if err := protoFromJSON(configured, candidate); err == nil && sameAlertOwnerIdentity(candidate, value) && configuredAlertOwnerStatusMatches(configured, value) {
			canonical, canonicalErr := canonicalJSONString(configured)
			if canonicalErr == nil {
				return canonical
			}
		}
	}
	return jsonFromProto(value)
}

func configuredAlertOwnerStatusMatches(configured types.String, value *alertsv1.AlertOwnerRefV1) bool {
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configured.ValueString()), &document); err != nil {
		return false
	}
	raw, configuredActive := document["active"]
	if !configuredActive {
		return true
	}
	var active bool
	return json.Unmarshal(raw, &active) == nil && active == value.Active
}

func alertEvaluationSettingsJSONFromProtoPreserving(
	configured types.String,
	interval types.Int64,
	value *alertsv1.AlertEvaluationSettingsV1,
) types.String {
	if value != nil && knownNonEmpty(configured) {
		candidate := &alertsv1.AlertEvaluationSettingsV1{}
		if err := protoFromJSON(configured, candidate); err == nil {
			if !interval.IsNull() && !interval.IsUnknown() {
				candidate.IntervalSeconds = interval.ValueInt64()
			}
			if candidate.NoDataBehavior == alertsv1.AlertNoDataBehaviorV1_ALERT_NO_DATA_BEHAVIOR_V1_UNSPECIFIED {
				candidate.NoDataBehavior = alertsv1.AlertNoDataBehaviorV1_ALERT_NO_DATA_BEHAVIOR_V1_HOLD_STATE
			}
			if proto.Equal(candidate, value) {
				canonical, canonicalErr := canonicalJSONString(configured)
				if canonicalErr == nil {
					return canonical
				}
			}
		}
	}
	return jsonFromProto(value)
}

func sameAlertOwnerIdentity(left, right *alertsv1.AlertOwnerRefV1) bool {
	if left == nil || right == nil {
		return false
	}
	if teamID := left.GetTeamId(); teamID != "" {
		return teamID == right.GetTeamId()
	}
	if userID := left.GetUserId(); userID != "" {
		return userID == right.GetUserId()
	}
	return false
}

func detectorRecipeConfig(config *alertsv1.AlertDetectorConfigV1) proto.Message {
	if config == nil {
		return nil
	}
	switch value := config.Config.(type) {
	case *alertsv1.AlertDetectorConfigV1_AgentQualityRegression:
		return value.AgentQualityRegression
	case *alertsv1.AlertDetectorConfigV1_CostPerSuccess:
		return value.CostPerSuccess
	case *alertsv1.AlertDetectorConfigV1_SloBurn:
		inputs := proto.Clone(value.SloBurn).(*alertsv1.SloBurnConfigV1)
		clearSLOBurnDerived(inputs)
		return inputs
	case *alertsv1.AlertDetectorConfigV1_AdvancedSignal:
		return value.AdvancedSignal
	case *alertsv1.AlertDetectorConfigV1_QueryThreshold:
		return value.QueryThreshold
	default:
		return nil
	}
}

// create_slo_burn_alert overwrites these from the referenced SLO revision, so they are not inputs.
var sloBurnDerivedFields = []protoreflect.Name{"slo_window_seconds", "target_percent", "window_mode", "calendar_period", "calendar_timezone", "revision_effective_from"}

// clearSLOBurnDerived returns the names of the derived fields that were set.
func clearSLOBurnDerived(config *alertsv1.SloBurnConfigV1) []string {
	message := config.ProtoReflect()
	var set []string
	for _, name := range sloBurnDerivedFields {
		field := message.Descriptor().Fields().ByName(name)
		if message.Has(field) {
			set = append(set, string(name))
			message.Clear(field)
		}
	}
	return set
}

func validQueryAlertClass(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "outcome", "budget", "symptom":
		return true
	default:
		return false
	}
}
func queryAlertClass(value string) alertsv1.AlertClassV1 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "outcome":
		return alertsv1.AlertClassV1_ALERT_CLASS_V1_OUTCOME
	case "budget":
		return alertsv1.AlertClassV1_ALERT_CLASS_V1_BUDGET
	default:
		return alertsv1.AlertClassV1_ALERT_CLASS_V1_SYMPTOM
	}
}
func alertSeverity(value string) (alertsv1.AlertSeverityV1, bool) {
	values := map[string]alertsv1.AlertSeverityV1{"info": alertsv1.AlertSeverityV1_ALERT_SEVERITY_V1_INFO, "warning": alertsv1.AlertSeverityV1_ALERT_SEVERITY_V1_WARNING, "critical": alertsv1.AlertSeverityV1_ALERT_SEVERITY_V1_CRITICAL}
	result, ok := values[value]
	return result, ok
}
