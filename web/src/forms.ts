export function initialExpiry(existing:string|null|undefined,editing:boolean,now=Date.now()):number|null {
 return existing?Date.parse(existing):editing?null:now+30*86400000
}

export interface RouteDraft {exit_id:number|null,domains:string,cidrs:string}
export function nodeRouting(defaultExit:number,rules:RouteDraft[]){
 if(!Number.isSafeInteger(defaultExit)||defaultExit<0)throw new Error('请选择默认出站。')
 const lines=(s:string)=>s.split(/[\n,]+/).map(x=>x.trim()).filter(Boolean)
 return {default_exit:defaultExit,rules:rules.map((r,i)=>{
  if(!Number.isSafeInteger(r.exit_id)||!r.exit_id||r.exit_id<0)throw new Error(`请为分流 ${i+1} 选择落地。`)
  const domain_suffixes=lines(r.domains),ip_cidrs=lines(r.cidrs)
  if(!domain_suffixes.length&&!ip_cidrs.length)throw new Error(`分流 ${i+1} 至少需要一个域名或 IP 网段。`)
  return {exit_id:r.exit_id,domain_suffixes,ip_cidrs}
 })}
}
