import test from 'node:test'
import assert from 'node:assert/strict'
import * as calendar from './calendar.ts'

test('expiry presets extend a future date by calendar months and clamp month ends',()=>{
 const now=new Date(2026,10,13,10,30).getTime()
 const base=new Date(2027,0,31,12,34,56).getTime()
 for(const [months,y,m,d] of [[1,2027,1,28],[3,2027,3,30],[6,2027,6,31],[12,2028,0,31]]){
  assert.equal(calendar.extendExpiry(base,months,now),new Date(y,m,d,12,34,56).getTime())
 }
 assert.equal(calendar.extendExpiry(new Date(2028,1,29,12).getTime(),12,now),new Date(2029,1,28,12).getTime())
})

test('expired, unset or invalid expiry presets start from now',()=>{
 const now=new Date(2026,10,13,10,30).getTime()
 for(const value of [null,undefined,NaN,0,now-1000]){
  assert.equal(calendar.extendExpiry(value,1,now),new Date(2026,11,13,10,30).getTime())
 }
})
