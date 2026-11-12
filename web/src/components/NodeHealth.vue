<script setup lang="ts">
import {onMounted,onUnmounted,ref} from 'vue'
import {NAlert,NEmpty,NTag} from 'naive-ui'
import {api,stamp} from '../api'
import {connectionState} from '../serverState'
import {automaticHealthView} from '../serverCards'
const rows=ref<any[]>([]),busy=ref(false),error=ref('')
const errors:Record<string,string>={timeout:'真实代理请求超时',proxy_failed:'代理链路连接失败',target_failed:'测试目标访问失败',configuration_unavailable:'节点配置暂不可用',no_eligible_users:'暂无满足套餐、额度及有效期条件的账号，未执行代理自测',account_unavailable:'自测账号已不可用，将在下一轮重新选择',machine_unavailable:'服务器未连接',upgrade_required:'请升级原生节点服务',connector_only:'此连接器请查看下方实际运行状态',expired:'自测任务未在有效期内完成'}
const connectorLabels:Record<string,string>={ready:'连接器就绪',starting:'正在连接',unhealthy:'检查失败',stopped:'已停止',queue_blocked:'上报队列阻塞'}
let timer:ReturnType<typeof setTimeout>|undefined,active=true
async function load(){busy.value=true;try{const data=await api('/api/me/nodes');if(active){rows.value=data;error.value=''}}catch(e){if(active)error.value=(e as Error).message}finally{busy.value=false;if(active)timer=setTimeout(load,15000)}}
onMounted(load);onUnmounted(()=>{active=false;clearTimeout(timer)})
</script>
<template><div class="page-heading"><div><span class="eyebrow">CONNECTIVITY</span><h1>节点连通性</h1><p>由归属服务器自动执行真实代理自测，持续查看节点是否正常。</p></div><span class="small muted">每个节点约 5 分钟自测一次 · 页面每 15 秒刷新</span></div>
<n-alert type="info" class="space-bottom">无需手动检测。服务器使用符合该节点权限的实际账号，经过节点入站、路由及落地访问固定测试目标，可能产生少量计费用量。结果反映服务器自测情况；服务器心跳与代理连通状态分别显示。</n-alert>
<n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert><n-empty v-if="!busy&&!rows.length&&!error" description="暂无可使用的节点，请检查套餐、额度和有效期"/>
<div class="health-grid"><article v-for="node in rows" :key="node.id" class="surface"><div class="section-heading"><h2>{{node.name}}</h2><n-tag size="small" :bordered="false" :type="automaticHealthView(node.latest_probe).tone">{{automaticHealthView(node.latest_probe).text}}</n-tag></div><p class="small muted">服务器：{{connectionState(node.machine_last_report_at)==='online'?'在线':'未连接或心跳过期'}}<br>节点最近上报：{{stamp(node.last_report_at)}}</p><p v-if="node.latest_probe" class="small muted">服务器自动自测 · {{stamp(node.latest_probe.finished_at||node.latest_probe.created_at)}}<br>{{errors[node.latest_probe.error_code]||node.latest_probe.error_code}}</p><p v-else class="small muted">等待归属服务器同步并自动检测。</p><p v-if="node.connector_status" class="small muted">连接器 {{node.connector_status.kind}}：{{connectorLabels[node.connector_status.state]||'状态未知'}}<br>状态时间：{{stamp(node.connector_status.seen_at)}}</p></article></div></template>
<style scoped>.health-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px}.section-heading{gap:10px;flex-wrap:wrap}.section-heading h2{overflow-wrap:anywhere}.small{line-height:1.8}@media(max-width:750px){.health-grid{grid-template-columns:1fr}.page-heading{align-items:stretch;flex-direction:column}}</style>
