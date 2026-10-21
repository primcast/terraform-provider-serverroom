package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/serverroom/servcast-terraform/client"
)

func configureData(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *configured {
	if req.ProviderData == nil {
		return nil
	}
	data, ok := req.ProviderData.(*configured)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("%T", req.ProviderData))
		return nil
	}
	return data
}

// ------------------------------------------------------------------------------------------------- servcast_account

type accountDataSource struct{ p *configured }

func newAccountDataSource() datasource.DataSource { return &accountDataSource{} }

type accountModel struct {
	UID      types.Int64  `tfsdk:"uid"`
	Brand    types.String `tfsdk:"brand"`
	Accounts types.List   `tfsdk:"account_ids"`
	Scopes   types.List   `tfsdk:"scopes"`
}

func (d *accountDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (d *accountDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The customer the API key belongs to, its services and the key's permissions.",
		Attributes: map[string]schema.Attribute{
			"uid":         schema.Int64Attribute{Computed: true},
			"brand":       schema.StringAttribute{Computed: true},
			"account_ids": schema.ListAttribute{Computed: true, ElementType: types.Int64Type},
			"scopes":      schema.ListAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *accountDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.p = configureData(req, resp)
}

func (d *accountDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	me, err := d.p.client.Whoami()
	if err != nil {
		resp.Diagnostics.AddError("Reading the account failed", err.Error())
		return
	}
	ids := make([]int64, 0, len(me.Accounts))
	for _, a := range me.Accounts {
		ids = append(ids, int64(a))
	}
	accounts, diags := types.ListValueFrom(ctx, types.Int64Type, ids)
	resp.Diagnostics.Append(diags...)
	scopes, diags := types.ListValueFrom(ctx, types.StringType, me.Key.Scopes)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, accountModel{UID: types.Int64Value(int64(me.UID)),
		Brand: types.StringValue(me.Brand), Accounts: accounts, Scopes: scopes})...)
}

// ------------------------------------------------------------------------------------------------- servcast_servers

type serversDataSource struct{ p *configured }

func newServersDataSource() datasource.DataSource { return &serversDataSource{} }

type serviceModel struct {
	AccountID types.Int64  `tfsdk:"account_id"`
	Plan      types.String `tfsdk:"plan"`
	Name      types.String `tfsdk:"name"`
	Status    types.Int64  `tfsdk:"status"`
}

type serversModel struct {
	Services []serviceModel `tfsdk:"services"`
}

func (d *serversDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_servers"
}

func (d *serversDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every service on the account: account id, plan code, name and status.",
		Attributes: map[string]schema.Attribute{
			"services": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"account_id": schema.Int64Attribute{Computed: true},
					"plan":       schema.StringAttribute{Computed: true},
					"name":       schema.StringAttribute{Computed: true},
					"status":     schema.Int64Attribute{Computed: true},
				}}},
		},
	}
}

func (d *serversDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.p = configureData(req, resp)
}

func (d *serversDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	list, err := d.p.client.Services(d.p.me.UID)
	if err != nil {
		resp.Diagnostics.AddError("Reading services failed", err.Error())
		return
	}
	out := serversModel{Services: []serviceModel{}}
	for _, s := range list {
		out.Services = append(out.Services, serviceModel{AccountID: types.Int64Value(int64(s.AccountID)),
			Plan: types.StringValue(s.AccountType), Name: types.StringValue(s.AccountName),
			Status: types.Int64Value(int64(s.Status))})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, out)...)
}

// ------------------------------------------------------------------------------------------------- servcast_server

type serverDataSource struct{ p *configured }

func newServerDataSource() datasource.DataSource { return &serverDataSource{} }

type serverModel struct {
	AccountID types.Int64  `tfsdk:"account_id"`
	ServerID  types.Int64  `tfsdk:"server_id"`
	OS        types.String `tfsdk:"os"`
	Location  types.String `tfsdk:"location"`
	PortSpeed types.Int64  `tfsdk:"port_speed_mbps"`
	Features  types.Map    `tfsdk:"features"`
	IPs       types.List   `tfsdk:"ipv4"`
}

func (d *serverDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server"
}

func (d *serverDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "One dedicated server: its machine, operating system, location, port speed, features and addresses.",
		Attributes: map[string]schema.Attribute{
			"account_id":      schema.Int64Attribute{Required: true, Description: "The service's account id."},
			"server_id":       schema.Int64Attribute{Computed: true},
			"os":              schema.StringAttribute{Computed: true},
			"location":        schema.StringAttribute{Computed: true},
			"port_speed_mbps": schema.Int64Attribute{Computed: true},
			"features":        schema.MapAttribute{Computed: true, ElementType: types.StringType},
			"ipv4":            schema.ListAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *serverDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.p = configureData(req, resp)
}

func (d *serverDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state serverModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	accountID := int(state.AccountID.ValueInt64())
	features, err := d.p.client.Features(accountID)
	if err != nil {
		resp.Diagnostics.AddError("Reading the server failed", err.Error())
		return
	}
	fm := client.FeatureMap(features)
	sid, _ := strconv.Atoi(fm["ServerID"])
	speed, _ := strconv.Atoi(fm["PortSpeed"])
	addresses := []string{}
	if sid > 0 {
		ips, err := d.p.client.IPs(accountID, sid)
		if err != nil {
			resp.Diagnostics.AddError("Reading the server's addresses failed", err.Error())
			return
		}
		for _, ip := range ips {
			addresses = append(addresses, ip.IP)
		}
	}
	state.ServerID = types.Int64Value(int64(sid))
	state.OS = types.StringValue(fm["OS"])
	state.Location = types.StringValue(fm["Location"])
	state.PortSpeed = types.Int64Value(int64(speed))
	var diags = resp.Diagnostics
	state.Features, diags = types.MapValueFrom(ctx, types.StringType, fm)
	resp.Diagnostics.Append(diags...)
	state.IPs, diags = types.ListValueFrom(ctx, types.StringType, addresses)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
