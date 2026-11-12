import {test} from 'node:test'
import assert from 'node:assert/strict'
import {certificateDraft,applyCertificateToConfig,parseDNSEnv,presetDraft,applyPresetDraft,newPresetDraft,publicHostForConfig} from './certificates.ts'

test('legacy file certificate hydrates and certificate-only edits preserve protocol keys',()=>{
 const config={preset:'hysteria2',protocol:'hysteria',version:2,tls:1,server_port:25000,server_name:'edge.example.com',server_key:'keep-key',cert_config:{cert_mode:'file',domain:'edge.example.com',cert_file:'/a/cert',key_file:'/a/key'}}
 const draft=certificateDraft(config)
 assert.equal(draft.mode,'file');assert.equal(draft.certFile,'/a/cert')
 draft.mode='dns';draft.email='admin@example.com';draft.envText='CF_API_TOKEN=token=='
 const next=applyCertificateToConfig(config,draft)
 assert.equal(next.server_key,'keep-key');assert.equal(next.server_port,25000);assert.equal(next.preset,'hysteria2')
 assert.deepEqual(next.cert_config.dns_env,{CF_API_TOKEN:'token=='})
 assert.equal(next.cert_config.cert_file,undefined);assert.equal(config.cert_config.cert_mode,'file')
})
test('saved credentials are retained only for the same mode and provider',()=>{
 const config={tls:1,server_name:'edge.example.com',cert_config:{cert_mode:'dns',domain:'edge.example.com',email:'admin@example.com',dns_provider:'cloudflare',dns_env_saved:true,secret_action:'keep'}}
 const draft=certificateDraft(config)
 assert.equal(applyCertificateToConfig(config,draft).cert_config.secret_action,'keep')
 draft.provider='alidns'
 assert.throws(()=>applyCertificateToConfig(config,draft),/新提供商|凭据/)
 draft.envText='ALICLOUD_ACCESS_KEY_ID=id\nALICLOUD_ACCESS_KEY_SECRET=secret'
 const cc=applyCertificateToConfig(config,draft).cert_config
 assert.equal(cc.secret_action,'replace');assert.equal(cc.dns_env.CF_API_TOKEN,undefined)
 draft.mode='http'
 const http=applyCertificateToConfig(config,draft).cert_config
 assert.equal(http.dns_env,undefined);assert.equal(http.dns_provider,undefined)
})
test('KEY=VALUE uses first equals, rejects duplicate names, treats shell syntax as data',()=>{
 assert.deepEqual(parseDNSEnv('CF_API_TOKEN=a==\r\n'),{CF_API_TOKEN:'a=='})
 for(const text of ['X=a\nX=b','export X=y','bad-name=value','missing','X=']) assert.throws(()=>parseDNSEnv(text))
 assert.equal(parseDNSEnv('X=$(touch never)').X,'$(touch never)')
})
test('new TLS config defaults to DNS and concrete SNI is separate from wildcard certificate',()=>{
 const config={tls:1,server_name:'edge.example.com'};const draft=certificateDraft(config)
 assert.equal(draft.mode,'dns');assert.equal(draft.domain,'edge.example.com')
 draft.domain='*.example.com';draft.email='a@example.com';draft.envText='CF_API_TOKEN=test'
 const result=applyCertificateToConfig(config,draft)
 assert.equal(result.server_name,'edge.example.com');assert.equal(result.cert_config.domain,'*.example.com')
})
test('editing presets uses actual protocol, port, SNI and path instead of default REALITY',()=>{
 const draft=presetDraft({preset:'hysteria2',protocol:'hysteria',server_port:25000,server_name:'edge.example.com',tls:1})
 assert.equal(draft.preset,'hysteria2');assert.equal(draft.port,25000);assert.equal(draft.server_name,'edge.example.com')
 const legacy=presetDraft({protocol:'vmess',server_port:25555,tls:1,network:'ws',networkSettings:{path:'/old'},tls_settings:{server_name:'ws.example.com'}})
 assert.equal(legacy.preset,'vmess-ws');assert.equal(legacy.path,'/old');assert.equal(legacy.server_name,'ws.example.com')
 assert.equal(presetDraft({protocol:'unknown',server_port:443}).preset,null)
 const enc=presetDraft({preset:'vless-enc',decryption:'mlkem768x25519plus.random.600s.secret'})
 assert.equal(enc.choice,'x25519:random')
})

test('one-click defaults use requested REALITY target and safe edits preserve existing keys',()=>{
 const fresh=newPresetDraft();assert.equal(fresh.server_name,'addons.mozilla.org');assert.equal(fresh.destination,'addons.mozilla.org:443')
 const config={preset:'vless-reality',protocol:'vless',server_port:24443,tls:2,tls_settings:{server_name:'old.example.com',dest:'old.example.com:443',private_key:'keep-private',public_key:'keep-public',short_id:'keep-short'}}
 const d=presetDraft(config);d.port=25555;d.server_name='addons.mozilla.org';d.destination='addons.mozilla.org:443'
 const result=applyPresetDraft(config,d)
 assert.equal(result.server_port,25555);assert.equal(result.tls_settings.server_name,'addons.mozilla.org')
 assert.equal(result.tls_settings.private_key,'keep-private');assert.equal(result.tls_settings.short_id,'keep-short')
 d.preset='hysteria2';assert.throws(()=>applyPresetDraft(config,d),/生成/)
})
test('connector edits save CF hostname and reject immutable MTG owner changes',()=>{
 const cf={preset:'cf-tunnel',protocol:'vless',tls:0,server_port:18001,network:'ws',connector:{kind:'cloudflared',hostname:'old.example.com',token:'keep-token',metrics_port:21001}}
 const draft=presetDraft(cf);draft.server_name='new.example.com'
 assert.equal(applyPresetDraft(cf,draft).connector.hostname,'new.example.com')
 assert.equal(applyPresetDraft(cf,draft).connector.token,'keep-token')
 assert.equal(publicHostForConfig(applyPresetDraft(cf,draft),'8.8.8.8'),'new.example.com')
 const mtg={preset:'mtg',protocol:'mtproxy',server_port:443,connector:{kind:'mtg',user_id:7,secret:'keep-secret',metrics_port:21002}}
 const owner=presetDraft(mtg);owner.owner_id=8
 assert.throws(()=>applyPresetDraft(mtg,owner),/归属|用户/)
})
