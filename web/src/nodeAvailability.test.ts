import test from 'node:test'
import assert from 'node:assert/strict'
import * as cards from './serverCards.ts'
test('node availability never labels an offline host or stale successful probe as online',()=>{
 const now=Date.parse('2026-11-13T12:00:00Z')
 const row={machine_last_report_at:'2026-11-13T11:59:50Z',latest_probe:{state:'succeeded',finished_at:'2026-11-13T11:59:00Z'}}
 assert.equal(cards.nodeAvailability(row,now).group,'online')
 assert.equal(cards.nodeAvailability({...row,machine_last_report_at:'2026-11-13T11:00:00Z'},now).group,'abnormal')
 assert.equal(cards.nodeAvailability({...row,latest_probe:{state:'succeeded',finished_at:'2026-11-13T11:40:00Z'}},now).group,'pending')
 assert.equal(cards.nodeAvailability({...row,latest_probe:{state:'failed',finished_at:'2026-11-13T11:59:00Z'}},now).group,'abnormal')
 assert.equal(cards.nodeAvailability({...row,latest_probe:null},now).group,'pending')
})
