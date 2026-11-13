import test from 'node:test'
import assert from 'node:assert/strict'
import * as renewal from './renewalState.ts'
test('a lost submission response is reconciled before a cancelled request can be resubmitted',()=>{
 assert.equal(renewal.reconcileRenewalKey('retry-key',[]),'retry-key')
 for(const status of ['pending','cancelled','approved','rejected']){
  assert.equal(renewal.reconcileRenewalKey('retry-key',[{request_id:'retry-key',status}]),'')
 }
 assert.doesNotThrow(()=>renewal.assertPendingRenewal({status:'pending'}))
 for(const status of ['cancelled','approved','rejected'])assert.throws(()=>renewal.assertPendingRenewal({status}))
})
