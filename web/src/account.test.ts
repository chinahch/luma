import test from 'node:test'
import assert from 'node:assert/strict'
import { createAccountState } from './account.ts'
test('a refresh replaces the shared quota snapshot with current server values',async()=>{
 const state=createAccountState<{used_bytes:number;quota_bytes:number}>()
 await state.refresh(async()=>({user:{used_bytes:100,quota_bytes:500},timezone:'UTC'}))
 await state.refresh(async()=>({user:{used_bytes:300,quota_bytes:900},timezone:'Asia/Shanghai'}))
 assert.equal(state.user.value?.used_bytes,300)
 assert.equal(state.user.value?.quota_bytes,900)
 assert.equal(state.timezone.value,'Asia/Shanghai')
 await assert.rejects(state.refresh(async()=>{throw new Error('offline')}))
 assert.equal(state.user.value?.used_bytes,300)
})
