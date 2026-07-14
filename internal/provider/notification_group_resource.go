package provider

import (
	"context"
	"encoding/json"
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
	"google.golang.org/protobuf/proto"
)

var _ resource.ResourceWithConfigure = &notificationGroupResource{}
var _ resource.ResourceWithImportState = &notificationGroupResource{}
var _ resource.ResourceWithModifyPlan = &notificationGroupResource{}
var _ resource.ResourceWithValidateConfig = &notificationGroupResource{}

type notificationGroupResource struct{ client *client.Client }
type notificationGroupModel struct {
	ID       types.String `tfsdk:"id"`
	GroupKey types.String `tfsdk:"group_key"`
	Name     types.String `tfsdk:"name"`
	Enabled  types.Bool   `tfsdk:"enabled"`
	Members  types.String `tfsdk:"members_json"`
	Revision types.Int64  `tfsdk:"revision"`
}
type notificationGroupMemberDocument struct {
	Kind     string `json:"kind"`
	MemberID string `json:"member_id"`
	Position int32  `json:"position"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

func NewNotificationGroupResource() resource.Resource { return &notificationGroupResource{} }
func (r *notificationGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_notification_group"
}
func (r *notificationGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "A bounded reusable group of alert contacts, provider destinations, or nested groups.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}, "group_key": schema.StringAttribute{Required: true}, "name": schema.StringAttribute{Required: true}, "enabled": schema.BoolAttribute{Required: true},
		"members_json": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{canonicalJSONPlanModifier{}}, Description: "Ordered JSON array of {kind, member_id, position, enabled}; kind is contact, destination, or group."},
		"revision":     schema.Int64Attribute{Computed: true},
	}}
}
func (r *notificationGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *notificationGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var data notificationGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	canonical, err := canonicalJSONString(data.Members)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("members_json"), "Invalid members JSON", err.Error())
		return
	}
	data.Members = canonical
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &data)...)
}
func (r *notificationGroupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data notificationGroupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	for name, value := range map[string]types.String{"group_key": data.GroupKey, "name": data.Name} {
		if !value.IsUnknown() && !value.IsNull() {
			if err := validationutil.NonEmpty(value.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Value must not be empty", err.Error())
			}
		}
	}
	if !data.Members.IsUnknown() && !data.Members.IsNull() {
		if _, err := decodeNotificationGroupMembers(data.Members.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("members_json"), "Invalid notification group members", err.Error())
		}
	}
}
func (r *notificationGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data notificationGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	message, err := r.request(&data, "create", data.GroupKey.ValueString(), nil)
	if err != nil {
		addRPCError(&resp.Diagnostics, "build notification group", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.CreateNotificationGroup(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "create notification group", err)
		return
	}
	setNotificationGroup(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *notificationGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data notificationGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.GetNotificationGroup(rpcCtx, &alertsv1.GetAlertResourceRequest{Id: data.ID.ValueString(), OrgId: r.client.OrgID()})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		addRPCError(&resp.Diagnostics, "get notification group", err)
		return
	}
	setNotificationGroup(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *notificationGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state notificationGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = state.ID
	message, err := r.request(&data, "update", state.ID.ValueString(), proto.Int64(state.Revision.ValueInt64()))
	if err != nil {
		addRPCError(&resp.Diagnostics, "build notification group", err)
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	item, err := r.client.Notifications.UpdateNotificationGroup(rpcCtx, message)
	if err != nil {
		addRPCError(&resp.Diagnostics, "update notification group", err)
		return
	}
	setNotificationGroup(&data, item)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func (r *notificationGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data notificationGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := r.client.Context(ctx)
	defer cancel()
	_, err := r.client.Notifications.SetNotificationGroupArchived(rpcCtx, &alertsv1.SetAlertNotificationGroupArchivedRequest{OrgId: r.client.OrgID(), GroupId: data.ID.ValueString(), ExpectedRevision: data.Revision.ValueInt64(), Archived: true, Reason: "removed from Terraform configuration", IdempotencyKey: idempotency.Key(r.client.TenantID(), r.client.OrgID(), "notification_group", "archive", data.ID.ValueString())})
	if err != nil {
		addRPCError(&resp.Diagnostics, "archive notification group", err)
	}
}
func (r *notificationGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
func (r *notificationGroupResource) request(data *notificationGroupModel, operation, identity string, expected *int64) (*alertsv1.UpsertAlertNotificationGroupRequest, error) {
	members, err := decodeNotificationGroupMembers(data.Members.ValueString())
	if err != nil {
		return nil, err
	}
	message := &alertsv1.UpsertAlertNotificationGroupRequest{Id: data.ID.ValueString(), GroupKey: data.GroupKey.ValueString(), Name: data.Name.ValueString(), Enabled: data.Enabled.ValueBool(), ExpectedRevision: expected, Members: members, OrgId: r.client.OrgID()}
	payload, err := protojson.Marshal(message)
	if err != nil {
		return nil, err
	}
	message.IdempotencyKey = idempotency.Key(r.client.TenantID(), r.client.OrgID(), "notification_group", operation, identity, string(payload))
	return message, nil
}
func decodeNotificationGroupMembers(value string) ([]*alertsv1.AlertNotificationGroupMemberV1, error) {
	var docs []notificationGroupMemberDocument
	if err := json.Unmarshal([]byte(value), &docs); err != nil {
		return nil, err
	}
	if len(docs) > 100 {
		return nil, fmt.Errorf("members_json must contain at most 100 members")
	}
	positions := map[int32]bool{}
	targets := map[string]bool{}
	result := make([]*alertsv1.AlertNotificationGroupMemberV1, 0, len(docs))
	for i, doc := range docs {
		kind, ok := notificationGroupMemberKind(doc.Kind)
		if !ok {
			return nil, fmt.Errorf("members_json[%d].kind must be contact, destination, or group", i)
		}
		if err := validationutil.UUID(doc.MemberID); err != nil {
			return nil, fmt.Errorf("members_json[%d].member_id %w", i, err)
		}
		if doc.Position < 0 || doc.Position > 9999 || positions[doc.Position] {
			return nil, fmt.Errorf("members_json[%d].position must be unique and between 0 and 9999", i)
		}
		target := doc.Kind + ":" + doc.MemberID
		if targets[target] {
			return nil, fmt.Errorf("members_json[%d] duplicates an existing member", i)
		}
		positions[doc.Position] = true
		targets[target] = true
		enabled := true
		if doc.Enabled != nil {
			enabled = *doc.Enabled
		}
		result = append(result, &alertsv1.AlertNotificationGroupMemberV1{Kind: kind, MemberId: doc.MemberID, Position: doc.Position, Enabled: enabled})
	}
	return result, nil
}
func notificationGroupMemberKind(value string) (alertsv1.AlertNotificationGroupMemberKindV1, bool) {
	kinds := map[string]alertsv1.AlertNotificationGroupMemberKindV1{"contact": alertsv1.AlertNotificationGroupMemberKindV1_ALERT_NOTIFICATION_GROUP_MEMBER_KIND_V1_CONTACT, "destination": alertsv1.AlertNotificationGroupMemberKindV1_ALERT_NOTIFICATION_GROUP_MEMBER_KIND_V1_DESTINATION, "group": alertsv1.AlertNotificationGroupMemberKindV1_ALERT_NOTIFICATION_GROUP_MEMBER_KIND_V1_GROUP}
	kind, ok := kinds[strings.ToLower(value)]
	return kind, ok
}
func setNotificationGroup(data *notificationGroupModel, item *alertsv1.AlertNotificationGroupV1) {
	data.ID = types.StringValue(item.Id)
	data.GroupKey = types.StringValue(item.GroupKey)
	data.Name = types.StringValue(item.Name)
	data.Enabled = types.BoolValue(item.Enabled)
	data.Revision = types.Int64Value(item.Revision)
	docs := make([]notificationGroupMemberDocument, 0, len(item.Members))
	for _, member := range item.Members {
		enabled := member.Enabled
		docs = append(docs, notificationGroupMemberDocument{Kind: strings.ToLower(strings.TrimPrefix(member.Kind.String(), "ALERT_NOTIFICATION_GROUP_MEMBER_KIND_V1_")), MemberID: member.MemberId, Position: member.Position, Enabled: &enabled})
	}
	raw, _ := json.Marshal(docs)
	canonical, _ := canonicalJSONString(types.StringValue(string(raw)))
	data.Members = canonical
}
