package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body map[string]interface{})) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != "srk_test" {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":{"code":"unauthenticated","message":"This credential is not valid."}}`)
			return
		}
		var body map[string]interface{}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		handler(w, r, body)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "srk_test")
	return c
}

func TestBrandURL(t *testing.T) {
	cases := map[string][2]string{
		"primcast.com":                    {"https://www.primcast.com/api", "primcast.com"},
		"primcast":                        {"https://www.primcast.com/api", "primcast.com"},
		"https://www.primcast.com":        {"https://www.primcast.com/api", "primcast.com"},
		"https://www.serverroom.net/api/": {"https://www.serverroom.net/api", "serverroom.net"},
		"http://127.0.0.1:8080/api/":      {"http://127.0.0.1:8080/api", "127.0.0.1"},
	}
	for in, want := range cases {
		base, brand := BrandURL(in)
		if base != want[0] || brand != want[1] {
			t.Errorf("BrandURL(%q) = %q, %q; want %q, %q", in, base, brand, want[0], want[1])
		}
	}
}

// One provider for every brand: with no URL the key's own prefix picks the website.
func TestTheKeyPicksTheBrand(t *testing.T) {
	cases := map[string]string{
		"srk_x": "https://www.serverroom.net/api", "pck_x": "https://www.primcast.com/api",
		" pck_x ": "https://www.primcast.com/api", "weird": "https://www.serverroom.net/api",
	}
	for key, want := range cases {
		if c := New("", key); c.BaseURL != want {
			t.Errorf("New(\"\", %q).BaseURL = %q; want %q", key, c.BaseURL, want)
		}
	}
	if CheckKey("", "xyz_unknownprefix") != ErrNeedsURL || CheckKey("https://example.com/api", "xyz_k") != nil ||
		CheckKey("", "srk_k") != nil {
		t.Error("CheckKey must ask for the URL only for an unknown key with no URL")
	}
	if c := New("http://127.0.0.1:9/api", "pck_x"); c.BaseURL != "http://127.0.0.1:9/api" {
		t.Errorf("an explicit URL must win over the key's brand, got %q", c.BaseURL)
	}
}

// Stock and plan routes are public: the key must not be sent (a key is refused on routes outside its list).
func TestPublicRoutesSendNoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != "" {
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case "/readyservers/list":
			var f map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&f)
			if f["maxprice"] == nil || f["count"] != float64(3) {
				w.WriteHeader(400)
				return
			}
			io.WriteString(w, `[{"name":"s924","serverid":924,"ram":64.0,"price":9,"order_url":"https://x/order?id=s924"}]`)
		case "/order/config":
			io.WriteString(w, `{"OS":[{"Value":"Ubuntu 24.04"},{"Value":"Debian 12"},{"Value":"Ubuntu 24.04"}]}`)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "srk_test")
	list, err := c.Stock(StockFilter{Limit: 3})
	if err != nil || len(list) != 1 || list[0].RAM != 64 || list[0].OrderURL == "" {
		t.Fatalf("Stock = %+v, %v", list, err)
	}
	names, err := c.PlanOS("M700")
	if err != nil || strings.Join(names, ",") != "Ubuntu 24.04,Debian 12" {
		t.Fatalf("PlanOS = %v, %v", names, err)
	}
}

func TestRescueRoutes(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request, body map[string]interface{}) {
		switch r.URL.Path {
		case "/rescue/start":
			if body["sid"] != float64(77) || body["accountid"] != float64(5) || body["sshkeys"] != "ssh-ed25519 AAA" {
				w.WriteHeader(712)
				io.WriteString(w, "bad body")
				return
			}
			io.WriteString(w, `{"session":{"session_id":"r1","state":"booting","address":"203.0.113.9","username":"root","password":"pw","active":true,"steps":[]}}`)
		case "/rescue/77":
			io.WriteString(w, `{"sid":77,"available":false,"reason":"Rescue mode is already running","session":null}`)
		case "/rescue/end":
			w.WriteHeader(201)
			io.WriteString(w, "Rescue mode is not running on this server")
		}
	})
	s, err := c.StartRescue(5, 77, "ssh-ed25519 AAA")
	if err != nil || s.Address != "203.0.113.9" || s.Password != "pw" || s.Extra["steps"] == nil {
		t.Fatalf("StartRescue = %+v, %v", s, err)
	}
	st, err := c.Rescue(5, 77)
	if err != nil || st.Available || st.Session != nil {
		t.Fatalf("Rescue = %+v, %v", st, err)
	}
	if err := c.EndRescue(5, 77); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("EndRescue must surface the refusal, got %v", err)
	}
}

func TestModernErrorsCarryTheMessage(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request, _ map[string]interface{}) {
		w.WriteHeader(400)
		io.WriteString(w, `{"error":{"code":"invalid","message":"The Gold tier checks at most every 60 seconds"}}`)
	})
	_, err := c.CreateMonitor(Monitor{Name: "x", Type: "ping", Target: "8.8.8.8", Interval: 10})
	if err == nil || !strings.Contains(err.Error(), "at most every 60 seconds") || err.(*Error).Code != "invalid" {
		t.Fatalf("got %v", err)
	}
}

func TestLegacyTreatsNon200AsRefusal(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request, body map[string]interface{}) {
		if r.URL.Path == "/updatereverse" {
			if body["dns"] != "mail.example.com" || body["ip"] != "162.244.80.25" {
				t.Errorf("body %v", body)
			}
			w.WriteHeader(201)
			io.WriteString(w, "Reverse DNS zone not managed by us")
		}
	})
	err := c.SetReverse(1, "162.244.80.25", "mail.example.com")
	if err == nil || !strings.Contains(err.Error(), "not managed by us") || err.(*Error).Status != 201 {
		t.Fatalf("got %v", err)
	}
}

func TestMonitorCreatedAnswer201IsSuccess(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request, body map[string]interface{}) {
		if r.Method != "POST" || r.URL.Path != "/alerts/monitors" || body["accountid"].(float64) != 246447 {
			t.Errorf("%s %s %v", r.Method, r.URL.Path, body)
		}
		w.WriteHeader(201)
		io.WriteString(w, `{"id":"mon_1","name":"Shop","type":"http","target":"https://shop.example.com/","interval":60,"account_id":246447,"state":{"status":"pending"}}`)
	})
	m, err := c.CreateMonitor(Monitor{AccountID: 246447, Name: "Shop", Type: "http", Target: "https://shop.example.com/"})
	if err != nil || m.ID != "mon_1" || m.Account != 246447 || m.State.Status != "pending" {
		t.Fatalf("got %+v, %v", m, err)
	}
}

func TestReinstallSendsExtrasOnlyWhenGiven(t *testing.T) {
	var seen map[string]interface{}
	c := fake(t, func(w http.ResponseWriter, r *http.Request, body map[string]interface{}) { seen = body })
	if err := c.Reinstall(7, "Debian 12", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := seen["user_data"]; ok {
		t.Fatalf("empty user_data was sent: %v", seen)
	}
	if err := c.Reinstall(7, "Debian 12", "ssh-ed25519 AAAA", "#!/bin/sh\n"); err != nil {
		t.Fatal(err)
	}
	if seen["ssh_keys"] != "ssh-ed25519 AAAA" || seen["user_data"] != "#!/bin/sh\n" {
		t.Fatalf("got %v", seen)
	}
}

func TestWebhookCreateKeepsTheSecretAndNotFoundIsRecognised(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request, _ map[string]interface{}) {
		if r.Method == "POST" {
			io.WriteString(w, `{"webhook":{"id":"wh_1","url":"https://x.example/h","events":["*"],"active":true},"secret":"whsec_abc"}`)
			return
		}
		io.WriteString(w, `{"webhooks":[]}`)
	})
	made, err := c.CreateWebhook(Webhook{URL: "https://x.example/h", Events: []string{"*"}})
	if err != nil || made.Secret != "whsec_abc" || made.ID != "wh_1" {
		t.Fatalf("got %+v, %v", made, err)
	}
	if _, err := c.Webhook("wh_gone"); !NotFound(err) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestBadKeyIsSaidPlainly(t *testing.T) {
	c := fake(t, func(http.ResponseWriter, *http.Request, map[string]interface{}) {})
	c.Key = "nope"
	if _, err := c.Whoami(); err == nil || err.(*Error).Status != 401 {
		t.Fatalf("got %v", err)
	}
}

func TestFeatureMap(t *testing.T) {
	m := FeatureMap([]Feature{{Item: "ServerID", Value: 4317.0}, {Item: "OS", Value: "Ubuntu 24.04"}, {Item: "x", Value: nil}})
	if m["ServerID"] != "4317" || m["OS"] != "Ubuntu 24.04" {
		t.Fatalf("got %v", m)
	}
	if _, ok := m["x"]; ok {
		t.Fatal("nil value kept")
	}
}

// The API sends times (and sometimes percent) as JSON floats.
func TestSetupProgressTakesFloats(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request, body map[string]interface{}) {
		io.WriteString(w, `{"progress":[{"accountid":5,"state":"running","stage":"installing","stages":{"queued":1794416452.0,"installing":1794416999.5},"percent":42.0,"started":1794416452.0,"updated":1794417000.25}]}`)
	})
	list, err := c.SetupProgress(5)
	if err != nil || len(list) != 1 || list[0].Percent != 42 || list[0].Stages["queued"] != 1794416452 {
		t.Fatalf("SetupProgress = %+v, %v", list, err)
	}
}

// Right after a reinstall request the list still shows the previous install, often "ready": waiting must skip it.
func TestNewInstallSkipsThePreviousOne(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request, body map[string]interface{}) {
		io.WriteString(w, `{"progress":[{"accountid":5,"state":"ready","stage":"ready","percent":100,"started":1000.0}]}`)
	})
	after := c.LastStarted(5)
	if after != 1000 {
		t.Fatalf("LastStarted = %v", after)
	}
	old := []Progress{{AccountID: 5, State: "ready", Started: 1000}}
	if p := NewInstall(old, 5, after); p != nil {
		t.Fatalf("the previous install counted as the new one: %+v", p)
	}
	fresh := []Progress{{AccountID: 6, State: "running", Started: 2000}, {AccountID: 5, State: "running", Started: 1100}}
	if p := NewInstall(fresh, 5, after); p == nil || p.Started != 1100 {
		t.Fatalf("NewInstall = %+v", p)
	}
	if p := NewInstall(old, 5, 0); p == nil {
		t.Fatal("with nothing before, any install on the account is the new one")
	}
}
