export class ApiError extends Error { constructor(public status:number,message:string){super(message)} }
export async function api<T=any>(path:string,options:RequestInit={}):Promise<T>{
 const response=await fetch(path,{...options,credentials:'same-origin',headers:{...(options.body?{'Content-Type':'application/json'}:{}),...options.headers}})
 const data=await response.json().catch(()=>({error:'服务返回了无效响应'}))
 if(!response.ok)throw new ApiError(response.status,data.error||'请求失败')
 return data as T
}
export function bytes(value:number|string|undefined):string{let n=Number(value||0);if(!Number.isFinite(n))return '—';if(n===0)return '0 B';const units=['B','KiB','MiB','GiB','TiB','PiB'];let i=0;while(n>=1024&&i<units.length-1){n/=1024;i++}return `${n.toLocaleString('zh-CN',{maximumFractionDigits:i?2:0})} ${units[i]}`}
export function stamp(value:string|null|undefined):string{return value?new Date(value).toLocaleString('zh-CN',{hour12:false}):'尚无上报'}
