// Package provider is the serverroom Terraform provider: one provider for every brand, the brand read off the API key.
//
// It manages what a customer API key may manage: webhooks, uptime and website monitors, the port-load alert, reverse
// DNS, and a server's operating system. It reads services, their features and addresses, the servers in stock and the
// operating systems a plan installs. It never orders, renews or cancels anything -- a key cannot spend money -- so
// servers are referenced by their service id, not created.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/primcast/terraform-provider-serverroom/client"
)

// New returns the provider factory main.go serves.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &serverroomProvider{version: version} }
}

type serverroomProvider struct {
	version string
}

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
	APIURL types.String `tfsdk:"api_url"`
}

// configured is what every resource and data source gets: the client and the key's own identity.
type configured struct {
	client *client.Client
	me     *client.Whoami
}

func (p *serverroomProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "serverroom"
	resp.Version = p.version
}

func (p *serverroomProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage your dedicated servers with an API key. Create the key in your control panel under API & Keys; " +
			"the key decides which account, and which website's API, the provider talks to.",
		MarkdownDescription: "Manage your dedicated servers with an API key. Create the key in your control panel under " +
			"**API & Keys**; the key decides which account, and which website's API, the provider talks to.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{Optional: true, Sensitive: true,
				Description: "API key. Defaults to the SERVERROOM_API_KEY environment variable."},
			"api_url": schema.StringAttribute{Optional: true,
				Description: "API base URL. Defaults to SERVERROOM_API_URL, else the API of the website the key was made on. " +
					"Set it to https://<your control panel website>/api if the provider asks for it."},
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

func (p *serverroomProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key := pick(cfg.APIKey, "SERVERROOM_API_KEY", "")
	if key == "" {
		resp.Diagnostics.AddError("No API key", "Set api_key in the provider block or SERVERROOM_API_KEY. Create a key "+
			"under API & Keys in your control panel.")
		return
	}
	client.Version = p.version
	apiURL := pick(cfg.APIURL, "SERVERROOM_API_URL", "")
	if err := client.CheckKey(apiURL, key); err != nil {
		resp.Diagnostics.AddError("Set api_url", "Set api_url in the provider block, or SERVERROOM_API_URL, to "+
			"https://<your control panel website>/api: this key's website is not known to this provider version.")
		return
	}
	c := client.New(apiURL, key)
	c.Agent = "terraform-provider-serverroom/" + p.version
	me, err := c.Whoami()
	if err != nil {
		resp.Diagnostics.AddError("The API key was not accepted", err.Error())
		return
	}
	data := &configured{client: c, me: me}
	resp.DataSourceData = data
	resp.ResourceData = data
}

func (p *serverroomProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newWebhookResource,
		newMonitorResource,
		newPortAlertResource,
		newReverseDNSResource,
		newServerOSResource,
	}
}

func (p *serverroomProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newAccountDataSource,
		newServersDataSource,
		newServerDataSource,
		newStockDataSource,
		newPlanOSDataSource,
	}
}
