package httpapi

import (
	"net/url"
	"testing"
)

func TestSubscriptionUsesNodeTLSAndTransportAliases(t *testing.T) {
	for _, tc := range []struct{ config, key, want string }{
		{`{"server_port":443,"network":"tcp","tls":1,"tls_settings":{"server_name":"cert.example.test"}}`, "sni", "cert.example.test"},
		{`{"server_port":443,"network":"grpc","tls":1,"networkSettings":{"service_name":"service-one"}}`, "serviceName", "service-one"},
		{`{"server_port":443,"network":"ws","tls":1,"networkSettings":{"headers":{"Host":"front.example.test"},"path":"/ws"}}`, "host", "front.example.test"},
	} {
		link := vlessLink("00000000-0000-4000-8000-000000000001", "node", "192.0.2.1", []byte(tc.config))
		u, err := url.Parse(link)
		if err != nil {
			t.Fatal(err)
		}
		if got := u.Query().Get(tc.key); got != tc.want {
			t.Fatalf("%s=%q want %q", tc.key, got, tc.want)
		}
	}
}
