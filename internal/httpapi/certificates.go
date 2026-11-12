package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/chinahch/luma/internal/protocol"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func maskCertificateConfig(config map[string]any) {
	cc, ok := config["cert_config"].(map[string]any)
	if !ok {
		return
	}
	env, _ := cc["dns_env"].(map[string]any)
	key, _ := cc["key_content"].(string)
	if len(env) > 0 {
		cc["dns_env_saved"] = true
		cc["secret_action"] = "keep"
	}
	if key != "" {
		cc["key_content_saved"] = true
		cc["secret_action"] = "keep"
	}
	delete(cc, "dns_env")
	delete(cc, "key_content")
}

func resolveCertificateConfig(raw, old []byte) ([]byte, error) {
	var config map[string]any
	if json.Unmarshal(raw, &config) != nil || config == nil {
		return nil, errors.New("节点配置必须是 JSON 对象")
	}
	value, exists := config["cert_config"]
	if !exists {
		return raw, nil
	}
	cc, ok := value.(map[string]any)
	if !ok || cc == nil {
		return nil, errors.New("证书配置必须是 JSON 对象")
	}
	action, _ := cc["secret_action"].(string)
	if action != "" && action != "keep" && action != "replace" {
		return nil, errors.New("证书凭据操作无效")
	}
	if action == "keep" {
		var previous map[string]any
		json.Unmarshal(old, &previous)
		oldCC, _ := previous["cert_config"].(map[string]any)
		if oldCC == nil || oldCC["cert_mode"] != cc["cert_mode"] {
			return nil, errors.New("证书模式已变更，请重新填写对应凭据")
		}
		switch cc["cert_mode"] {
		case "dns":
			oldProvider, _ := oldCC["dns_provider"].(string)
			provider, _ := cc["dns_provider"].(string)
			op, oldOK := protocol.FindDNSProvider(oldProvider)
			np, newOK := protocol.FindDNSProvider(provider)
			if !oldOK || !newOK || op.Name != np.Name {
				return nil, errors.New("DNS 提供商已变更，请填写新提供商的完整凭据")
			}
			if _, provided := cc["dns_env"]; provided {
				return nil, errors.New("替换 DNS 凭据时请选择替换操作")
			}
			cc["dns_env"] = oldCC["dns_env"]
		case "content":
			if _, provided := cc["key_content"]; provided {
				return nil, errors.New("替换 PEM 私钥时请选择替换操作")
			}
			cc["key_content"] = oldCC["key_content"]
		default:
			return nil, errors.New("当前证书模式没有可保留的凭据")
		}
	}
	delete(cc, "secret_action")
	delete(cc, "dns_env_saved")
	delete(cc, "key_content_saved")
	if err := protocol.ValidateCertificate(config); err != nil {
		return nil, err
	}
	if cc["cert_mode"] == "dns" {
		provider, _ := cc["dns_provider"].(string)
		p, _ := protocol.FindDNSProvider(provider)
		cc["dns_provider"] = p.Name
	}
	return json.Marshal(config)
}

func (a *API) resolveNodeCertificate(c *gin.Context, tx pgx.Tx, id int64, raw, oldPublic []byte) ([]byte, bool) {
	var old []byte
	if id > 0 {
		var cipher []byte
		err := tx.QueryRow(c.Request.Context(), `SELECT encrypted_config FROM node_private WHERE node_id=$1`, id).Scan(&cipher)
		if err != nil && err != pgx.ErrNoRows {
			internal(c, err)
			return nil, false
		}
		var ok bool
		old, ok = a.openNodeConfig(c, id, oldPublic, cipher)
		if !ok {
			return nil, false
		}
	}
	resolved, err := resolveCertificateConfig(raw, old)
	if err != nil {
		fail(c, 400, err.Error())
		return nil, false
	}
	return resolved, true
}
