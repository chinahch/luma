export function connectionState(heartbeat:string|null|undefined,now=Date.now()):'pending'|'online'|'offline'{
 const at=heartbeat?Date.parse(heartbeat):NaN
 return !Number.isFinite(at)?'pending':now-at<=45000?'online':'offline'
}
export function cpuPercent(value:unknown):number|null{
 return typeof value==='number'&&Number.isFinite(value)&&value>=0?value:null
}
export function storagePercent(value:{total?:number,used?:number}|null|undefined):number|null{
 if(!value||typeof value.total!=='number'||typeof value.used!=='number'||!Number.isFinite(value.total)||!Number.isFinite(value.used)||value.total<=0||value.used<0)return null
 return value.used/value.total*100
}
