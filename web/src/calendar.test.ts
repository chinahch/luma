import test from 'node:test'
import assert from 'node:assert/strict'
import {calendarRange} from './calendar.ts'
test('shortcuts use selected timezone date even across midnight and DST',()=>{
 const now=new Date('2026-11-11T16:30:00Z')
 assert.deepEqual(calendarRange('Asia/Shanghai',7,0,now),{from:'2026-11-06',to:'2026-11-12'})
 assert.deepEqual(calendarRange('UTC',1,0,now),{from:'2026-11-11',to:'2026-11-11'})
 assert.deepEqual(calendarRange('America/Los_Angeles',1,1,new Date('2026-11-02T07:30:00Z')),{from:'2026-10-31',to:'2026-10-31'})
})
