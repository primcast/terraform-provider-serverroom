package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/primcast/terraform-provider-serverroom/client"
)

func configureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *configured {
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

func stringList(ctx context.Context, values []string) (types.List, error) {
	list, diags := types.ListValueFrom(ctx, types.StringType, values)
	if diags.HasError() {
		return list, fmt.Errorf("%v", diags)
	}
	return list, nil
}

// ================================================================================================= serverroom_webhook

type webhookResource struct{ p *configured }

func newWebhookResource() resource.Resource { return &webhookResource{} }

type webhookModel struct {
	ID          types.String `tfsdk:"id"`
	URL         types.String `tfsdk:"url"`
	Events      types.List   `tfsdk:"events"`
	Description types.String `tfsdk:"description"`
	Active      types.Bool   `tfsdk:"active"`
	Secret      types.String `tfsdk:"secret"`
}

func (r *webhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *webhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A webhook endpoint: what happens on the account is POSTed to it, signed with its secret.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"url":         schema.StringAttribute{Required: true, Description: "The https:// endpoint."},
			"events":      schema.ListAttribute{Required: true, ElementType: types.StringType, Description: "Event types, or [\"*\"] for all."},
			"description": schema.StringAttribute{Optional: true, Computed: true},
			"active":      schema.BoolAttribute{Optional: true, Computed: true},
			"secret": schema.StringAttribute{Computed: true, Sensitive: true,
				Description:   "The signing secret, known only for an endpoint this configuration created.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *webhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.p = configureResource(req, resp)
}

func (r *webhookResource) toClient(ctx context.Context, m webhookModel) (client.Webhook, error) {
	var events []string
	if diags := m.Events.ElementsAs(ctx, &events, false); diags.HasError() {
		return client.Webhook{}, fmt.Errorf("%v", diags)
	}
	w := client.Webhook{URL: m.URL.ValueString(), Events: events, Description: m.Description.ValueString()}
	if !m.Active.IsNull() && !m.Active.IsUnknown() {
		v := m.Active.ValueBool()
		w.Active = &v
	}
	return w, nil
}

func (r *webhookResource) fill(ctx context.Context, m *webhookModel, w *client.Webhook) error {
	m.ID = types.StringValue(w.ID)
	m.URL = types.StringValue(w.URL)
	m.Description = types.StringValue(w.Description)
	m.Active = types.BoolValue(w.Active == nil || *w.Active)
	list, err := stringList(ctx, w.Events)
	m.Events = list
	return err
}

func (r *webhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.toClient(ctx, plan)
	if err == nil {
		var made *client.Webhook
		made, err = r.p.client.CreateWebhook(w)
		if err == nil {
			secret := made.Secret
			if err = r.fill(ctx, &plan, made); err == nil {
				plan.Secret = types.StringValue(secret)
				if w.Active != nil && !*w.Active {
					err = r.p.client.UpdateWebhook(made.ID, client.Webhook{URL: w.URL, Events: w.Events, Active: w.Active})
					plan.Active = types.BoolValue(false)
				}
			}
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Creating the webhook failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	w, err := r.p.client.Webhook(state.ID.ValueString())
	if client.NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err == nil {
		err = r.fill(ctx, &state, w)
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading the webhook failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *webhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state webhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	w, err := r.toClient(ctx, plan)
	if err == nil {
		err = r.p.client.UpdateWebhook(state.ID.ValueString(), w)
	}
	if err == nil {
		var fresh *client.Webhook
		if fresh, err = r.p.client.Webhook(state.ID.ValueString()); err == nil {
			err = r.fill(ctx, &plan, fresh)
			plan.Secret = state.Secret
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating the webhook failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if err := r.p.client.DeleteWebhook(state.ID.ValueString()); err != nil && !client.NotFound(err) {
		resp.Diagnostics.AddError("Deleting the webhook failed", err.Error())
	}
}

func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("secret"), "")...)
}

// ================================================================================================= serverroom_monitor

type monitorResource struct{ p *configured }

func newMonitorResource() resource.Resource { return &monitorResource{} }

type monitorModel struct {
	ID        types.String `tfsdk:"id"`
	AccountID types.Int64  `tfsdk:"account_id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Target    types.String `tfsdk:"target"`
	Port      types.Int64  `tfsdk:"port"`
	Interval  types.Int64  `tfsdk:"interval"`
	Timeout   types.Int64  `tfsdk:"timeout"`
	Confirm   types.Int64  `tfsdk:"confirm"`
	Keyword   types.String `tfsdk:"keyword"`
	Paused    types.Bool   `tfsdk:"paused"`
	Status    types.String `tfsdk:"status"`
}

func (r *monitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor"
}

func (r *monitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A monitor of your own: a website, a TCP port or a ping target, checked as often as the " +
			"service's support tier allows. Alerts go to the channels set on the Alerts page.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"account_id": schema.Int64Attribute{Required: true, Description: "The service whose tier the monitor counts against.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"name":     schema.StringAttribute{Required: true},
			"type":     schema.StringAttribute{Required: true, Description: "http, tcp or ping."},
			"target":   schema.StringAttribute{Required: true, Description: "A URL for http, a host name or IP otherwise."},
			"port":     schema.Int64Attribute{Optional: true, Description: "The port, for tcp."},
			"interval": schema.Int64Attribute{Optional: true, Computed: true, Description: "Seconds between checks."},
			"timeout":  schema.Int64Attribute{Optional: true, Computed: true},
			"confirm":  schema.Int64Attribute{Optional: true, Computed: true, Description: "Failed checks in a row before an alert."},
			"keyword":  schema.StringAttribute{Optional: true, Description: "A word the page must contain (http)."},
			"paused":   schema.BoolAttribute{Optional: true, Computed: true},
			"status":   schema.StringAttribute{Computed: true, Description: "up, down, pending or paused."},
		},
	}
}

func (r *monitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.p = configureResource(req, resp)
}

func intOr(v types.Int64) int {
	if v.IsNull() || v.IsUnknown() {
		return 0
	}
	return int(v.ValueInt64())
}

func (r *monitorResource) toClient(m monitorModel) client.Monitor {
	out := client.Monitor{AccountID: intOr(m.AccountID), Name: m.Name.ValueString(), Type: m.Type.ValueString(),
		Target: m.Target.ValueString(), Port: intOr(m.Port), Interval: intOr(m.Interval), Timeout: intOr(m.Timeout),
		Confirm: intOr(m.Confirm), Keyword: m.Keyword.ValueString()}
	if !m.Paused.IsNull() && !m.Paused.IsUnknown() {
		out.Paused = m.Paused.ValueBool()
	}
	return out
}

func (r *monitorResource) fill(m *monitorModel, mon *client.Monitor) {
	m.ID = types.StringValue(mon.ID)
	if mon.Account != 0 {
		m.AccountID = types.Int64Value(int64(mon.Account))
	}
	m.Name = types.StringValue(mon.Name)
	m.Type = types.StringValue(mon.Type)
	m.Target = types.StringValue(mon.Target)
	if mon.Port != 0 {
		m.Port = types.Int64Value(int64(mon.Port))
	}
	m.Interval = types.Int64Value(int64(mon.Interval))
	m.Timeout = types.Int64Value(int64(mon.Timeout))
	m.Confirm = types.Int64Value(int64(mon.Confirm))
	if mon.Keyword != "" {
		m.Keyword = types.StringValue(mon.Keyword)
	}
	m.Paused = types.BoolValue(mon.Paused)
	m.Status = types.StringValue(mon.State.Status)
}

func (r *monitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	made, err := r.p.client.CreateMonitor(r.toClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating the monitor failed", err.Error())
		return
	}
	r.fill(&plan, made)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *monitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	mon, err := r.p.client.Monitor(state.ID.ValueString())
	if client.NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading the monitor failed", err.Error())
		return
	}
	r.fill(&state, mon)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *monitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state monitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	saved, err := r.p.client.UpdateMonitor(state.ID.ValueString(), r.toClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Updating the monitor failed", err.Error())
		return
	}
	r.fill(&plan, saved)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *monitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if err := r.p.client.DeleteMonitor(state.ID.ValueString()); err != nil && !client.NotFound(err) {
		resp.Diagnostics.AddError("Deleting the monitor failed", err.Error())
	}
}

func (r *monitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ================================================================================================= serverroom_port_alert

type portAlertResource struct{ p *configured }

func newPortAlertResource() resource.Resource { return &portAlertResource{} }

type portAlertModel struct {
	ID        types.String `tfsdk:"id"`
	AccountID types.Int64  `tfsdk:"account_id"`
	Threshold types.Int64  `tfsdk:"threshold"`
	Sustain   types.Int64  `tfsdk:"sustain"`
	Direction types.String `tfsdk:"direction"`
	PortSpeed types.Int64  `tfsdk:"port_speed_mbps"`
}

func (r *portAlertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_alert"
}

func (r *portAlertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Alert when a dedicated server's port stays above a share of its sold speed. Destroying it turns " +
			"the alert off.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"account_id": schema.Int64Attribute{Required: true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"threshold":       schema.Int64Attribute{Optional: true, Computed: true, Description: "Percent of the port speed, 50-100 (default 90)."},
			"sustain":         schema.Int64Attribute{Optional: true, Computed: true, Description: "Minutes, 10-240 (default 15)."},
			"direction":       schema.StringAttribute{Optional: true, Computed: true, Description: "either (default), inbound or outbound."},
			"port_speed_mbps": schema.Int64Attribute{Computed: true},
		},
	}
}

func (r *portAlertResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.p = configureResource(req, resp)
}

func (r *portAlertResource) save(ctx context.Context, plan *portAlertModel, enabled bool) error {
	rule := client.PortRule{Enabled: enabled, Threshold: intOr(plan.Threshold), Sustain: intOr(plan.Sustain),
		Direction: plan.Direction.ValueString()}
	if rule.Threshold == 0 {
		rule.Threshold = 90
	}
	if rule.Sustain == 0 {
		rule.Sustain = 15
	}
	if rule.Direction == "" {
		rule.Direction = "either"
	}
	saved, err := r.p.client.SetPortRule(int(plan.AccountID.ValueInt64()), rule)
	if err != nil {
		return err
	}
	plan.ID = types.StringValue(strconv.FormatInt(plan.AccountID.ValueInt64(), 10))
	plan.Threshold = types.Int64Value(int64(saved.Threshold))
	plan.Sustain = types.Int64Value(int64(saved.Sustain))
	plan.Direction = types.StringValue(saved.Direction)
	plan.PortSpeed = types.Int64Value(int64(saved.PortSpeedMbps))
	return nil
}

func (r *portAlertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portAlertModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if err := r.save(ctx, &plan, true); err != nil {
		resp.Diagnostics.AddError("Setting the port alert failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *portAlertResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portAlertModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	rule, err := r.p.client.PortRule(int(state.AccountID.ValueInt64()))
	if client.NotFound(err) || (err == nil && !rule.Enabled) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading the port alert failed", err.Error())
		return
	}
	state.Threshold = types.Int64Value(int64(rule.Threshold))
	state.Sustain = types.Int64Value(int64(rule.Sustain))
	state.Direction = types.StringValue(rule.Direction)
	state.PortSpeed = types.Int64Value(int64(rule.PortSpeedMbps))
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *portAlertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portAlertModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if err := r.save(ctx, &plan, true); err != nil {
		resp.Diagnostics.AddError("Updating the port alert failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *portAlertResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portAlertModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if err := r.save(ctx, &state, false); err != nil && !client.NotFound(err) {
		resp.Diagnostics.AddError("Turning the port alert off failed", err.Error())
	}
}

func (r *portAlertResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Import by account id", "e.g. terraform import serverroom_port_alert.web 246447")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("account_id"), id)...)
}

// ================================================================================================= serverroom_reverse_dns

type reverseDNSResource struct{ p *configured }

func newReverseDNSResource() resource.Resource { return &reverseDNSResource{} }

type reverseDNSModel struct {
	ID   types.String `tfsdk:"id"`
	IP   types.String `tfsdk:"ip"`
	Name types.String `tfsdk:"name"`
}

func (r *reverseDNSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_reverse_dns"
}

func (r *reverseDNSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The reverse DNS (PTR) name of one of your server's addresses. Destroying it leaves the name as it " +
			"is: an address always has a reverse name, and ours is restored only by support.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"ip": schema.StringAttribute{Required: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name": schema.StringAttribute{Required: true, Description: "The host name the address should answer to."},
		},
	}
}

func (r *reverseDNSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.p = configureResource(req, resp)
}

func (r *reverseDNSResource) set(ctx context.Context, plan *reverseDNSModel) error {
	if err := r.p.client.SetReverse(r.p.me.Dbid, plan.IP.ValueString(), strings.TrimSuffix(plan.Name.ValueString(), ".")); err != nil {
		return err
	}
	plan.ID = plan.IP
	return nil
}

func (r *reverseDNSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan reverseDNSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if err := r.set(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Setting reverse DNS failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *reverseDNSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state reverseDNSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	name, err := r.p.client.Reverse(state.IP.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading reverse DNS failed", err.Error())
		return
	}
	if name != "" {
		state.Name = types.StringValue(strings.TrimSuffix(name, "."))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *reverseDNSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan reverseDNSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if err := r.set(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Setting reverse DNS failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *reverseDNSResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *reverseDNSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip"), req.ID)...)
}

// ================================================================================================= serverroom_server_os

type serverOSResource struct{ p *configured }

func newServerOSResource() resource.Resource { return &serverOSResource{} }

type serverOSModel struct {
	ID             types.String `tfsdk:"id"`
	AccountID      types.Int64  `tfsdk:"account_id"`
	OS             types.String `tfsdk:"os"`
	SSHKeys        types.String `tfsdk:"ssh_keys"`
	UserData       types.String `tfsdk:"user_data"`
	AllowReinstall types.Bool   `tfsdk:"allow_reinstall"`
	Wait           types.Bool   `tfsdk:"wait"`
}

func (r *serverOSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_os"
}

func (r *serverOSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The operating system on a dedicated server. Changing `os` (or `ssh_keys` / `user_data`) " +
			"REINSTALLS the server and erases its disks -- only when `allow_reinstall = true`. Creating this resource " +
			"for a server already running `os` adopts it without touching it; destroying it leaves the server as it is.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"account_id": schema.Int64Attribute{Required: true, Description: "The server's service id (data.serverroom_servers).",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"os":              schema.StringAttribute{Required: true, Description: "Exactly as sold for the plan, e.g. \"Ubuntu 24.04\"."},
			"ssh_keys":        schema.StringAttribute{Optional: true, Description: "OpenSSH public keys for root, one per line (Linux)."},
			"user_data":       schema.StringAttribute{Optional: true, Sensitive: true, Description: "#cloud-config (Ubuntu) or a #! script, run once after the install."},
			"allow_reinstall": schema.BoolAttribute{Optional: true, Description: "Must be true for a change to reinstall the server."},
			"wait": schema.BoolAttribute{Optional: true, Description: "Wait (up to 90 minutes) until the reinstall is done " +
				"and the server is back, so what depends on it runs against the new system."},
		},
	}
}

func (r *serverOSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.p = configureResource(req, resp)
}

func (r *serverOSResource) current(accountID int) (string, error) {
	features, err := r.p.client.Features(accountID)
	if err != nil {
		return "", err
	}
	return client.FeatureMap(features)["OS"], nil
}

func (r *serverOSResource) reinstall(plan serverOSModel) error {
	if plan.AllowReinstall.IsNull() || !plan.AllowReinstall.ValueBool() {
		return fmt.Errorf("this would reinstall server %d with %s and erase its disks; set allow_reinstall = true to go ahead",
			plan.AccountID.ValueInt64(), plan.OS.ValueString())
	}
	return r.p.client.Reinstall(int(plan.AccountID.ValueInt64()), plan.OS.ValueString(), plan.SSHKeys.ValueString(),
		plan.UserData.ValueString())
}

// waitReady follows the install that began after `after` (client.LastStarted, read before the request) until it
// is ready, failed, or 90 minutes pass.
func (r *serverOSResource) waitReady(ctx context.Context, plan serverOSModel, after float64) error {
	if plan.Wait.IsNull() || !plan.Wait.ValueBool() {
		return nil
	}
	id := int(plan.AccountID.ValueInt64())
	deadline := time.Now().Add(90 * time.Minute)
	seen := false
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(30 * time.Second):
		}
		list, err := r.p.client.SetupProgress(id)
		if err != nil {
			continue // a blip while the API restarts is not a failed install
		}
		p := client.NewInstall(list, id, after)
		if p == nil {
			if seen {
				return nil // the half-hour window passed after it finished
			}
			continue // not queued yet: the list may still show the previous install
		}
		seen = true
		tflog.Info(ctx, "reinstall progress", map[string]interface{}{"stage": p.Stage, "percent": p.Percent})
		switch p.State {
		case "ready":
			return nil
		case "failed":
			return fmt.Errorf("the reinstall of server %d failed at %s; support has been told", id, p.Stage)
		}
	}
	return fmt.Errorf("server %d was still installing after 90 minutes", id)
}

func (r *serverOSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverOSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	running, err := r.current(int(plan.AccountID.ValueInt64()))
	if err == nil && running != plan.OS.ValueString() {
		after := r.p.client.LastStarted(int(plan.AccountID.ValueInt64()))
		if err = r.reinstall(plan); err == nil {
			err = r.waitReady(ctx, plan, after)
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Setting the operating system failed", err.Error())
		return
	}
	plan.ID = types.StringValue(strconv.FormatInt(plan.AccountID.ValueInt64(), 10))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverOSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverOSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	running, err := r.current(int(state.AccountID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Reading the operating system failed", err.Error())
		return
	}
	if running != "" {
		state.OS = types.StringValue(running)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *serverOSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state serverOSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	changed := !plan.OS.Equal(state.OS) || !plan.SSHKeys.Equal(state.SSHKeys) || !plan.UserData.Equal(state.UserData)
	if changed {
		after := r.p.client.LastStarted(int(plan.AccountID.ValueInt64()))
		if err := r.reinstall(plan); err != nil {
			resp.Diagnostics.AddError("Reinstalling the server failed", err.Error())
			return
		}
		if err := r.waitReady(ctx, plan, after); err != nil {
			resp.Diagnostics.AddError("Reinstalling the server failed", err.Error())
			return
		}
	}
	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverOSResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}
