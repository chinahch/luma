export type CertificateDraft={mode:string;domain:string;email:string;provider:string;sni:string;envText:string;certFile:string;keyFile:string;certContent:string;keyContent:string;savedEnv:boolean;savedKey:boolean;originalProvider:string;originalMode:string}
type Config=Record<string,any>
export function certificateDraft(config:Config):CertificateDraft{
 const c=config.cert_config||{},sni=config.tls_settings?.server_name||config.server_name||c.domain||''
 return {mode:c.cert_mode||c.mode||'dns',domain:c.domain||sni,email:c.email||'',provider:c.dns_provider||'cloudflare',sni,envText:Object.entries(c.dns_env||{}).map(([key,value])=>`${key}=${value}`).join('\n'),certFile:c.cert_file||'',keyFile:c.key_file||'',certContent:c.cert_content||'',keyContent:c.key_content||'',savedEnv:!!c.dns_env_saved,savedKey:!!c.key_content_saved,originalProvider:c.dns_provider||'cloudflare',originalMode:c.cert_mode||c.mode||'dns'}
}
export function parseDNSEnv(text:string):Record<string,string>{
 if(new TextEncoder().encode(text).length>65536)throw new Error('DNS 凭据不能超过 64 KiB')
 const env:Record<string,string>={}
 for(const line of text.replaceAll('\r\n','\n').split('\n')){if(!line.trim())continue;const at=line.indexOf('='),key=line.slice(0,at).trim(),value=line.slice(at+1)
  if(at<1||!/^[A-Z_][A-Z0-9_]{0,127}$/.test(key)||!value.trim()||/[\r\n\0]/.test(value))throw new Error('DNS 凭据需要每行 KEY=VALUE，不要包含 export')
  if(Object.hasOwn(env,key))throw new Error('DNS 凭据变量名重复');env[key]=value
 }
 if(Object.keys(env).length>64)throw new Error('DNS 凭据不能超过 64 项')
 return env
}
export function applyCertificateToConfig(config:Config,d:CertificateDraft):Config{
 const result=structuredClone(config)
 if(Number(config.tls)!==1)return result
 const c:Config={cert_mode:d.mode,domain:d.domain.trim()}
 result.server_name=d.sni.trim();result.tls_settings={...(result.tls_settings||{}),server_name:result.server_name}
 if(['dns','http'].includes(d.mode))c.email=d.email.trim()
 if(d.mode==='dns'){
  c.dns_provider=d.provider
  if(d.savedEnv&&!d.envText.trim()){
   if(d.originalMode!=='dns'||d.originalProvider!==d.provider)throw new Error('更换 DNS 提供商后，请填写新提供商的完整凭据')
   c.secret_action='keep'
  }else {c.dns_env=parseDNSEnv(d.envText);c.secret_action='replace'}
 }else if(d.mode==='file'){c.cert_file=d.certFile.trim();c.key_file=d.keyFile.trim()}
 else if(d.mode==='content'){
  c.cert_content=d.certContent
  if(d.savedKey&&!d.keyContent.trim()&&d.originalMode==='content')c.secret_action='keep'
  else {c.key_content=d.keyContent;c.secret_action='replace'}
 }
 result.cert_config=c
 return result
}
export function presetDraft(config:Config){
 let preset:string|null=config.preset||null
 const protocol=config.protocol,network=config.network,tls=Number(config.tls),enc=!!config.decryption
 if(!preset){
  if(config.connector?.kind==='mtg')preset='mtg';else if(config.connector?.kind==='cloudflared')preset='cf-tunnel'
  else if(protocol==='socks')preset='socks5'
  else if(protocol==='hysteria'||protocol==='hysteria2')preset='hysteria2'
  else if(protocol==='shadowsocks')preset=String(config.cipher||'').startsWith('2022-')?'ss2022':'shadowsocks'
  else if(protocol==='anytls')preset=tls===2?'anytls-reality':tls===1?'anytls-tls':null
  else if(protocol==='vmess'&&network==='ws'&&tls===1)preset='vmess-ws'
  else if(protocol==='vless'){
   if(network==='ws'&&tls===1)preset='vless-ws'
   else if(network==='xhttp'&&tls===2)preset=enc?'vless-xhttp-reality-enc':'vless-xhttp-reality'
   else if(network==='xhttp'&&tls===1&&config.networkSettings?.alpn==='h3')preset='vless-xhttp-tls-h3'
   else if(tls===2)preset=enc?'vless-enc-reality-vision':'vless-reality'
   else if(enc)preset='vless-enc'
  }
 }
 let auth='x25519',appearance='native'
 if(enc){const parts=String(config.decryption).split('.');if(['native','xorpub','random'].includes(parts[1]))appearance=parts[1];try{if(atob((parts.at(-1)||'').replaceAll('-','+').replaceAll('_','/')).length===64)auth='mlkem768'}catch{}}
 return {preset,port:config.server_port||443,server_name:config.server_name||config.tls_settings?.server_name||config.cert_config?.domain||config.connector?.hostname||'',destination:config.tls_settings?.dest||'',path:config.networkSettings?.path||'',choice:`${auth}:${appearance}`,metrics_port:config.connector?.metrics_port||21001,tunnel_token:'',owner_id:config.connector?.user_id||null}
}
export function newPresetDraft(){return {...presetDraft({}),preset:'vless-reality',port:25000,server_name:'addons.mozilla.org',destination:'addons.mozilla.org:443'}}
export function applyPresetDraft(config:Config,d:ReturnType<typeof presetDraft>):Config{
 const original=presetDraft(config),result=structuredClone(config)
 if(!d.preset)return result
 if(d.preset!==original.preset)throw new Error('切换协议后请先点击一键生成配置')
 if(config.decryption&&d.choice!==original.choice)throw new Error('更换 ENC 认证/外观后请重新生成密钥')
 result.server_port=d.port
 if(Number(config.tls)===2)result.tls_settings={...config.tls_settings,server_name:d.server_name,dest:d.destination||`${d.server_name}:443`}
 else if(Number(config.tls)===1){result.server_name=d.server_name;result.tls_settings={...config.tls_settings,server_name:d.server_name}}
 if(['ws','xhttp'].includes(config.network))result.networkSettings={...config.networkSettings,path:d.path}
 if(config.connector){
  result.connector={...config.connector,metrics_port:d.metrics_port}
  if(config.connector.kind==='cloudflared'){result.connector.hostname=d.server_name;if(d.tunnel_token)result.connector.token=d.tunnel_token}
  if(config.connector.kind==='mtg'){
   if(d.owner_id!==original.owner_id)throw new Error('MTProxy 归属用户不可变更，请为其他用户新建节点')
   if(d.server_name!==original.server_name)throw new Error('更换 MTProxy 伪装域名后请重新生成配置与密钥')
  }
 }
 return result
}
export function credentialRequirement(provider:any):string{
 const group=(keys:string[])=>keys.join(' 或 '),set=(groups:string[][])=>groups.map(group).join(' + ')
 if(provider?.credential_sets?.length)return provider.credential_sets.map((groups:string[][])=>'（'+set(groups)+'）').join(' 或 ')
 return provider?.required?.length?provider.required.map(group).join('；'):'可使用节点机已配置的云身份'
}
export function publicHostForConfig(config:Config,current:string):string{return config.connector?.kind==='cloudflared'?config.connector.hostname||'':current}
