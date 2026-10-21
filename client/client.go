// Package client talks to the Server Room / Primcast customer API with an API key.
//
// One client serves the Terraform provider and the servcast CLI, so both see the same routes, the same errors and
// the same idea of success. The API is the brand's own: https://www.serverroom.net/api or https://www.primcast.com/api,
// with the key in the X-Auth-Token header. A key reaches only the routes its permissions list (ServCast
// apikeys/scopes.py) and never anything that spends money, so nothing here orders or cancels a service.
//
// SUCCESS IS PER ROUTE. The newer routes (webhooks, alerts) answer 2xx with JSON and errors as
// {"error": {"code", "message"}}. The older ones answer 200 for success and use 201, 202, 204 and 7xx for "refused",
// with a plain-text reason. Each method below says which it is, rather than one rule guessing.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Version is sent in the User-Agent, so our request logs can tell the provider and the CLI from a browser.
var Version = "0.1.0"

// Client is safe for concurrent use.
type Client struct {
	BaseURL string // e.g. https://www.serverroom.net/api
	Key     string
	Brand   string // serverroom.net or primcast.com, sent as X-Website
	HTTP    *http.Client
	Agent   string
}

// Error is a refusal the API explained.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s (HTTP %d, %s)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
}

// NotFound reports whether err is the API saying the thing does not exist.
func NotFound(err error) bool {
	e, ok := err.(*Error)
	return ok && (e.Status == 404 || e.Code == "not_found")
}

// New makes a client for a brand: "serverroom.net" (the default) or "primcast.com", or a full base URL.
func New(brandOrURL, key string) *Client {
	base, brand := BrandURL(brandOrURL)
	return &Client{BaseURL: base, Key: key, Brand: brand, HTTP: &http.Client{Timeout: 60 * time.Second},
		Agent: "servcast-go/" + Version}
}

// BrandURL turns a brand name or URL into (API base URL, brand host).
func BrandURL(value string) (string, string) {
	value = strings.TrimSpace(value)
	switch strings.TrimPrefix(strings.TrimPrefix(value, "www."), "https://www.") {
	case "", "serverroom", "serverroom.net":
		return "https://www.serverroom.net/api", "serverroom.net"
	case "primcast", "primcast.com":
		return "https://www.primcast.com/api", "primcast.com"
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return strings.TrimRight(value, "/"), ""
	}
	return strings.TrimRight(value, "/"), strings.TrimPrefix(u.Hostname(), "www.")
}

type response struct {
	status int
	body   []byte
}

func (c *Client) do(method, path string, body interface{}) (*response, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", c.Key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.Agent)
	if c.Brand != "" {
		req.Header.Set("X-Website", c.Brand)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	return &response{status: resp.StatusCode, body: raw}, nil
}

func refusal(r *response) error {
	var shaped struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(r.body, &shaped) == nil && shaped.Error.Message != "" {
		return &Error{Status: r.status, Code: shaped.Error.Code, Message: shaped.Error.Message}
	}
	text := strings.TrimSpace(string(r.body))
	if text == "" || len(text) > 400 || strings.HasPrefix(text, "<") {
		text = http.StatusText(r.status)
		if text == "" {
			text = "refused"
		}
	}
	if r.status == 401 {
		text = "the API key was refused: " + text
	}
	return &Error{Status: r.status, Message: text}
}

// modern: any 2xx is success, the body (if any) is JSON.
func (c *Client) modern(method, path string, body, out interface{}) error {
	r, err := c.do(method, path, body)
	if err != nil {
		return err
	}
	if r.status < 200 || r.status >= 300 {
		return refusal(r)
	}
	if out != nil && len(bytes.TrimSpace(r.body)) > 0 {
		if err := json.Unmarshal(r.body, out); err != nil {
			return fmt.Errorf("unexpected answer from %s %s: %w", method, path, err)
		}
	}
	return nil
}

// legacy: only 200 is success; every other code is a refusal with a reason.
func (c *Client) legacy(method, path string, body, out interface{}) error {
	r, err := c.do(method, path, body)
	if err != nil {
		return err
	}
	if r.status != 200 {
		return refusal(r)
	}
	if out != nil && len(bytes.TrimSpace(r.body)) > 0 {
		if err := json.Unmarshal(r.body, out); err != nil {
			return fmt.Errorf("unexpected answer from %s %s: %w", method, path, err)
		}
	}
	return nil
}

// ------------------------------------------------------------------------------------------------- account

// Whoami is the key's own identity.
type Whoami struct {
	Dbid       int    `json:"dbid"`
	UID        int    `json:"uid"`
	Brand      string `json:"brand"`
	Accounts   []int  `json:"accounts"`
	Credential string `json:"credential"`
	Key        struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	} `json:"key"`
}

func (c *Client) Whoami() (*Whoami, error) {
	var out Whoami
	return &out, c.modern("GET", "/apikeys/whoami", nil, &out)
}

// Service is one row of the customer's services.
type Service struct {
	AccountID   int    `json:"AccountID"`
	AccountType string `json:"AccountType"`
	AccountName string `json:"AccountName"`
	Status      int    `json:"Status"`
}

func (c *Client) Services(uid int) ([]Service, error) {
	var out []Service
	return out, c.legacy("GET", fmt.Sprintf("/user/accounts/%d", uid), nil, &out)
}

// Feature is one line of a service: ServerID, CPU, RAM, OS, PortSpeed, Location...
type Feature struct {
	Item  string      `json:"Item"`
	Value interface{} `json:"Value"`
}

func (c *Client) Features(accountID int) ([]Feature, error) {
	var out []Feature
	return out, c.legacy("GET", fmt.Sprintf("/accounts/features/%d", accountID), nil, &out)
}

// FeatureMap folds the feature lines into Item -> Value (text), the last line of an Item winning.
func FeatureMap(features []Feature) map[string]string {
	out := map[string]string{}
	for _, f := range features {
		if f.Value == nil {
			continue
		}
		out[f.Item] = strings.TrimSpace(fmt.Sprint(f.Value))
	}
	return out
}

// ------------------------------------------------------------------------------------------------- servers

// IP is one address of a server, with its reverse DNS when we manage the zone.
type IP struct {
	IP      string `json:"ip"`
	Reverse string `json:"reverse,omitempty"`
}

func (c *Client) IPs(accountID, serverID int) ([]IP, error) {
	var out []IP
	return out, c.legacy("GET", fmt.Sprintf("/ips/%d/%d", accountID, serverID), nil, &out)
}

func (c *Client) Reverse(ip string) (string, error) {
	var out struct {
		Reverse *string `json:"reverse"`
	}
	if err := c.legacy("GET", "/reversedns/"+url.PathEscape(ip), nil, &out); err != nil {
		return "", err
	}
	if out.Reverse == nil {
		return "", nil
	}
	return *out.Reverse, nil
}

func (c *Client) SetReverse(dbid int, ip, name string) error {
	return c.legacy("POST", "/updatereverse", map[string]interface{}{"dbid": dbid, "ip": ip, "dns": name}, nil)
}

func (c *Client) Reboot(accountID int) error {
	return c.legacy("POST", "/rebootserver", map[string]interface{}{"accountid": accountID}, nil)
}

// Reinstall WIPES the server and installs osName. sshKeys and userData are optional (Linux only).
func (c *Client) Reinstall(accountID int, osName, sshKeys, userData string) error {
	body := map[string]interface{}{"accountid": accountID, "osn": osName}
	if strings.TrimSpace(sshKeys) != "" {
		body["ssh_keys"] = sshKeys
	}
	if strings.TrimSpace(userData) != "" {
		body["user_data"] = userData
	}
	return c.legacy("POST", "/reinstallos", body, nil)
}

// ------------------------------------------------------------------------------------------------- webhooks

type Webhook struct {
	ID          string   `json:"id,omitempty"`
	URL         string   `json:"url"`
	Events      []string `json:"events"`
	Description string   `json:"description,omitempty"`
	Active      *bool    `json:"active,omitempty"`
	Secret      string   `json:"secret,omitempty"`
}

func (c *Client) Webhooks() ([]Webhook, error) {
	var out struct {
		Webhooks []Webhook `json:"webhooks"`
	}
	return out.Webhooks, c.modern("GET", "/webhooks", nil, &out)
}

func (c *Client) Webhook(id string) (*Webhook, error) {
	hooks, err := c.Webhooks()
	if err != nil {
		return nil, err
	}
	for i := range hooks {
		if hooks[i].ID == id {
			return &hooks[i], nil
		}
	}
	return nil, &Error{Status: 404, Code: "not_found", Message: "no webhook " + id}
}

func (c *Client) CreateWebhook(w Webhook) (*Webhook, error) {
	var out struct {
		Webhook Webhook `json:"webhook"`
		Secret  string  `json:"secret"`
	}
	if err := c.modern("POST", "/webhooks", w, &out); err != nil {
		return nil, err
	}
	if out.Webhook.Secret == "" {
		out.Webhook.Secret = out.Secret
	}
	return &out.Webhook, nil
}

func (c *Client) UpdateWebhook(id string, w Webhook) error {
	return c.modern("PATCH", "/webhooks/"+url.PathEscape(id), w, nil)
}

func (c *Client) DeleteWebhook(id string) error {
	return c.modern("DELETE", "/webhooks/"+url.PathEscape(id), nil, nil)
}

// ------------------------------------------------------------------------------------------------- alerts

type Monitor struct {
	ID            string `json:"id,omitempty"`
	AccountID     int    `json:"accountid,omitempty"`
	Account       int    `json:"account_id,omitempty"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Target        string `json:"target"`
	Port          int    `json:"port,omitempty"`
	Interval      int    `json:"interval,omitempty"`
	Timeout       int    `json:"timeout,omitempty"`
	Confirm       int    `json:"confirm,omitempty"`
	Expect        []int  `json:"expect,omitempty"`
	Keyword       string `json:"keyword,omitempty"`
	KeywordAbsent bool   `json:"keyword_absent,omitempty"`
	Paused        bool   `json:"paused"`
	State         struct {
		Status    string `json:"status"`
		LastCheck int64  `json:"lastcheck"`
		Ms        int    `json:"ms"`
		Error     string `json:"error"`
	} `json:"state,omitempty"`
}

func (c *Client) Monitors() ([]Monitor, error) {
	var out struct {
		Monitors []Monitor `json:"monitors"`
	}
	return out.Monitors, c.modern("GET", "/alerts/monitors", nil, &out)
}

func (c *Client) Monitor(id string) (*Monitor, error) {
	all, err := c.Monitors()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, &Error{Status: 404, Code: "not_found", Message: "no monitor " + id}
}

func (c *Client) CreateMonitor(m Monitor) (*Monitor, error) {
	var out Monitor
	return &out, c.modern("POST", "/alerts/monitors", m, &out)
}

func (c *Client) UpdateMonitor(id string, m Monitor) (*Monitor, error) {
	var out Monitor
	return &out, c.modern("PUT", "/alerts/monitors/"+url.PathEscape(id), m, &out)
}

func (c *Client) DeleteMonitor(id string) error {
	return c.modern("DELETE", "/alerts/monitors/"+url.PathEscape(id), nil, nil)
}

// TestResult is one check run on demand.
type TestResult struct {
	OK         bool   `json:"ok"`
	Ms         int    `json:"ms"`
	Error      string `json:"error"`
	HTTPStatus int    `json:"http_status"`
}

func (c *Client) TestMonitor(id string) (*TestResult, error) {
	var out TestResult
	return &out, c.modern("POST", "/alerts/monitors/"+url.PathEscape(id)+"/test", map[string]string{}, &out)
}

type PortRule struct {
	Enabled       bool   `json:"enabled"`
	Threshold     int    `json:"threshold"`
	Sustain       int    `json:"sustain"`
	Direction     string `json:"direction"`
	PortSpeedMbps int    `json:"port_speed_mbps,omitempty"`
}

func (c *Client) PortRule(accountID int) (*PortRule, error) {
	var out PortRule
	return &out, c.modern("GET", fmt.Sprintf("/alerts/port/%d", accountID), nil, &out)
}

func (c *Client) SetPortRule(accountID int, r PortRule) (*PortRule, error) {
	var out PortRule
	return &out, c.modern("PUT", fmt.Sprintf("/alerts/port/%d", accountID), r, &out)
}
