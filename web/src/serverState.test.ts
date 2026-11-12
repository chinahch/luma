import test from 'node:test'
import assert from 'node:assert/strict'
import * as state from './serverState.ts'
const helpers=state as any
test('server connectivity comes from heartbeat even when it has no nodes or metrics',()=>{
 assert.equal(typeof helpers.connectionState,'function')
 const now=Date.parse('2026-11-12T12:00:00Z')
 assert.equal(helpers.connectionState(null,now),'pending')
 assert.equal(helpers.connectionState('2026-11-12T11:59:15Z',now),'online')
 assert.equal(helpers.connectionState('2026-11-12T11:59:14Z',now),'offline')
 assert.equal(helpers.connectionState('invalid',now),'pending')
})
test('missing CPU and storage samples stay unavailable while a measured zero is valid',()=>{
 assert.equal(typeof helpers.cpuPercent,'function')
 assert.equal(helpers.cpuPercent(null),null)
 assert.equal(helpers.cpuPercent(undefined),null)
 assert.equal(helpers.cpuPercent(0),0)
 assert.equal(helpers.cpuPercent(47.125),47.125)
 assert.equal(helpers.cpuPercent(NaN),null)
 assert.equal(helpers.storagePercent({total:0,used:0}),null)
 assert.equal(helpers.storagePercent({total:100,used:0}),0)
 assert.equal(helpers.storagePercent({total:100,used:25}),25)
 assert.equal(helpers.storagePercent(null),null)
 assert.equal(helpers.storagePercent({total:100}),null)
})
