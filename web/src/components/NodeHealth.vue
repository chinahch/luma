<script setup lang="ts">
import { onMounted,ref } from 'vue'
import { NAlert,NButton,NCheckbox,NEmpty,NTag } from 'naive-ui'
import { api,stamp } from '../api'
import { connectionState } from '../serverState'
import ProbeButton from './ProbeButton.vue'
const connectorLabels:Record<string,string>={ready:'连接器就绪',starting:'正在连接',unhealthy:'检查失败',stopped:'已停止',queue_blocked:'上报队列阻塞'}
const rows=ref<any[]>([]),busy=ref(false),error=ref('')
const selected=ref<number[]>([]),batchBusy=ref(false),buttons=new Map<number,InstanceType<typeof ProbeButton>>()
function bindButton(id:number,button:any){if(button)buttons.set(id,button);else buttons.delete(id)}
function select(id:number,checked:boolean){selected.value=checked?[...selected.value,id]:selected.value.filter(x=>x!==id)}
async function batch(){batchBusy.value=true;try{for(const id of selected.value.slice(0,3))await buttons.get(id)?.start()}finally{batchBusy.value=false}}
async function load(){busy.value=true;error.value='';try{rows.value=await api('/api/me/nodes');selected.value=selected.value.filter(id=>rows.value.some(row=>row.id===id&&row.active))}catch(e){error.value=(e as Error).message}finally{busy.value=false}}
onMounted(load)
</script>
<template><div class="page-heading"><div><span class="eyebrow">CONNECTIVITY</span><h1>节点连通性</h1><p>查看当前账户可使用的节点，并检测真实代理请求。</p></div><div class="button-row"><n-button :loading="batchBusy" :disabled="!selected.length" @click="batch">检测所选（{{selected.length}}/3）</n-button><n-button :loading="busy" @click="load">刷新节点</n-button></div></div><n-alert type="info" class="space-bottom">检测使用你的实际凭据，从归属节点机经过节点入站、路由及落地访问测试目标。可能产生少量计费用量；结果不代表你所在地网络的可达性。心跳只表示近期上报。</n-alert><n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert><n-empty v-if="!busy&&!rows.length&&!error" description="暂无可使用的节点，请检查套餐、额度和有效期"/><div class="health-grid"><article v-for="node in rows" :key="node.id" class="surface"><div class="section-heading"><div class="button-row"><n-checkbox :checked="selected.includes(node.id)" :disabled="!node.active||(!selected.includes(node.id)&&selected.length>=3)" :aria-label="'选择节点 '+node.name" @update:checked="select(node.id,$event)"/><h2>{{node.name}}</h2></div><n-tag size="small" :bordered="false" :type="node.active?'success':'default'">{{node.active?'启用':'停用'}}</n-tag></div><p class="muted small">近期上报：{{connectionState(node.last_report_at)==='online'?'45 秒内收到心跳':node.last_report_at?'心跳已过期':'暂无心跳'}}<br>{{stamp(node.last_report_at)}}</p><p v-if="node.connector_status" class="small muted">连接器 {{node.connector_status.kind}}：{{connectorLabels[node.connector_status.state]||node.connector_status.state||'状态未知'}}<br>状态时间：{{stamp(node.connector_status.seen_at)}} · {{connectionState(node.connector_status.seen_at)==='online'?'近期更新':'状态可能已过期'}}</p><ProbeButton :ref="el=>bindButton(node.id,el)" :node-id="node.id" :initial="node.latest_probe" :disabled="!node.active" @result="node.latest_probe=$event"/><p v-if="node.probe_supported===false" class="small muted">原生版本或连接器不支持通用网页代理检测，可点击查看实际检测边界。</p></article></div></template>
<style scoped>.health-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px}.button-row{flex-wrap:wrap}.section-heading{gap:10px;flex-wrap:wrap}@media(max-width:750px){.health-grid{grid-template-columns:1fr}.page-heading{align-items:stretch;flex-direction:column}}</style>
