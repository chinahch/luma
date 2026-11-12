package httpapi

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type diagnosticFinding struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}
type diagnosticLog struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Count   int    `json:"count"`
	Scope   string `json:"scope"`
}
type diagnosticResult struct {
	Version   int                 `json:"version"`
	NodeID    int64               `json:"node_id"`
	Port      int                 `json:"expected_port"`
	Transport string              `json:"transport"`
	Listening *bool               `json:"listening"`
	Users     int                 `json:"eligible_users"`
	Runtime   string              `json:"runtime_state"`
	Findings  []diagnosticFinding `json:"findings"`
	Logs      []diagnosticLog     `json:"logs"`
}

var diagnosticMessages = map[string]string{
	"no_users":         "No eligible users; the native kernel intentionally does not open a listener.",
	"listener_missing": "The expected local port is not listening.",
	"listener_present": "The expected local port is bound; this does not verify authentication or external reachability.",
	"listener_unknown": "Local socket inspection is unavailable on this operating system.",
	"runtime_stopped":  "The assigned native kernel is stopped.", "runtime_exited": "The assigned node service has exited.", "runtime_unassigned": "The machine has no running service handle for this node.",
	"port_in_use":      "Machine logs contain an address-already-in-use error; it may concern another node.",
	"auth_failure":     "Machine logs contain an authentication failure; it may concern another node.",
	"config_rejected":  "Machine logs contain a configuration or kernel-start rejection; it may concern another node.",
	"certificate":      "Machine logs contain a certificate or TLS handshake error; it may concern another node.",
	"logs_unavailable": "Recent machine service logs were unavailable.", "machine_log_evidence": "Log classifications are machine-level evidence and are not attributed to this node.",
	"node_disabled": "This logical node is disabled in the panel.", "connector_only": "MTProxy uses an independent connector; native runtime status does not describe its service.",
}

func sanitizeDiagnostic(raw json.RawMessage, node int64) (json.RawMessage, bool) {
	var d diagnosticResult
	if len(raw) > 32768 || json.Unmarshal(raw, &d) != nil || d.Version != 1 || d.NodeID != node || d.Port < 1 || d.Port > 65535 || (d.Transport != "tcp" && d.Transport != "udp") || d.Users < 0 || d.Users > 10000000 || len(d.Findings) > 16 || len(d.Logs) > 8 {
		return nil, false
	}
	switch d.Runtime {
	case "running", "stopped", "exited", "unassigned", "unsupported":
	default:
		return nil, false
	}
	for i := range d.Findings {
		f := &d.Findings[i]
		msg, ok := diagnosticMessages[f.Code]
		if !ok {
			return nil, false
		}
		f.Message = msg
		switch f.Severity {
		case "info", "warning", "error":
		default:
			return nil, false
		}
	}
	for i := range d.Logs {
		l := &d.Logs[i]
		msg, ok := diagnosticMessages[l.Code]
		if !ok || l.Count < 1 || l.Count > 120 {
			return nil, false
		}
		l.Message = msg
		l.Scope = "machine"
	}
	if d.Findings == nil {
		d.Findings = []diagnosticFinding{}
	}
	if d.Logs == nil {
		d.Logs = []diagnosticLog{}
	}
	b, e := json.Marshal(d)
	return b, e == nil
}
func (a *API) createDiagnostic(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var in struct{}
	if !decode(c, &in) {
		return
	}
	var mid *int64
	var cfg []byte
	var active bool
	e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT machine_id,config,active FROM nodes WHERE id=$1`, id).Scan(&mid, &cfg, &active)
	if e == pgx.ErrNoRows {
		fail(c, 404, "node not found")
		return
	}
	if e != nil {
		internal(c, e)
		return
	}
	if mid == nil {
		fail(c, 409, "node has no machine")
		return
	}
	cfg, ok = a.fullNodeConfig(c, id, cfg)
	if !ok {
		return
	}
	var v struct {
		Port            int    `json:"server_port"`
		Protocol        string `json:"protocol"`
		Network         string `json:"network"`
		TLS             int    `json:"tls"`
		NetworkSettings struct {
			ALPN string `json:"alpn"`
		} `json:"networkSettings"`
	}
	if json.Unmarshal(cfg, &v) != nil || v.Port < 1 || v.Port > 65535 {
		fail(c, 409, "invalid stored node port")
		return
	}
	transport := "tcp"
	if v.Protocol == "hysteria" || v.Protocol == "hysteria2" || v.Protocol == "tuic" || v.Protocol == "wireguard" || (v.Network == "xhttp" && v.TLS == 1 && v.NetworkSettings.ALPN == "h3") {
		transport = "udp"
	}
	var users int
	e = a.s.Pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM users u CROSS JOIN nodes n WHERE n.id=$1 AND `+entitlementSQL, id).Scan(&users)
	if e != nil {
		internal(c, e)
		return
	}
	payload := probePayload{Action: "diagnostic", NodeID: id, Port: v.Port, Transport: transport, Users: users, Active: active, Protocol: v.Protocol}
	a.enqueueProbe(c, *mid, &id, "diagnostic", payload, "")
}
