import test from 'node:test'
import assert from 'node:assert/strict'
import * as feature from './featureState.ts'

test('subscription reads preserve the original URL and recovery does not rotate it',async()=>{
 const state=(feature as any).subscriptionState()
 let reads=0
 await state.load(async()=>{reads++;return {path:'/sub/original',needs_recovery:false}})
 await state.load(async()=>{reads++;return {path:'/sub/original',needs_recovery:false}})
 assert.equal(reads,2);assert.equal(state.path,'/sub/original')
 await assert.rejects(state.load(async()=>{throw new Error('key unavailable')}))
 assert.equal(state.path,'/sub/original')
 await state.load(async()=>({path:'',needs_recovery:true}))
 assert.equal(state.path,'');assert.equal(state.needsRecovery,true)
 assert.equal((feature as any).recoveryToken('https://panel.test/sub/abc_123?format=clash'),'abc_123')
 assert.equal((feature as any).recoveryToken('abc_123'),'abc_123')
 assert.throws(()=>(feature as any).recoveryToken('https://panel.test/not-a-subscription'))
})
test('node plan selection deduplicates IDs and explicit empty means unrestricted',()=>{
 assert.deepEqual((feature as any).planIDs([3,1,3]),[3,1]);assert.deepEqual((feature as any).planIDs([]),[])
 assert.throws(()=>(feature as any).planIDs([0]));assert.throws(()=>(feature as any).planIDs([1.5]))
})
test('node active toggle rolls back failed isolated status update',async()=>{
 const node={active:true,name:'unchanged'}
 await assert.rejects((feature as any).toggleActive(node,false,async(active:boolean)=>{assert.equal(node.active,false);assert.equal(active,false);throw new Error('offline')}))
 assert.equal(node.active,true)
 await (feature as any).toggleActive(node,false,async()=>{});assert.equal(node.active,false);assert.equal(node.name,'unchanged')
})
test('probe states distinguish waiting, unsupported and real success',()=>{
 assert.equal((feature as any).probePending({state:'queued'}),true)
 assert.equal((feature as any).probePending({state:'running'}),true)
 for(const state of ['failed','unsupported','expired','succeeded'])assert.equal((feature as any).probePending({state}),false)
 assert.equal((feature as any).probeLabel(undefined),'尚未检测')
 assert.equal((feature as any).probeLabel({state:'unsupported'}),'不支持检测')
 assert.equal((feature as any).probeLabel({state:'succeeded',latency_ms:0}),'连通正常')
})
test('invite landing keeps opaque codes and rejects empty invitation',()=>{
 assert.equal((feature as any).inviteCode('?invite=opaque%2Bcode'),'opaque+code')
 assert.equal((feature as any).inviteCode('?invite='),null);assert.equal((feature as any).inviteCode('?x=1'),null)
})
test('monthly reset timestamps use the panel timezone at the midnight boundary',()=>{
 assert.equal((feature as any).cycleStamp('2026-11-12T16:00:00Z','Asia/Shanghai'),'2026/11/13 00:00:00')
 assert.equal((feature as any).cycleStamp('2026-11-12T16:00:00Z','UTC'),'2026/11/12 16:00:00')
})
test('diagnostic completion never implies healthy listening without host evidence',()=>{
 assert.deepEqual((feature as any).diagnosticSummary({state:'succeeded'}),{tone:'warning',title:'未收到诊断资料',detail:'任务已结束，但没有可分析的节点机诊断结果。'})
 const stopped={version:1,node_id:7,expected_port:443,transport:'tcp',listening:false,eligible_users:2,runtime_state:'stopped',findings:[],logs:[]}
 assert.equal((feature as any).diagnosticSummary({state:'succeeded',diagnostics:stopped}).title,'端口未监听')
 assert.equal((feature as any).diagnosticSummary({state:'succeeded',diagnostics:{...stopped,listening:null}}).title,'监听状态未确认')
 assert.equal((feature as any).diagnosticSummary({state:'succeeded',diagnostics:{...stopped,listening:true}}).title,'已确认监听')
})
test('diagnostic waiting and upgrade states explain host retrieval boundaries',()=>{
 assert.equal((feature as any).diagnosticSummary({state:'queued'}).title,'等待节点机领取')
 assert.equal((feature as any).diagnosticSummary({state:'expired'}).title,'诊断任务已过期')
 assert.match((feature as any).diagnosticSummary({state:'unsupported',error_code:'upgrade_required'}).detail,/升级/)
 assert.match((feature as any).diagnosticSummary({state:'succeeded',diagnostics:{version:1,node_id:7,expected_port:443,transport:'tcp',listening:false,eligible_users:0,runtime_state:'idle',findings:[],logs:[]}}).detail,/没有符合使用条件/)
})
test('machine public address choices require physical interface evidence and prefer IPv4',()=>{
 const physical={public_addresses:{ipv4:['203.0.113.10'],ipv6:['2001:db8::10'],source:'physical_interface',detected_at:'2026-11-12T00:00:00Z'}}
 assert.deepEqual((feature as any).machinePublicAddresses(physical),['203.0.113.10','2001:db8::10'])
 assert.deepEqual((feature as any).machinePublicAddresses({public_addresses:{...physical.public_addresses,source:'ip_echo'}}),[])
 assert.deepEqual((feature as any).machinePublicAddresses({public_ip:'198.51.100.5',warp_ip:'198.51.100.6'}),[])
 assert.deepEqual((feature as any).machinePublicAddresses({public_addresses:{...physical.public_addresses,ipv4:[]}}),['2001:db8::10'])
})
test('machine address autofill preserves manual hosts and clears a previous automatic address when evidence disappears',()=>{
 assert.equal((feature as any).autofillPublicHost('','','203.0.113.10'),'203.0.113.10')
 assert.equal((feature as any).autofillPublicHost('203.0.113.10','203.0.113.10','2001:db8::10'),'2001:db8::10')
 assert.equal((feature as any).autofillPublicHost('my.example.com','203.0.113.10','2001:db8::10'),'my.example.com')
 assert.equal((feature as any).autofillPublicHost('203.0.113.10','203.0.113.10',''),'')
 assert.equal((feature as any).autofillPublicHost('manual.example.com','',''),'manual.example.com')
})

test('offline machine is not mislabeled as an unsupported old version',()=>{
 assert.equal(feature.diagnosticSummary({state:'unsupported',error_code:'machine_unavailable'}).title,'服务器未连接')
 assert.equal(feature.probeLabel({state:'unsupported',error_code:'machine_unavailable'}),'服务器未连接')
})

test('only recent physical evidence is used for automatic host filling',()=>{
 const now=Date.parse('2026-11-12T14:00:00Z'),base={public_addresses:{source:'physical_interface',ipv4:['1.1.1.1'],ipv6:[],detected_at:'2026-11-12T13:59:30Z'}}
 assert.equal((feature as any).automaticMachineAddress(base,now),'1.1.1.1')
 assert.equal((feature as any).automaticMachineAddress({...base,public_addresses:{...base.public_addresses,detected_at:'2026-11-11T14:00:00Z'}},now),'')
 assert.equal((feature as any).automaticMachineAddress({...base,public_addresses:{...base.public_addresses,detected_at:'2026-11-13T14:00:00Z'}},now),'')
 assert.deepEqual(feature.machinePublicAddresses({...base,public_addresses:{...base.public_addresses,detected_at:'2026-11-11T14:00:00Z'}}),['1.1.1.1'])
})

test('landing latency is ICMP RTT after connectivity, never HTTP elapsed time',()=>{
 assert.equal(feature.probeLabel({state:'succeeded',latency_ms:9999,ping_ms:12.5} as any),'连通正常 · Ping 12.50 ms')
 assert.equal(feature.probeLabel({state:'succeeded',ping_error_code:'icmp_timeout'} as any),'代理连通 · Ping 无响应')
})
