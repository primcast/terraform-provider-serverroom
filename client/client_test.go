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
		"":                           {"https://www.serverroom.net/api", "serverroom.net"},
		"primcast.com":               {"https://www.primcast.com/api", "primcast.com"},
		"https://www.primcast.com":   {"https://www.primcast.com/api", "primcast.com"},
		"http://127.0.0.1:8080/api/": {"http://127.0.0.1:8080/api", "127.0.0.1"},
	}
	for in, want := range cases {
		base, brand := BrandURL(in)
		if base != want[0] || brand != want[1] {
			t.Errorf("BrandURL(%q) = %q, %q; want %q, %q", in, base, brand, want[0], want[1])
		}
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
