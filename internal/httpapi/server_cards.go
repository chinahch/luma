package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/language"
	"io"
	"net/http"
	"net/netip"
	"regexp"
	"time"
)

func (a *API) machineLayout(c *gin.Context) {
	var raw []byte
	err := a.s.Pool.QueryRow(c.Request.Context(), `SELECT machine_ids FROM machine_card_layouts WHERE user_id=$1`, current(c).ID).Scan(&raw)
	if err == pgx.ErrNoRows {
		raw = []byte("[]")
	} else if err != nil {
		internal(c, err)
		return
	}
	c.Data(200, "application/json", raw)
}
func (a *API) saveMachineLayout(c *gin.Context) {
	var in struct {
		IDs []int64 `json:"machine_ids"`
	}
	if !decode(c, &in) {
		return
	}
	if len(in.IDs) > 10000 {
		fail(c, 400, "too many machine cards")
		return
	}
	seen := map[int64]bool{}
	for _, id := range in.IDs {
		if id <= 0 || seen[id] {
			fail(c, 400, "invalid or duplicate machine card")
			return
		}
		seen[id] = true
	}
	var count int
	if err := a.s.Pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM machines WHERE id=ANY($1::bigint[])`, in.IDs).Scan(&count); err != nil {
		internal(c, err)
		return
	}
	if count != len(in.IDs) {
		fail(c, 400, "machine no longer exists; refresh cards")
		return
	}
	if in.IDs == nil {
		in.IDs = []int64{}
	}
	raw, _ := json.Marshal(in.IDs)
	if _, err := a.s.Pool.Exec(c.Request.Context(), `INSERT INTO machine_card_layouts(user_id,machine_ids) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET machine_ids=excluded.machine_ids,updated_at=now()`, current(c).ID, raw); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

func parseCountryResponse(raw []byte, ip string) (string, error) {
	var data struct {
		IP      string `json:"ip"`
		Country string `json:"country"`
	}
	if json.Unmarshal(raw, &data) != nil || !countryCode.MatchString(data.Country) {
		return "", errors.New("invalid country result")
	}
	requested, e := netip.ParseAddr(ip)
	got, ge := netip.ParseAddr(data.IP)
	if e != nil || ge != nil || requested.Unmap() != got.Unmap() {
		return "", errors.New("country result IP mismatch")
	}
	if _, e = language.ParseRegion(data.Country); e != nil || data.Country == "ZZ" {
		return "", errors.New("unknown country")
	}
	return data.Country, nil
}

// Lookup uses the explicit physical IP, never the caller's egress address.
// Only that public address is sent to the fixed HTTPS country.is endpoint.
func (a *API) refreshMachineCountry(ctx context.Context, id int64) {
	var raw []byte
	if err := a.s.Pool.QueryRow(ctx, `SELECT status->'public_addresses' FROM machines WHERE id=$1`, id).Scan(&raw); err != nil || len(raw) == 0 {
		return
	}
	var v struct {
		Source string    `json:"source"`
		IPv4   []string  `json:"ipv4"`
		IPv6   []string  `json:"ipv6"`
		At     time.Time `json:"detected_at"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Source != "physical_interface" || time.Since(v.At) > 2*time.Minute || time.Until(v.At) > 30*time.Second {
		return
	}
	ip := ""
	for _, candidate := range append(v.IPv4, v.IPv6...) {
		addr, e := netip.ParseAddr(candidate)
		if e == nil && addr.IsGlobalUnicast() && !addr.IsPrivate() && !reservedPublicIP(addr) {
			ip = addr.Unmap().String()
			break
		}
	}
	if ip == "" {
		return
	}
	// Claim the refresh atomically; retries and multiple panel processes reuse it.
	var claimed int64
	err := a.s.Pool.QueryRow(ctx, `INSERT INTO machine_countries(machine_id,ip) VALUES($1,$2::inet) ON CONFLICT(machine_id) DO UPDATE SET ip=excluded.ip,country=CASE WHEN machine_countries.ip=excluded.ip THEN machine_countries.country ELSE '' END,checked_at=now() WHERE machine_countries.ip<>excluded.ip OR machine_countries.checked_at<now()-interval '24 hours' RETURNING machine_id`, id, ip).Scan(&claimed)
	if err != nil {
		return
	}
	lookup, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(lookup, "GET", "https://api.country.is/"+ip, nil)
	if err != nil {
		return
	}
	client := a.countryHTTP
	if client == nil {
		client = &http.Client{Timeout: 1500 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(req)
	country := ""
	if err == nil {
		body, e := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		if response.StatusCode == 200 && e == nil {
			country, _ = parseCountryResponse(body, ip)
		}
	}
	if country != "" {
		_, _ = a.s.Pool.Exec(ctx, `UPDATE machine_countries SET country=$2 WHERE machine_id=$1 AND ip=$3::inet`, id, country, ip)
	} else {
		_, _ = a.s.Pool.Exec(ctx, `UPDATE machine_countries SET country='',checked_at=now()-interval '23 hours' WHERE machine_id=$1 AND ip=$2::inet`, id, ip)
	}
}
