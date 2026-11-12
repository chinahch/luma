// Immutable installer revision, updated with the matching native release.
export const installerRevision='a1c730a58c064434d8329f8948e16f28ab284312'
export function shellQuote(value:string){return "'"+value.replaceAll("'", "'\"'\"'")+"'"}
export function machineCommand(origin:string,id:number,action:'install'|'rebind'):string{
 const u=new URL(origin.trim())
 if(!['http:','https:'].includes(u.protocol)||u.username||u.password||u.search||u.hash||u.pathname!=='/'||!Number.isSafeInteger(id)||id<=0||!['install','rebind'].includes(action))throw new Error('请填写节点能访问的面板地址，不带路径、账号或查询参数。')
 const host=u.hostname.toLowerCase()
 if(!/^[a-z0-9.:[\]-]+$/i.test(host)||host==='localhost'||host.endsWith('.localhost')||host==='0.0.0.0'||host.startsWith('127.')||['[::]','[::1]'].includes(host))throw new Error('localhost 或回环地址无法供远程节点连接，请填写面板域名或可达 IP。')
 const script=`set -eu; tmp=$(mktemp); trap 'rm -f "$tmp"' EXIT; curl -fsSL ${shellQuote(`https://raw.githubusercontent.com/chinahch/caddy/${installerRevision}/install-v2.sh`)} -o "$tmp"; bash "$tmp" --mode machine --panel ${shellQuote(u.origin)} --machine-id ${id} --prompt-token${action==='rebind'?' --rebind':''}`
 return 'bash -c '+shellQuote(script)
}
