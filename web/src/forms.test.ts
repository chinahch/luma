import test from 'node:test'
import assert from 'node:assert/strict'
import { initialExpiry } from './forms.ts'
test('editing an unlimited account keeps null instead of adding thirty days',()=>{
 assert.equal(initialExpiry(null,true,1700000000000),null)
 assert.equal(initialExpiry('2026-11-30T00:00:00Z',true,1700000000000),1795996800000)
 assert.equal(initialExpiry(null,false,1700000000000),1702592000000)
})

test('node routing serializes ordered domain and CIDR rules with the established API keys',()=>{
 const f=forms as any
 assert.equal(typeof f.nodeRouting,'function')
 assert.deepEqual(f.nodeRouting(0,[{exit_id:7,domains:' example.com,\n test.org ',cidrs:'203.0.113.0/24\n2001:db8::/32'}]),{default_exit:0,rules:[{exit_id:7,domain_suffixes:['example.com','test.org'],ip_cidrs:['203.0.113.0/24','2001:db8::/32']}]})
 assert.deepEqual(f.nodeRouting(2,[]),{default_exit:2,rules:[]})
 assert.throws(()=>f.nodeRouting(0,[{exit_id:null,domains:'example.com',cidrs:''}]))
 assert.throws(()=>f.nodeRouting(0,[{exit_id:2,domains:'',cidrs:''}]))
})
import * as forms from './forms.ts'

test('explicit global mode persists both on and off and requires a global landing',()=>{
 const f=forms as any
 assert.deepEqual(f.nodeRouting(7,[],false),{global:false,default_exit:7,rules:[]})
 assert.deepEqual(f.nodeRouting(7,[],true),{global:true,default_exit:7,rules:[]})
 assert.throws(()=>f.nodeRouting(0,[],true))
})
