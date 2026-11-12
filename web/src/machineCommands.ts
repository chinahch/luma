// Immutable installer revision, updated with the matching native release.
export const installerRevision='bd0a796db6a43129dfdbce9adfe3336f77e47569'
export function shellQuote(value:string){return "'"+value.replaceAll("'", "'\"'\"'")+"'"}
export function machineCommand(origin:string,id:number,code:string):string{
 const u=new URL(origin.trim())
 if(!['http:','https:'].includes(u.protocol)||u.username||u.password||u.search||u.hash||u.pathname!=='/'||!Number.isSafeInteger(id)||id<=0)throw new Error('请填写节点能访问的面板地址，不带路径、账号或查询参数。')
 const host=u.hostname.toLowerCase().replace(/\.+$/,'')
 if(!/^[a-z0-9.:[\]-]+$/i.test(host)||host==='localhost'||host.endsWith('.localhost')||host==='0.0.0.0'||host.startsWith('127.')||/^\[::ffff:7f[0-9a-f]{2}:[0-9a-f]{1,4}\]$/.test(host)||['[::]','[::1]','[::ffff:0:0]'].includes(host))throw new Error('localhost 或回环地址无法供远程节点连接，请填写面板域名或可达 IP。')
 if(!/^[A-Za-z0-9_-]{43}$/.test(code))throw new Error('安装命令已失效，请重新生成。')
 const script=`set -eu; tmp=$(mktemp); trap 'rm -f "$tmp"' EXIT; curl -fsSL ${shellQuote(`https://raw.githubusercontent.com/chinahch/caddy/${installerRevision}/install-v2.sh`)} -o "$tmp"; bash "$tmp" --mode machine --machine-id ${id} --panel ${shellQuote(u.origin)} --enrollment-code ${shellQuote(code)}`
 return 'bash -c '+shellQuote(script)
}
