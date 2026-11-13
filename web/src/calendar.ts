// Match the expiry picker: arithmetic uses the browser's local calendar.
export function extendExpiry(expiry:number|null|undefined,months:number,now=Date.now()):number{
 const date=new Date(typeof expiry==='number'&&Number.isFinite(expiry)&&expiry>now?expiry:now)
 const day=date.getDate()
 date.setDate(1)
 date.setMonth(date.getMonth()+months)
 const lastDay=new Date(date.getFullYear(),date.getMonth()+1,0).getDate()
 date.setDate(Math.min(day,lastDay))
 return date.getTime()
}

export function calendarRange(timezone:string,days:number,offset=0,now=new Date()):{from:string;to:string}{
 const parts=new Intl.DateTimeFormat('en-US',{timeZone:timezone,year:'numeric',month:'2-digit',day:'2-digit'}).formatToParts(now)
 const part=(name:string)=>Number(parts.find(p=>p.type===name)?.value)
 const end=new Date(Date.UTC(part('year'),part('month')-1,part('day')-offset))
 const start=new Date(end);start.setUTCDate(start.getUTCDate()-days+1)
 return {from:start.toISOString().slice(0,10),to:end.toISOString().slice(0,10)}
}
