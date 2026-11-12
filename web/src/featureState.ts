export interface SubscriptionLink {path:string;needs_recovery:boolean}
export function subscriptionState(){
 return {path:'',needsRecovery:false,async load(read:()=>Promise<SubscriptionLink>){const data=await read();this.path=data.path||'';this.needsRecovery=data.needs_recovery}}
}
export function recoveryToken(value:string){
 const input=value.trim()
 if(!input)throw new Error('请粘贴原订阅链接或令牌。')
 let token=input
 if(input.includes('://')||input.startsWith('/')){
  let url:URL;try{url=new URL(input,'https://luma.invalid')}catch{throw new Error('订阅链接格式无效。')}
  const match=url.pathname.match(/^\/sub\/([A-Za-z0-9_-]+)$/)
  if(!match)throw new Error('请粘贴完整的原订阅链接。')
  token=match[1]!
 }
 if(!/^[A-Za-z0-9_-]+$/.test(token))throw new Error('订阅令牌格式无效。')
 return token
}
export function planIDs(ids:number[]){
 if(ids.some(id=>!Number.isSafeInteger(id)||id<=0))throw new Error('套餐选择无效，请刷新后重试。')
 return [...new Set(ids)]
}
export async function toggleActive(row:{active:boolean},active:boolean,write:(active:boolean)=>Promise<unknown>){
 const before=row.active;row.active=active
 try{await write(active)}catch(e){row.active=before;throw e}
}
export interface Probe {id?:string;state:string;latency_ms?:number|null;error_code?:string;created_at?:string;finished_at?:string|null;source?:string;ping_ms?:number|null;ping_error_code?:string}
export function probePending(probe?:Pick<Probe,'state'>|null){return probe?.state==='queued'||probe?.state==='running'}
export function probeLabel(probe?:Probe|null){
 if(!probe)return '尚未检测'
 if(probe.error_code==='machine_unavailable')return '服务器未连接'
 if(probe.state==='succeeded'){if(probe.ping_ms!=null)return `连通正常 · Ping ${probe.ping_ms.toFixed(2)} ms`;if(probe.ping_error_code==='icmp_timeout')return '代理连通 · Ping 无响应';if(probe.ping_error_code)return '代理连通 · Ping 不可用';return '连通正常'}
 return ({queued:'排队中',running:'检测中',failed:'请求失败',unsupported:'不支持检测',expired:'任务过期'} as Record<string,string>)[probe.state]||'未知状态'
}
export function inviteCode(search:string){return new URLSearchParams(search).get('invite')?.trim()||null}
export function cycleStamp(value:string,zone:string){return new Date(value).toLocaleString('zh-CN',{hour12:false,timeZone:zone})}
export interface NodeDiagnostics {version:number;node_id:number;expected_port:number;transport:string;listening:boolean|null;eligible_users:number;runtime_state:string;findings:{code:string;message:string;severity:string}[];logs:{code:string;message:string;count:number;scope:'machine'}[]}
export interface DiagnosticJob extends Probe {diagnostics?:NodeDiagnostics|null}
export function diagnosticSummary(job?:DiagnosticJob|null):{tone:'info'|'warning'|'error';title:string;detail:string}{
 if(!job)return {tone:'info',title:'尚未查询',detail:'从节点机读取受限诊断摘要，查询不会修改节点配置或重启服务。'}
 if(job.state==='queued')return {tone:'info',title:'等待节点机领取',detail:'节点机离线或机器认证无效时，面板无法远程获取日志；任务可能过期。'}
 if(job.state==='running')return {tone:'info',title:'节点机正在诊断',detail:'正在检查运行状态、监听端口和符合条件的用户，并整理脱敏摘要。'}
 if(job.state==='expired')return {tone:'warning',title:'诊断任务已过期',detail:'未在期限内收到节点机结果。请检查原生服务、机器认证和网络连接后重试。'}
 if(job.error_code==='machine_unavailable')return {tone:'warning',title:'服务器未连接',detail:'未收到服务器近期心跳或检测轮询，当前无法获取远程日志。请检查机器认证、原生服务和网络连接。'}
 if(job.state==='unsupported'||job.error_code==='upgrade_required')return {tone:'warning',title:'节点版本不支持诊断',detail:'请升级该服务器上的原生服务，再重新查询。当前无法获取节点机诊断日志。'}
 if(job.state==='failed')return {tone:'error',title:'诊断未完成',detail:'未能获取完整的节点机诊断结果。请检查下方错误信息、机器认证和原生服务状态。'}
 const d=job.diagnostics
 if(!d)return {tone:'warning',title:'未收到诊断资料',detail:'任务已结束，但没有可分析的节点机诊断结果。'}
 if(d.listening===false)return {tone:'warning',title:'端口未监听',detail:d.eligible_users===0?'没有符合使用条件的用户时，原生内核可能保持空闲。请检查账号启用、套餐、额度和到期时间。':'已有符合使用条件的用户，但未确认到配置端口的监听。请结合运行状态和下方发现检查配置、内核启动及端口冲突。'}
 if(d.listening===true)return {tone:'info',title:'已确认监听',detail:'节点机确认配置端口存在监听；这不代表认证、TLS 或公网代理请求一定成功，请另做连通性检测。'}
 return {tone:'warning',title:'监听状态未确认',detail:'本次诊断未确认监听状态，请结合运行状态、传输协议和节点机发现进一步排查。'}
}
export function machinePublicAddresses(status:{public_addresses?:{ipv4?:unknown;ipv6?:unknown;source?:string;detected_at?:string}}|null|undefined):string[]{
 const addresses=status?.public_addresses
 if(addresses?.source!=='physical_interface')return []
 const ipv4=Array.isArray(addresses.ipv4)?addresses.ipv4.filter((ip):ip is string=>typeof ip==='string'&&!!ip):[]
 const ipv6=Array.isArray(addresses.ipv6)?addresses.ipv6.filter((ip):ip is string=>typeof ip==='string'&&!!ip):[]
 return [...new Set([...ipv4,...ipv6])]
}
export function autofillPublicHost(current:string,previousAuto:string,candidate:string){return !current.trim()||(!!previousAuto&&current===previousAuto)?candidate:current}

export function automaticMachineAddress(status:Parameters<typeof machinePublicAddresses>[0],now=Date.now()):string{
 const at=Date.parse(status?.public_addresses?.detected_at||''),age=now-at
 if(!Number.isFinite(at)||age>120000||age< -30000)return ''
 return machinePublicAddresses(status)[0]||''
}
