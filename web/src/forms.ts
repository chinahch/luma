export function initialExpiry(existing:string|null|undefined,editing:boolean,now=Date.now()):number|null {
 return existing?Date.parse(existing):editing?null:now+30*86400000
}
