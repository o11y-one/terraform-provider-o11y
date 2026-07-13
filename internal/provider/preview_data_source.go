package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/o11y-one/terraform-provider-o11y/internal/client"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
	"github.com/o11y-one/terraform-provider-o11y/internal/idempotency"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

var _ datasource.DataSourceWithConfigure = &alertPreviewDataSource{}

type alertPreviewDataSource struct{ client *client.Client }
type alertPreviewModel struct {
	DefinitionID               types.String `tfsdk:"definition_id"`
	PreviewID                  types.String `tfsdk:"preview_id"`
	PredictedFiringCount       types.Int64  `tfsdk:"predicted_firing_count"`
	PredictedNotificationCount types.Int64  `tfsdk:"predicted_notification_count"`
	ActivationBlockers         types.String `tfsdk:"activation_blockers_json"`
	Result                     types.String `tfsdk:"result_json"`
	CreatedAt                  types.String `tfsdk:"created_at"`
}

func NewAlertPreviewDataSource() datasource.DataSource { return &alertPreviewDataSource{} }
func (d *alertPreviewDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_preview"
}
func (d *alertPreviewDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Runs and returns a deterministic preview for an existing shadow alert.", Attributes: map[string]schema.Attribute{"definition_id": schema.StringAttribute{Required: true}, "preview_id": schema.StringAttribute{Computed: true}, "predicted_firing_count": schema.Int64Attribute{Computed: true}, "predicted_notification_count": schema.Int64Attribute{Computed: true}, "activation_blockers_json": schema.StringAttribute{Computed: true}, "result_json": schema.StringAttribute{Computed: true}, "created_at": schema.StringAttribute{Computed: true}}}
}
func (d *alertPreviewDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}
func (d *alertPreviewDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data alertPreviewModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rpcCtx, cancel := d.client.Context(ctx)
	defer cancel()
	result, err := d.client.Previews.PreviewAlert(rpcCtx, &alertsv1.PreviewAlertRequest{DefinitionId: data.DefinitionID.ValueString(), IdempotencyKey: idempotency.Key(d.client.TenantID(), d.client.OrgID(), "alert_preview", "read", data.DefinitionID.ValueString())})
	if err != nil {
		addRPCError(&resp.Diagnostics, "preview alert", err)
		return
	}
	preview := result.Preview
	if preview == nil {
		resp.Diagnostics.AddError("Invalid preview response", "API returned no preview")
		return
	}
	data.PreviewID = types.StringValue(preview.Id)
	data.PredictedFiringCount = types.Int64Value(int64(preview.PredictedFiringCount))
	data.PredictedNotificationCount = types.Int64Value(int64(preview.PredictedNotificationCount))
	data.ActivationBlockers = protoValueJSON(preview.ActivationBlockers)
	data.Result = jsonFromStruct(preview.Result)
	if preview.CreatedAt != nil {
		data.CreatedAt = types.StringValue(preview.CreatedAt.AsTime().UTC().Format("2006-01-02T15:04:05.999999999Z"))
	} else {
		data.CreatedAt = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
func protoValueJSON(value *structpb.Value) types.String {
	if value == nil {
		return types.StringValue("null")
	}
	raw, _ := protojson.Marshal(value)
	var document any
	_ = json.Unmarshal(raw, &document)
	canonical, _ := json.Marshal(document)
	return types.StringValue(string(canonical))
}
