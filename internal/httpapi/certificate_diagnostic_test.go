package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCertificateDiagnosticStatusIsFixedAndSanitized(t *testing.T) {
	raw := json.RawMessage(`{"version":1,"node_id":1,"expected_port":443,"transport":"tcp","eligible_users":1,"runtime_state":"stopped","findings":[{"code":"certificate_failed","message":"sentinel-private-credential","severity":"error"}],"logs":[]}`)
	clean, ok := sanitizeDiagnostic(raw, 1)
	if !ok || strings.Contains(string(clean), "sentinel-private-credential") {
		t.Fatal("certificate status not sanitized", string(clean))
	}
}
