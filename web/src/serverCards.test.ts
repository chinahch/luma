import {test} from 'node:test'
import assert from 'node:assert/strict'
import {serverCards,moveCard,countryLabel,automaticHealthView} from './serverCards.ts'
test('server cards group owned nodes, include empty machines and retain saved order',()=>{
 const cards=serverCards([{id:1,name:'one'},{id:2,name:'two'},{id:3,name:'empty'}],[{id:10,machine_id:1,active:true},{id:11,machine_id:2,active:false},{id:12,machine_id:null,active:true}],[2,1,999])
 assert.deepEqual(cards.map(c=>c.id),[2,1,3,0]);assert.equal(cards[0]!.nodes.length,1);assert.equal(cards[1]!.enabled,1);assert.equal(cards[2]!.nodes.length,0)
 assert.deepEqual(moveCard([2,1,3],3,2),[3,2,1]);assert.deepEqual(moveCard([2,1],99,1),[2,1])
})
test('country labels require valid region codes and self-test success becomes stale',()=>{
 assert.match(countryLabel('HK'),/香港/);assert.equal(countryLabel(''),'国家未知');assert.equal(countryLabel('bad'),'国家未知')
 const now=Date.parse('2026-11-13T00:00:00Z')
 assert.equal(automaticHealthView({state:'succeeded',finished_at:'2026-11-12T23:59:00Z'},now).text,'连通正常')
 assert.equal(automaticHealthView({state:'succeeded',finished_at:'2026-11-12T20:00:00Z'},now).text,'结果已过期')
 assert.equal(automaticHealthView(null,now).text,'等待服务器自测')
})
