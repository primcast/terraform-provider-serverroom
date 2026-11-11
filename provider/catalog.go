package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/primcast/terraform-provider-serverroom/client"
)

// ------------------------------------------------------------------------------------------------- serverroom_stock

type stockDataSource struct{ p *configured }

func newStockDataSource() datasource.DataSource { return &stockDataSource{} }

type stockServerModel struct {
	Name      types.String  `tfsdk:"name"`
	ServerID  types.Int64   `tfsdk:"server_id"`
	Plan      types.String  `tfsdk:"plan"`
	CPU       types.String  `tfsdk:"cpu"`
	RAMGB     types.Float64 `tfsdk:"ram_gb"`
	Disk      types.String  `tfsdk:"disk"`
	GPU       types.String  `tfsdk:"gpu"`
	Location  types.String  `tfsdk:"location"`
	OS        types.String  `tfsdk:"os"`
	Traffic   types.String  `tfsdk:"traffic"`
	Price     types.Float64 `tfsdk:"price"`
	ListPrice types.Float64 `tfsdk:"list_price"`
	OrderURL  types.String  `tfsdk:"order_url"`
}

type stockModel struct {
	Location types.List         `tfsdk:"location"`
	Plan     types.List         `tfsdk:"plan"`
	GPU      types.List         `tfsdk:"gpu"`
	MinRAMGB types.Int64        `tfsdk:"min_ram_gb"`
	MaxPrice types.Float64      `tfsdk:"max_price"`
	Limit    types.Int64        `tfsdk:"limit"`
	Servers  []stockServerModel `tfsdk:"servers"`
}

func (d *stockDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stock"
}

func (d *stockDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Servers in stock now, ready to deliver, cheapest first, each with its order link. Read-only: an API " +
			"key cannot order or pay, so a server is ordered by opening its order_url.",
		Attributes: map[string]schema.Attribute{
			"location":   schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Cities or regions; \"Amsterdam\" matches \"Amsterdam, EU\"."},
			"plan":       schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Plan codes, e.g. [\"M710x\", \"D104\"]."},
			"gpu":        schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "GPU names, matched loosely (\"RTX\")."},
			"min_ram_gb": schema.Int64Attribute{Optional: true, Description: "At least this much memory."},
			"max_price":  schema.Float64Attribute{Optional: true, Description: "At most this first-month price, in USD."},
			"limit":      schema.Int64Attribute{Optional: true, Description: "How many to return, up to 40 (default 20)."},
			"servers": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name":       schema.StringAttribute{Computed: true, Description: "The machine's name, e.g. s924."},
					"server_id":  schema.Int64Attribute{Computed: true},
					"plan":       schema.StringAttribute{Computed: true},
					"cpu":        schema.StringAttribute{Computed: true},
					"ram_gb":     schema.Float64Attribute{Computed: true},
					"disk":       schema.StringAttribute{Computed: true},
					"gpu":        schema.StringAttribute{Computed: true},
					"location":   schema.StringAttribute{Computed: true},
					"os":         schema.StringAttribute{Computed: true, Description: "What is installed now; chosen again at order."},
					"traffic":    schema.StringAttribute{Computed: true},
					"price":      schema.Float64Attribute{Computed: true, Description: "First-month price in USD."},
					"list_price": schema.Float64Attribute{Computed: true, Description: "Monthly price from the second month on, when the first month is discounted; else equal to price."},
					"order_url":  schema.StringAttribute{Computed: true, Description: "The order page, prefilled with this machine."},
				}}},
		},
	}
}

func (d *stockDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.p = configureData(req, resp)
}

func listStrings(ctx context.Context, l types.List) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var out []string
	l.ElementsAs(ctx, &out, false)
	return out
}

func (d *stockDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state stockModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	f := client.StockFilter{Location: listStrings(ctx, state.Location), Type: listStrings(ctx, state.Plan),
		GPU: listStrings(ctx, state.GPU), MinRAM: int(state.MinRAMGB.ValueInt64()), MaxPrice: state.MaxPrice.ValueFloat64(),
		Limit: 20}
	if !state.Limit.IsNull() && state.Limit.ValueInt64() > 0 {
		f.Limit = int(state.Limit.ValueInt64())
	}
	list, err := d.p.client.Stock(f)
	if err != nil {
		resp.Diagnostics.AddError("Reading the stock failed", err.Error())
		return
	}
	state.Servers = []stockServerModel{}
	for _, s := range list {
		list := s.ListPrice
		if list == 0 {
			list = s.Price
		}
		state.Servers = append(state.Servers, stockServerModel{
			Name: types.StringValue(s.Name), ServerID: types.Int64Value(int64(s.ServerID)), Plan: types.StringValue(s.Type),
			CPU: types.StringValue(s.CPU), RAMGB: types.Float64Value(s.RAM), Disk: types.StringValue(s.Disk),
			GPU: types.StringValue(s.GPU), Location: types.StringValue(s.Location), OS: types.StringValue(s.OS),
			Traffic: types.StringValue(s.Traffic), Price: types.Float64Value(s.Price), ListPrice: types.Float64Value(list),
			OrderURL: types.StringValue(s.OrderURL)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ------------------------------------------------------------------------------------------------- serverroom_plan_os

type planOSDataSource struct{ p *configured }

func newPlanOSDataSource() datasource.DataSource { return &planOSDataSource{} }

type planOSModel struct {
	Plan  types.String `tfsdk:"plan"`
	Names types.List   `tfsdk:"names"`
}

func (d *planOSDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plan_os"
}

func (d *planOSDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The operating systems a plan installs, by the exact names serverroom_server_os takes.",
		Attributes: map[string]schema.Attribute{
			"plan":  schema.StringAttribute{Required: true, Description: "Plan code, e.g. \"M710x\" (the plan attribute of serverroom_servers)."},
			"names": schema.ListAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *planOSDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.p = configureData(req, resp)
}

func (d *planOSDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state planOSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	names, err := d.p.client.PlanOS(state.Plan.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the plan failed", err.Error())
		return
	}
	var diags = resp.Diagnostics
	state.Names, diags = types.ListValueFrom(ctx, types.StringType, names)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
