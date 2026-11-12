package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type countryTestTransport struct {
	calls int
	t     *testing.T
}

type failingCountryTransport struct{}

func (failingCountryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
}
func TestExpiredCountryIsUnknownAfterFailedRefresh(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	var id int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash,status) VALUES('country-expired',$1,jsonb_build_object('public_addresses',jsonb_build_object('source','physical_interface','ipv4',jsonb_build_array('8.8.8.8'),'detected_at',now()))) RETURNING id`, []byte("local-only")).Scan(&id)
	s.Pool.Exec(ctx, `INSERT INTO machine_countries(machine_id,ip,country,checked_at) VALUES($1,'8.8.8.8','US',now()-interval '2 days')`, id)
	a := &API{s: s, countryHTTP: &http.Client{Transport: failingCountryTransport{}}}
	a.refreshMachineCountry(ctx, id)
	var country string
	s.Pool.QueryRow(ctx, `SELECT country FROM machine_countries WHERE machine_id=$1`, id).Scan(&country)
	if country != "" {
		t.Fatal("expired country retained after failed refresh", country)
	}
}

func (t *countryTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.calls++
	if r.URL.String() != "https://api.country.is/8.8.8.8" {
		t.t.Fatal("lookup used egress instead of physical IP", r.URL.String())
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ip":"8.8.8.8","country":"US"}`)), Header: make(http.Header)}, nil
}
func TestPhysicalCountryLookupIsCachedAndDoesNotUseWARPAddress(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	var id int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash,status) VALUES('country-cache',$1,jsonb_build_object('public_addresses',jsonb_build_object('source','physical_interface','ipv4',jsonb_build_array('8.8.8.8'),'detected_at',now()))) RETURNING id`, []byte("local-only")).Scan(&id); err != nil {
		t.Fatal(err)
	}
	tr := &countryTestTransport{t: t}
	a := &API{s: s, countryHTTP: &http.Client{Transport: tr}}
	a.refreshMachineCountry(ctx, id)
	a.refreshMachineCountry(ctx, id)
	var country string
	if err := s.Pool.QueryRow(ctx, `SELECT country FROM machine_countries WHERE machine_id=$1`, id).Scan(&country); err != nil || country != "US" || tr.calls != 1 {
		t.Fatal("country not cached", country, tr.calls, err)
	}
	s.Pool.Exec(ctx, `UPDATE machines SET status=jsonb_build_object('public_addresses',jsonb_build_object('source','warp','ipv4',jsonb_build_array('1.1.1.1'),'detected_at',now())) WHERE id=$1`, id)
	a.refreshMachineCountry(ctx, id)
	if tr.calls != 1 {
		t.Fatal("WARP address sent for country lookup")
	}
}
