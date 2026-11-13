export function reconcileRenewalKey(key:string,rows:{request_id:string;status?:string}[]):string{
 return rows.some(row=>row.request_id===key)?'':key
}
export function assertPendingRenewal(response:{status:string}):void{
 if(response.status!=='pending')throw new Error('此申请已处理，请刷新后重新申请。')
}
