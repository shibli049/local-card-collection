package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListenHostOptions(t *testing.T) {
	for _, scenario := range []struct{ input, want string }{
		{"localhost", "127.0.0.1"}, {"127.0.0.1", "127.0.0.1"}, {"0.0.0.0", "0.0.0.0"}, {" LOCALHOST ", "127.0.0.1"},
	} {
		host, err := normalizeListenHost(scenario.input)
		if err != nil || host != scenario.want {
			t.Fatalf("%q: %q, %v", scenario.input, host, err)
		}
	}
	for _, host := range []string{"", "example.com", "192.168.1.50", "0.0.0.0:8765", "::"} {
		if _, err := normalizeListenHost(host); err == nil {
			t.Fatal("unsupported host accepted", host)
		}
	}
}

func TestListenerUsesConfiguredInterfaceAndPort(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "0.0.0.0"} {
		t.Run(host, func(t *testing.T) {
			listener, err := listenHTTP(host, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			address := listener.Addr().(*net.TCPAddr)
			if !address.IP.Equal(net.ParseIP(host)) || address.Port == 0 {
				t.Fatal("wrong listen address", address)
			}
		})
	}
	for _, port := range []int{-1, 65536} {
		if listener, err := listenHTTP("127.0.0.1", port); err == nil {
			listener.Close()
			t.Fatal("invalid port accepted", port)
		}
	}
}

func TestListenerKeepsHostWhenDefaultPortIsBusy(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "0.0.0.0"} {
		t.Run(host, func(t *testing.T) {
			occupied, err := net.Listen("tcp4", net.JoinHostPort(host, "8765"))
			if err != nil {
				t.Skip("port 8765 is already in use")
			}
			defer occupied.Close()
			listener, err := listenHTTP(host, 8765)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			address := listener.Addr().(*net.TCPAddr)
			if address.Port == 8765 || !address.IP.Equal(net.ParseIP(host)) {
				t.Fatal("fallback changed bind mode", address)
			}
		})
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if other, err := listenHTTP("127.0.0.1", port); err == nil {
		other.Close()
		t.Fatal("an explicit busy nondefault port was silently replaced")
	}
}

func TestRequestHostChecksFollowListenMode(t *testing.T) {
	for _, scenario := range []struct {
		host          string
		network, want bool
	}{
		{"localhost:8765", false, true}, {"127.0.0.1:8765", false, true}, {"[::1]:8765", false, true},
		{"192.168.1.50:8765", false, false}, {"192.168.1.50:8765", true, true}, {"10.0.0.2:8765", true, true},
		{"attacker.example:8765", false, false}, {"attacker.example:8765", true, false}, {"localhost.attacker.example", true, false},
	} {
		if got := allowedRequestHost(scenario.host, scenario.network); got != scenario.want {
			t.Fatalf("host=%s network=%t allowed=%t", scenario.host, scenario.network, got)
		}
	}
}

func TestNetworkModePreservesAuthenticationAndCrossSiteChecks(t *testing.T) {
	c := newTestCatalog(t)
	root := t.TempDir()
	writeImage(t, root, "plain.jpg")
	if _, err := c.scan(root); err != nil {
		t.Fatal(err)
	}
	call := func(handler http.Handler, method, path, body, origin, site string, cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "http://192.168.1.50:8765"+path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		if site != "" {
			request.Header.Set("Sec-Fetch-Site", site)
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	local, network := c.handler(), c.handlerForHost("0.0.0.0")
	if r := call(local, "GET", "/api/images", "", "", "", nil); r.Code != 403 {
		t.Fatal("default mode allowed a network host", r.Code)
	}
	response := call(network, "GET", "/api/images", "", "", "", nil)
	if response.Code != 200 {
		t.Fatal("network browse blocked", response.Code)
	}
	var listing Listing
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Total != 1 || listing.Root != "" {
		t.Fatal("public network browse exposed collection root")
	}
	body := `{"path":"plain.jpg","description":"Updated over network"}`
	if r := call(network, "POST", "/api/description", body, "http://192.168.1.50:8765", "same-origin", nil); r.Code != 401 {
		t.Fatal("network mutation allowed without login", r.Code)
	}
	login := call(network, "POST", "/api/login", `{"username":"`+adminUsername+`","password":"`+adminPassword+`"}`, "http://192.168.1.50:8765", "same-origin", nil)
	if login.Code != 200 || len(login.Result().Cookies()) != 1 {
		t.Fatal("network sign-in failed", login.Code)
	}
	cookie := login.Result().Cookies()[0]
	for _, scenario := range []struct{ origin, site string }{{"http://attacker.example", ""}, {"", "cross-site"}, {"http://192.168.1.50:9999", "same-origin"}} {
		if r := call(network, "POST", "/api/description", body, scenario.origin, scenario.site, cookie); r.Code != 403 {
			t.Fatal("cross-site or mismatched-origin mutation accepted", r.Code)
		}
	}
	if c.data.Images["plain.jpg"].Description != "" {
		t.Fatal("blocked request changed data")
	}
	if r := call(network, "POST", "/api/description", body, "http://192.168.1.50:8765", "same-origin", cookie); r.Code != 200 {
		t.Fatal("same-origin admin mutation blocked", r.Code, r.Body.String())
	}
	if c.data.Images["plain.jpg"].Description != "Updated over network" {
		t.Fatal("admin network edit not saved")
	}
}
