// Package provider is the Terraform provider for Server Room and Primcast.
//
// It manages what a customer API key may manage (ServCast apikeys/scopes.py): webhooks, uptime and website monitors,
// the port-load alert, reverse DNS, and a server's operating system. It reads services, their features and their
// addresses. It never orders, renews or cancels anything -- a key cannot spend money -- so servers are referenced
// by account id, not created.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/serverroom/servcast-terraform/client"
)

// New returns the provider factory main.go serves.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &servcastProvider{version: version} }
}

type servcastProvider struct {
	version string
}

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
	Brand  types.String `tfsdk:"brand"`
	APIURL types.String `tfsdk:"api_url"`
}

// configured is what every resource and data source gets: the client and the key's own identity.
type configured struct {
	client *client.Client
	me     *client.Whoami
}

func (p *servcastProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "servcast"
	resp.Version = p.version
}

func (p *servcastProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Server Room and Primcast services with an API key (control panel, API & Keys).",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{Optional: true, Sensitive: true,
				Description: "API key. Defaults to the SERVCAST_API_KEY environment variable."},
			"brand": schema.StringAttribute{Optional: true,
				Description: "serverroom.net (default) or primcast.com: the brand the account is with. Defaults to SERVCAST_BRAND."},
			"api_url": schema.StringAttribute{Optional: true,
				Description: "Override the API base URL (testing). Defaults to SERVCAST_API_URL, else the brand's /api."},
		},
	}
}

func pick(value types.String, envName, fallback string) string {
	if !value.IsNull() && !value.IsUnknown() && value.ValueString() != "" {
		return value.ValueString()
	}
	if v := os.Getenv(envName); v != "" {
		return v
	}
	return fallback
}

func (p *servcastProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key := pick(cfg.APIKey, "SERVCAST_API_KEY", "")
	if key == "" {
		resp.Diagnostics.AddError("No API key", "Set api_key in the provider block or SERVCAST_API_KEY. Create a key "+
			"under API & Keys in your control panel.")
		return
	}
	c := client.New(pick(cfg.Brand, "SERVCAST_BRAND", "serverroom.net"), key)
	if base := pick(cfg.APIURL, "SERVCAST_API_URL", ""); base != "" {
		c.BaseURL = base
	}
	client.Version = p.version
	c.Agent = "terraform-provider-servcast/" + p.version
	me, err := c.Whoami()
	if err != nil {
		resp.Diagnostics.AddError("The API key was not accepted", err.Error())
		return
	}
	data := &configured{client: c, me: me}
	resp.DataSourceData = data
	resp.ResourceData = data
}

func (p *servcastProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newWebhookResource,
		newMonitorResource,
		newPortAlertResource,
		newReverseDNSResource,
		newServerOSResource,
	}
}

func (p *servcastProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newAccountDataSource,
		newServersDataSource,
		newServerDataSource,
	}
}
