package provider

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/o11y-one/terraform-provider-o11y/internal/client"
	validationutil "github.com/o11y-one/terraform-provider-o11y/internal/validation"
)

var _ provider.Provider = &o11yProvider{}
var _ provider.ProviderWithValidateConfig = &o11yProvider{}

type o11yProvider struct {
	version string
	mu      sync.Mutex
	client  *client.Client
}

type providerModel struct {
	Endpoint           types.String `tfsdk:"endpoint"`
	Token              types.String `tfsdk:"token"`
	TenantID           types.String `tfsdk:"tenant_id"`
	OrgID              types.String `tfsdk:"org_id"`
	InsecureSkipVerify types.Bool   `tfsdk:"insecure_skip_verify"`
	ConnectTimeout     types.Int64  `tfsdk:"connect_timeout_seconds"`
	RequestTimeout     types.Int64  `tfsdk:"request_timeout_seconds"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &o11yProvider{version: version} }
}

func (p *o11yProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "o11y"
	resp.Version = p.version
}

func (p *o11yProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{
		Description: "Manage O11y.one alert contacts, notification groups, destinations, policies, maintenance windows, silences, shadow alert definitions, and previews through the authenticated gRPC API.",
		Attributes: map[string]providerschema.Attribute{
			"endpoint":                providerschema.StringAttribute{Optional: true, Description: "O11y.one gRPC API origin, https://grpc.o11y.one. Defaults to O11Y_ENDPOINT."},
			"token":                   providerschema.StringAttribute{Optional: true, Sensitive: true, Description: "O11y.one bearer token. Defaults to O11Y_TOKEN."},
			"tenant_id":               providerschema.StringAttribute{Optional: true, Description: "Tenant UUID. Defaults to O11Y_TENANT_ID."},
			"org_id":                  providerschema.StringAttribute{Optional: true, Description: "Organization UUID. Defaults to O11Y_ORG_ID."},
			"insecure_skip_verify":    providerschema.BoolAttribute{Optional: true, Description: "Allow HTTP or skip TLS verification. Defaults to false."},
			"connect_timeout_seconds": providerschema.Int64Attribute{Optional: true, Description: "Connection timeout in seconds. Defaults to 10."},
			"request_timeout_seconds": providerschema.Int64Attribute{Optional: true, Description: "Per-RPC timeout in seconds. Defaults to 30."},
		},
	}
}

func (p *o11yProvider) ValidateConfig(ctx context.Context, req provider.ValidateConfigRequest, resp *provider.ValidateConfigResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	endpoint := validateValue(data.Endpoint, "O11Y_ENDPOINT")
	allowInsecure := !data.InsecureSkipVerify.IsNull() && data.InsecureSkipVerify.ValueBool()
	if !data.Endpoint.IsUnknown() && endpoint != "" {
		if err := validationutil.Endpoint(endpoint, allowInsecure); err != nil {
			resp.Diagnostics.AddAttributeError(pathRoot("endpoint"), "Invalid endpoint", err.Error())
		}
	}
	for name, item := range map[string]struct {
		value types.String
		env   string
	}{"endpoint": {data.Endpoint, "O11Y_ENDPOINT"}, "token": {data.Token, "O11Y_TOKEN"}, "tenant_id": {data.TenantID, "O11Y_TENANT_ID"}, "org_id": {data.OrgID, "O11Y_ORG_ID"}} {
		if !item.value.IsUnknown() && validateValue(item.value, item.env) == "" {
			resp.Diagnostics.AddError("Missing provider configuration", fmt.Sprintf("%s must be configured directly or through its O11Y environment variable", name))
		}
	}
	for name, item := range map[string]struct {
		value types.String
		env   string
	}{"tenant_id": {data.TenantID, "O11Y_TENANT_ID"}, "org_id": {data.OrgID, "O11Y_ORG_ID"}} {
		value := validateValue(item.value, item.env)
		if !item.value.IsUnknown() && value != "" {
			if err := validationutil.UUID(value); err != nil {
				resp.Diagnostics.AddError("Invalid provider scope", name+" "+err.Error())
			}
		}
	}
	validatePositive(&resp.Diagnostics, "connect_timeout_seconds", data.ConnectTimeout)
	validatePositive(&resp.Diagnostics, "request_timeout_seconds", data.RequestTimeout)
}

func (p *o11yProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	values := map[string]string{"endpoint": valueOrEnv(data.Endpoint, "O11Y_ENDPOINT"), "token": valueOrEnv(data.Token, "O11Y_TOKEN"), "tenant_id": valueOrEnv(data.TenantID, "O11Y_TENANT_ID"), "org_id": valueOrEnv(data.OrgID, "O11Y_ORG_ID")}
	for name, value := range values {
		if value == "" {
			resp.Diagnostics.AddError("Missing provider configuration", name+" must be known and non-empty during provider configuration")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	config := client.Config{Endpoint: values["endpoint"], Token: values["token"], TenantID: values["tenant_id"], OrgID: values["org_id"], InsecureSkipVerify: boolValue(data.InsecureSkipVerify), ConnectTimeout: secondsValue(data.ConnectTimeout, 10), RequestTimeout: secondsValue(data.RequestTimeout, 30)}
	c, err := client.New(ctx, config)
	if err != nil {
		resp.Diagnostics.AddError("Unable to configure O11y.one client", err.Error())
		return
	}
	p.mu.Lock()
	previous := p.client
	p.client = c
	p.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	resp.DataSourceData, resp.ResourceData = c, c
}

func (p *o11yProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewContactResource, NewNotificationGroupResource, NewDestinationResource, NewNotificationPolicyResource, NewMaintenanceWindowResource, NewSilenceResource, NewAgentQualityAlertResource, NewCostAlertResource, NewSLOAlertResource, NewSymptomAlertResource}
}
func (p *o11yProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewAlertPreviewDataSource}
}

func valueOrEnv(value types.String, env string) string {
	if value.IsUnknown() {
		return ""
	}
	if !value.IsNull() {
		return value.ValueString()
	}
	return os.Getenv(env)
}
func validateValue(value types.String, env string) string {
	if value.IsUnknown() {
		return ""
	}
	return valueOrEnv(value, env)
}
func (p *o11yProvider) close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil {
		return nil
	}
	err := p.client.Close()
	p.client = nil
	return err
}
func boolValue(value types.Bool) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueBool()
}
func secondsValue(value types.Int64, fallback int64) time.Duration {
	if value.IsNull() || value.IsUnknown() {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(value.ValueInt64()) * time.Second
}
func validatePositive(diags *diag.Diagnostics, name string, value types.Int64) {
	if !value.IsNull() && !value.IsUnknown() && value.ValueInt64() <= 0 {
		diags.AddError("Invalid timeout", name+" must be greater than zero")
	}
}

// Keep framework path construction local to avoid validators that can accidentally expose sensitive values.
func pathRoot(name string) path.Path { return path.Root(name) }
