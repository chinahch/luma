<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue'
import {NAlert,NEmpty,NIcon,NTag} from 'naive-ui'
import {Server} from '@vicons/tabler'
import {api,stamp} from '../api'
import {connectionState} from '../serverState'
import {nodeAvailability} from '../serverCards'
const rows=ref<any[]>([]),busy=ref(false),error=ref(''),now=ref(Date.now())
const summary=computed(()=>rows.value.reduce((s,n)=>{s[nodeAvailability(n,now.value).group]++;return s},{online:0,abnormal:0,pending:0}))
const errors:Record<string,string>={timeout:'真实代理请求超时',proxy_failed:'代理链路连接失败',target_failed:'测试目标访问失败',configuration_unavailable:'节点配置暂不可用',no_eligible_users:'暂无满足套餐、额度及有效期条件的账号，未执行代理自测',account_unavailable:'自测账号已不可用，将在下一轮重新选择',machine_unavailable:'服务器未连接',upgrade_required:'请升级原生节点服务',connector_only:'此连接器请查看下方实际运行状态',expired:'自测任务未在有效期内完成'}
const connectorLabels:Record<string,string>={ready:'连接器就绪',starting:'正在连接',unhealthy:'检查失败',stopped:'已停止',queue_blocked:'上报队列阻塞'}
let timer:ReturnType<typeof setTimeout>|undefined,active=true
async function load(){busy.value=true;try{const data=await api('/api/me/nodes');if(active){rows.value=data;now.value=Date.now();error.value=''}}catch(e){if(active){now.value=Date.now();error.value=(e as Error).message}}finally{busy.value=false;if(active)timer=setTimeout(load,15000)}}
onMounted(load);onUnmounted(()=>{active=false;clearTimeout(timer)})
</script>
<template>
 <div class="page-heading"><div><span class="eyebrow">NODE STATUS</span><h1>节点在线情况</h1></div><span class="small muted">自动更新 · 每 15 秒刷新</span></div>
 <n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert>
 <article class="surface status-surface" :aria-busy="busy">
  <div class="status-summary"><strong>全部节点 <span class="muted">{{rows.length}}</span></strong><div class="status-counts"><span><i class="status-dot good"></i>正常 <b>{{summary.online}}</b></span><span><i class="status-dot bad"></i>异常 <b>{{summary.abnormal}}</b></span><span><i class="status-dot waiting"></i>待检测 <b>{{summary.pending}}</b></span></div></div>
  <n-empty v-if="!busy&&!rows.length&&!error" description="暂无可用节点" style="padding:32px 0"/>
  <div v-if="rows.length" class="status-columns"><span>节点</span><span>状态</span><span>服务器</span><span>最近检测</span></div>
  <div v-for="node in rows" :key="node.id" class="status-row">
   <div class="node-name"><span class="node-symbol"><n-icon :component="Server" size="19"/></span><div><strong>{{node.name}}</strong><small class="muted">{{(node.type||node.protocol||'节点').toUpperCase()}} · #{{node.id}}</small></div></div>
   <div><n-tag size="small" round :bordered="false" :type="nodeAvailability(node,now).tone">{{nodeAvailability(node,now).text}}</n-tag></div>
   <div class="host-state"><i class="status-dot" :class="connectionState(node.machine_last_report_at,now)==='online'?'good':'waiting'"></i>{{connectionState(node.machine_last_report_at,now)==='online'?'在线':'未连接'}}</div>
   <div class="probe-time"><span>{{node.latest_probe?stamp(node.latest_probe.finished_at||node.latest_probe.created_at):'等待自动检测'}}</span><small v-if="node.latest_probe?.error_code" class="muted">{{errors[node.latest_probe.error_code]||node.latest_probe.error_code}}</small><small v-if="node.connector_status" class="muted">{{node.connector_status.kind}} · {{connectorLabels[node.connector_status.state]||'状态未知'}}</small></div>
  </div>
 </article>
</template>
<style scoped>
.status-surface{padding:0 24px;overflow:hidden}.status-summary{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:20px 0}.status-summary strong{font-size:15px}.status-summary strong span{margin-left:8px;font-weight:500}.status-counts{display:flex;gap:22px;font-size:13px}.status-counts>span{display:flex;align-items:center;gap:7px}.status-counts b{font-weight:600}.status-dot{flex:none;width:7px;height:7px;box-shadow:none;margin:0}.good{background:#279b75}.bad{background:#dd5b67}.waiting{background:#a2acb5}.status-columns,.status-row{display:grid;grid-template-columns:minmax(140px,1.3fr) minmax(110px,.7fr) minmax(85px,.7fr) minmax(175px,1fr);gap:20px;align-items:center}.status-columns{padding:11px 0;border-top:1px solid var(--line);font-size:12px;color:var(--muted)}.status-row{padding:17px 0;border-top:1px solid var(--line)}.node-name{display:flex;gap:12px;align-items:center;min-width:0}.node-name strong{font-size:14px;overflow-wrap:anywhere}.node-name small,.probe-time small{display:block;font-size:12px;line-height:1.7}.node-symbol{display:flex;align-items:center;justify-content:center;width:36px;height:36px;border-radius:10px;background:rgba(39,155,117,.08);color:var(--green);flex:none}.host-state{display:flex;gap:7px;align-items:center;font-size:13px}.probe-time{font-size:12px;overflow-wrap:anywhere}@media(max-width:760px){.status-surface{padding:0 16px}.status-summary{align-items:flex-start;gap:12px;flex-direction:column}.status-counts{gap:18px}.status-columns{display:none}.status-row{grid-template-columns:minmax(0,1fr) auto;gap:10px 14px}.probe-time{text-align:right}.page-heading{align-items:stretch;flex-direction:column;gap:8px}}
</style>
