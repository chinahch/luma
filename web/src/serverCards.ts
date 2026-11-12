import type {Probe} from './featureState'
import {probeLabel} from './featureState.ts'
export function serverCards(machines:any[],nodes:any[],order:number[]){
 const cards=machines.map(machine=>{const owned=nodes.filter(n=>n.machine_id===machine.id);return {...machine,nodes:owned,enabled:owned.filter(n=>n.active).length}})
 const position=new Map(order.map((id,index)=>[id,index]));cards.sort((a,b)=>(position.get(a.id)??100000+a.id)-(position.get(b.id)??100000+b.id))
 const missing=nodes.filter(n=>!machines.some(m=>m.id===n.machine_id))
 if(missing.length)cards.push({id:0,name:'待分配服务器',nodes:missing,enabled:missing.filter(n=>n.active).length,country:''})
 return cards
}
export function moveCard(order:number[],from:number,to:number){if(from===to||!order.includes(from)||!order.includes(to))return [...order];const result=order.filter(id=>id!==from);result.splice(result.indexOf(to),0,from);return result}
export function countryLabel(code:string){if(!/^[A-Z]{2}$/.test(code)||code==='ZZ')return '国家未知';try{const country=new Intl.DisplayNames(['zh-CN'],{type:'region',fallback:'none'}).of(code);if(!country)return '国家未知';return String.fromCodePoint(...[...code].map(c=>127397+c.charCodeAt(0)))+' '+country}catch{return '国家未知'}}
export function automaticHealthView(probe:Probe|null|undefined,now=Date.now()):{text:string;tone:'success'|'error'|'warning'|'default'}{
 if(!probe)return {text:'等待服务器自测',tone:'default'}
 const at=Date.parse(probe.finished_at||probe.created_at||'')
 if(Number.isFinite(at)&&now-at>10*60000)return {text:'结果已过期',tone:'warning'}
 return {text:probeLabel(probe),tone:probe.state==='succeeded'?'success':probe.state==='failed'?'error':'default'}
}
