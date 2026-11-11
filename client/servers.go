package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// ------------------------------------------------------------------------------------------------- services

// RenameService sets a service's name; a dedicated server's name is its host name. (account:write, legacy)
func (c *Client) RenameService(accountID int, name string) error {
	return c.legacy("PUT", fmt.Sprintf("/user/account/%d", accountID), map[string]string{"AccountName": name}, nil)
}

// Progress is where the install or reinstall of one server stands.
type Progress struct {
	AccountID int              `json:"accountid"`
	Kind      string           `json:"kind"`
	OS        string           `json:"os"`
	State     string           `json:"state"` // running, attention, failed, ready
	Stage     string           `json:"stage"` // queued, preparing, installing, booting, finishing, ready
	Stages    map[string]int64 `json:"stages"`
	Percent   int              `json:"percent"`
	Started   int64            `json:"started"`
	Updated   int64            `json:"updated"`
}

// SetupProgress lists the installs running or finished in the last half hour; accountID 0 for all. (servers:read)
func (c *Client) SetupProgress(accountID int) ([]Progress, error) {
	path := "/account/setupprogress"
	if accountID > 0 {
		path += fmt.Sprintf("?accountid=%d", accountID)
	}
	var out struct {
		Progress []Progress `json:"progress"`
	}
	return out.Progress, c.legacy("GET", path, nil, &out)
}

// ------------------------------------------------------------------------------------------------- rescue

// RescueSession is a running rescue session: the server netbooted into a live system, reachable over SSH as root.
type RescueSession struct {
	SessionID string                 `json:"session_id"`
	State     string                 `json:"state"`
	Address   string                 `json:"address"`
	Active    bool                   `json:"active"`
	Username  string                 `json:"username"`
	Password  string                 `json:"password"`
	Extra     map[string]interface{} `json:"-"`
}

// UnmarshalJSON keeps every field the API sends, so a field added later still reaches --json output.
func (r *RescueSession) UnmarshalJSON(raw []byte) error {
	type plain RescueSession
	if err := json.Unmarshal(raw, (*plain)(r)); err != nil {
		return err
	}
	return json.Unmarshal(raw, &r.Extra)
}

// MarshalJSON writes back everything the API sent.
func (r RescueSession) MarshalJSON() ([]byte, error) {
	if r.Extra != nil {
		return json.Marshal(r.Extra)
	}
	type plain RescueSession
	return json.Marshal(plain(r))
}

// RescueState says whether rescue mode runs on a server or may start, and why not.
type RescueState struct {
	SID       int            `json:"sid"`
	Available bool           `json:"available"`
	Reason    string         `json:"reason"`
	Session   *RescueSession `json:"session"`
}

// Rescue reads a server's rescue state. sid is the server ID (the ServerID feature). (rescue:read, legacy)
func (c *Client) Rescue(accountID, sid int) (*RescueState, error) {
	var out RescueState
	return &out, c.legacy("GET", fmt.Sprintf("/rescue/%d?accountid=%d", sid, accountID), nil, &out)
}

// StartRescue powers the server off and boots the rescue system with sshKeys for root. (rescue:write, legacy)
func (c *Client) StartRescue(accountID, sid int, sshKeys string) (*RescueSession, error) {
	var out struct {
		Session *RescueSession `json:"session"`
	}
	body := map[string]interface{}{"sid": sid, "accountid": accountID}
	if strings.TrimSpace(sshKeys) != "" {
		body["sshkeys"] = sshKeys
	}
	if err := c.legacy("POST", "/rescue/start", body, &out); err != nil {
		return nil, err
	}
	return out.Session, nil
}

// EndRescue boots the server from its own disk again. (rescue:write, legacy)
func (c *Client) EndRescue(accountID, sid int) error {
	return c.legacy("POST", "/rescue/end", map[string]interface{}{"sid": sid, "accountid": accountID}, nil)
}

// ------------------------------------------------------------------------------------------------- stock (public)

// StockServer is one machine for sale now, ready to deliver.
type StockServer struct {
	Name      string  `json:"name"`
	ServerID  int     `json:"serverid"`
	Type      string  `json:"type"`
	CPU       string  `json:"cpu"`
	RAM       float64 `json:"ram"`
	Disk      string  `json:"hdd"`
	GPU       string  `json:"gpu,omitempty"`
	Location  string  `json:"location"`
	OS        string  `json:"os"`
	Traffic   string  `json:"traffic"`
	Price     float64 `json:"price"`
	ListPrice float64 `json:"listprice,omitempty"`
	Discount  *struct {
		Percent float64 `json:"percent"`
	} `json:"discount,omitempty"`
	FirstMonth bool   `json:"firstmonth,omitempty"`
	OrderURL   string `json:"order_url"`
}

// StockFilter narrows the stock list. Values match forgivingly: "Amsterdam" finds "Amsterdam, EU".
type StockFilter struct {
	Location []string `json:"location,omitempty"`
	CPU      []string `json:"cpu,omitempty"`
	GPU      []string `json:"gpu,omitempty"`
	Type     []string `json:"type,omitempty"`
	OS       []string `json:"os,omitempty"`
	MinRAM   int      `json:"minram,omitempty"`
	MaxPrice float64  `json:"maxprice,omitempty"`
	Limit    int      `json:"count,omitempty"`
}

// Stock lists the machines for sale now, cheapest first, each with its order link on the key's own brand. It needs
// no permission (the website reads it with no key), so it is sent without one.
func (c *Client) Stock(f StockFilter) ([]StockServer, error) {
	if f.MaxPrice == 0 && f.MinRAM == 0 && len(f.Location)+len(f.CPU)+len(f.GPU)+len(f.Type)+len(f.OS) == 0 {
		f.MaxPrice = 1e9 // the route wants one filter; "any price" is the honest one
	}
	r, err := c.send("POST", "/readyservers/list", f, false)
	if err != nil {
		return nil, err
	}
	if r.status != 200 {
		return nil, refusal(r)
	}
	var out []StockServer
	if err := json.Unmarshal(r.body, &out); err != nil {
		return nil, fmt.Errorf("unexpected answer from the stock list: %w", err)
	}
	return out, nil
}

// StockOptions lists the values one stock filter takes: location, cpu, gpu, ram, type, os, traffic.
func (c *Client) StockOptions(spec string) ([]string, error) {
	r, err := c.send("GET", "/readyservers/options?specs="+url.QueryEscape(spec), nil, false)
	if err != nil {
		return nil, err
	}
	if r.status != 200 {
		return nil, refusal(r)
	}
	var out map[string][]interface{}
	if err := json.Unmarshal(r.body, &out); err != nil {
		return nil, fmt.Errorf("unexpected answer from the stock options: %w", err)
	}
	var values []string
	for _, v := range out[spec] {
		values = append(values, strings.TrimSpace(fmt.Sprint(v)))
	}
	return values, nil
}

// PlanOS lists the operating systems a plan (e.g. "D104", "M700") installs, by the exact names /reinstallos takes, in
// the catalogue's order. Public, like Stock.
func (c *Client) PlanOS(plan string) ([]string, error) {
	r, err := c.send("GET", "/order/config?type="+url.QueryEscape(plan), nil, false)
	if err != nil {
		return nil, err
	}
	if r.status != 200 {
		return nil, refusal(r)
	}
	var config struct {
		OS []struct {
			Value string `json:"Value"`
		} `json:"OS"`
	}
	if err := json.Unmarshal(r.body, &config); err != nil || len(config.OS) == 0 {
		return nil, &Error{Status: 404, Code: "not_found", Message: "no plan " + plan + " (serverroom stock options type)"}
	}
	seen := map[string]bool{}
	var names []string
	for _, o := range config.OS {
		if o.Value != "" && !seen[o.Value] {
			seen[o.Value] = true
			names = append(names, o.Value)
		}
	}
	return names, nil
}

// ------------------------------------------------------------------------------------------------- coming next
//
// Routes the bare-metal API is getting (2026-11) and the provider/CLI will use once a key may call them. Each lands
// as a client method here plus a resource; nothing else in the provider has to change:
//
//   - private network between your servers      -> PrivateNetworks / serverroom_private_network
//   - server firewall rules                     -> FirewallRules   / serverroom_firewall
//   - extra IPv4 addresses and routed subnets   -> IPs + IPRoutes  / serverroom_ip_route
//   - self-service upgrades                     -> Upgrades        / serverroom_server (plan changes)
//   - boot from ISO                             -> BootISO         / CLI "iso"
//   - activity log                              -> Activity        / CLI "activity", data source
//   - deploy from your own image                -> ImageDeploy     / serverroom_server_os.image
