import test from 'node:test'
import assert from 'node:assert/strict'
import { initialExpiry } from './forms.ts'
test('editing an unlimited account keeps null instead of adding thirty days',()=>{
 assert.equal(initialExpiry(null,true,1700000000000),null)
 assert.equal(initialExpiry('2026-11-30T00:00:00Z',true,1700000000000),1795996800000)
 assert.equal(initialExpiry(null,false,1700000000000),1702592000000)
})
