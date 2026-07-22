package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/o11y-one/terraform-provider-o11y/internal/client"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
	"github.com/o11y-one/terraform-provider-o11y/internal/idempotency"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ datasource.DataSourceWithConfigure = &alertPreviewDataSource{}

type alertPreviewDataSource struct{ client *client.Client }
type alertPreviewModel struct {
	DefinitionID               types.String `tfsdk:"definition_id"`
	RevisionID                 types.String `tfsdk:"revision_id"`
	RangeStart                 types.String `tfsdk:"range_start"`
	RangeEnd                   types.String `tfsdk:"range_end"`
	EvidenceLimit              types.Int64  `tfsdk:"evidence_limit"`
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
	resp.Schema = schema.Schema{Description: "Runs and returns a deterministic bounded preview for an existing Observe alert revision.", Attributes: map[string]schema.Attribute{
		"definition_id": schema.StringAttribute{Required: true}, "revision_id": schema.StringAttribute{Optional: true, Description: "Exact immutable alert revision. Omit to preview the current revision."},
		"range_start": schema.StringAttribute{Optional: true, Description: "Optional RFC3339 preview range start; configure together with range_end."}, "range_end": schema.StringAttribute{Optional: true, Description: "Optional RFC3339 preview range end; configure together with range_start."},
		"evidence_limit": schema.Int64Attribute{Optional: true, Description: "Optional bounded evidence sample limit."}, "preview_id": schema.StringAttribute{Computed: true},
		"predicted_firing_count": schema.Int64Attribute{Computed: true}, "predicted_notification_count": schema.Int64Attribute{Computed: true}, "activation_blockers_json": schema.StringAttribute{Computed: true}, "result_json": schema.StringAttribute{Computed: true}, "created_at": schema.StringAttribute{Computed: true},
	}}
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
	message := &alertsv1.PreviewAlertRequest{DefinitionId: data.DefinitionID.ValueString(), RevisionId: stringValue(data.RevisionID)}
	if knownNonEmpty(data.RangeStart) || knownNonEmpty(data.RangeEnd) {
		if !knownNonEmpty(data.RangeStart) || !knownNonEmpty(data.RangeEnd) {
			resp.Diagnostics.AddError("Invalid preview range", "range_start and range_end must be configured together")
			return
		}
		start, startErr := time.Parse(time.RFC3339, data.RangeStart.ValueString())
		end, endErr := time.Parse(time.RFC3339, data.RangeEnd.ValueString())
		if startErr != nil || endErr != nil || !end.After(start) {
			resp.Diagnostics.AddError("Invalid preview range", "range_start and range_end must be valid RFC3339 timestamps and range_end must be later")
			return
		}
		message.RangeStart, message.RangeEnd = timestamppb.New(start), timestamppb.New(end)
	}
	if !data.EvidenceLimit.IsNull() && !data.EvidenceLimit.IsUnknown() {
		if data.EvidenceLimit.ValueInt64() <= 0 || data.EvidenceLimit.ValueInt64() > int64(^uint32(0)) {
			resp.Diagnostics.AddError("Invalid evidence limit", "evidence_limit must be greater than zero and fit in uint32")
			return
		}
		message.EvidenceLimit = proto.Uint32(uint32(data.EvidenceLimit.ValueInt64()))
	}
	payload, _ := json.Marshal(message)
	message.IdempotencyKey = idempotency.Key(d.client.TenantID(), d.client.OrgID(), "alert_preview", "read", data.DefinitionID.ValueString(), data.RevisionID.ValueString(), string(payload))
	result, err := d.client.Previews.PreviewAlert(rpcCtx, message)
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
	blockers, _ := json.Marshal(preview.ActivationBlockers)
	data.ActivationBlockers = types.StringValue(string(blockers))
	data.Result = jsonFromProto(preview.Result)
	if preview.CreatedAt != nil {
		data.CreatedAt = types.StringValue(preview.CreatedAt.AsTime().UTC().Format("2006-01-02T15:04:05.999999999Z"))
	} else {
		data.CreatedAt = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
