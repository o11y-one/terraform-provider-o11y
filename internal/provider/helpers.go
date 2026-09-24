package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type canonicalJSONPlanModifier struct {
	intDefaults map[string]int64
}

func (m canonicalJSONPlanModifier) Description(context.Context) string {
	return "Canonicalizes a JSON object and applies documented integer defaults."
}

func (m canonicalJSONPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m canonicalJSONPlanModifier) PlanModifyString(
	_ context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	value := req.PlanValue
	var err error
	for key, defaultValue := range m.intDefaults {
		value, err = canonicalJSONWithIntDefault(value, key, defaultValue)
		if err != nil {
			resp.Diagnostics.AddError("Invalid JSON", err.Error())
			return
		}
	}
	if len(m.intDefaults) == 0 {
		value, err = canonicalJSONString(value)
		if err != nil {
			resp.Diagnostics.AddError("Invalid JSON", err.Error())
			return
		}
	}
	resp.PlanValue = value
}

func protoFromJSON(value types.String, result proto.Message) error {
	if result == nil {
		return fmt.Errorf("protobuf destination must not be nil")
	}
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return nil
	}
	return protojson.UnmarshalOptions{DiscardUnknown: false}.Unmarshal([]byte(value.ValueString()), result)
}

func jsonFromProto(value proto.Message) types.String {
	if value == nil {
		return types.StringValue("{}")
	}
	raw, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(value)
	var document any
	_ = json.Unmarshal(raw, &document)
	canonical, _ := json.Marshal(document)
	return types.StringValue(string(canonical))
}

func jsonFromProtoPreserving(configured types.String, value proto.Message) types.String {
	if value != nil && knownNonEmpty(configured) {
		candidate := value.ProtoReflect().New().Interface()
		if err := protoFromJSON(configured, candidate); err == nil {
			if scope, ok := candidate.(*alertsv1.AlertScopeV1); ok {
				echoScopeFilterValues(scope)
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

// The server stores one value list per filter and echoes it twice: its string
// members as values and every member as typed_values.
// ponytail: a uint_value that fits int64 echoes as int_value and still diffs; map it if anyone authors uint_value.
func echoScopeFilterValues(scope *alertsv1.AlertScopeV1) {
	for _, filter := range scope.TelemetryAttributeFilters {
		if len(filter.TypedValues) == 0 {
			for _, value := range filter.Values {
				filter.TypedValues = append(filter.TypedValues, &alertsv1.AlertScalarValueV1{Value: &alertsv1.AlertScalarValueV1_StringValue{StringValue: value}})
			}
			continue
		}
		filter.Values = nil
		for _, value := range filter.TypedValues {
			if text, ok := value.Value.(*alertsv1.AlertScalarValueV1_StringValue); ok {
				filter.Values = append(filter.Values, text.StringValue)
			}
		}
	}
}

func stringMapFromJSON(value types.String) (map[string]string, error) {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return map[string]string{}, nil
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(value.ValueString()), &result); err != nil {
		return nil, err
	}
	if result == nil {
		result = map[string]string{}
	}
	return result, nil
}

func jsonFromStringMap(value map[string]string) types.String {
	if value == nil {
		value = map[string]string{}
	}
	raw, _ := json.Marshal(value)
	return types.StringValue(string(raw))
}

func canonicalJSONString(value types.String) (types.String, error) {
	if value.IsNull() || value.IsUnknown() {
		return value, nil
	}
	var document any
	if err := json.Unmarshal([]byte(value.ValueString()), &document); err != nil {
		return value, err
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		return value, err
	}
	return types.StringValue(string(canonical)), nil
}

func canonicalJSONWithIntDefault(value types.String, key string, defaultValue int64) (types.String, error) {
	if value.IsNull() || value.IsUnknown() {
		return value, nil
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(value.ValueString()), &document); err != nil {
		return value, err
	}
	if _, ok := document[key]; !ok {
		document[key] = defaultValue
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		return value, err
	}
	return types.StringValue(string(canonical)), nil
}

func addRPCError(diags *diag.Diagnostics, operation string, err error) {
	diags.AddError("O11y.one API error", fmt.Sprintf("%s: %v", operation, err))
}

func validateTimeRange(diags *diag.Diagnostics, startsAt, endsAt types.String) {
	if startsAt.IsNull() || startsAt.IsUnknown() || endsAt.IsNull() || endsAt.IsUnknown() {
		return
	}
	start, startErr := time.Parse(time.RFC3339, startsAt.ValueString())
	end, endErr := time.Parse(time.RFC3339, endsAt.ValueString())
	if startErr != nil {
		diags.AddError("Invalid starts_at", "starts_at must be RFC3339: "+startErr.Error())
	}
	if endErr != nil {
		diags.AddError("Invalid ends_at", "ends_at must be RFC3339: "+endErr.Error())
	}
	if startErr == nil && endErr == nil && !end.After(start) {
		diags.AddError("Invalid time range", "ends_at must be after starts_at")
	}
}

func timestampString(value *timestamppb.Timestamp) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(value.AsTime().UTC().Format(time.RFC3339Nano))
}
