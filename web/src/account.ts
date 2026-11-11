import {ref,shallowRef} from 'vue'
export function createAccountState<T>(){
 const user=shallowRef<T|null>(null),timezone=ref('UTC')
 async function refresh(read:()=>Promise<{user:T;timezone:string}>){const data=await read();user.value=data.user;timezone.value=data.timezone}
 return {user,timezone,refresh}
}
