<script setup lang="ts">
import { computed,h,onMounted,onUnmounted,ref } from 'vue'
import { NAlert,NButton,NDataTable,NEmpty,NFormItem,NIcon,NInput,NModal,NPopconfirm,NProgress,NTag,useMessage } from 'naive-ui'
import { Plus,Refresh } from '@vicons/tabler'
import { api,bytes,stamp } from '../api'
import { connectionState,cpuPercent,storagePercent } from '../serverState'
import MachineAccess from './MachineAccess.vue'
interface Machine {id:number,name:string,node_count:number,last_report_at:string|null,status:any}
const message=useMessage(),rows=ref<Machine[]>([]),busy=ref(false),error=ref(''),now=ref(Date.now())
const editor=ref(false),editing=ref<number|null>(null),name=ref(''),saving=ref(false),formError=ref(''),accessMachine=ref<Machine|null>(null)
const detailsID=ref<number|null>(null),nodes=ref<any[]>([]),nodesBusy=ref(false),nodesError=ref('')
let timer:ReturnType<typeof setInterval>|undefined,active=true
const details=computed(()=>rows.value.find(x=>x.id===detailsID.value))
const associatedNodes=computed(()=>nodes.value.filter(x=>x.machine_id===detailsID.value))
const onlineCount=computed(()=>rows.value.filter(x=>connectionState(x.last_report_at,now.value)==='online').length)
const stateLabels={pending:'未连接',online:'在线',offline:'离线'}
function status(row:Machine){const state=connectionState(row.last_report_at,now.value);return h(NTag,{size:'small',bordered:false,type:state==='online'?'success':state==='offline'?'error':'default'},()=>stateLabels[state])}
function percent(value:number|null){return value===null?'暂无数据':`${value.toFixed(1)}%`}
function metric(label:string,value:number|null){return h('div',{class:'server-metric'},[h('span',{class:'metric-label'},label),value===null?h('span',{class:'metric-missing'},'暂无数据'):h(NProgress,{type:'line',percentage:Math.min(100,value),showIndicator:false,status:value>=85?'warning':'success',height:5}),value===null?null:h('span',{class:'metric-value'},percent(value))])}
function storage(value:any){return storagePercent(value)===null?'暂无数据':`${bytes(value.used)} / ${bytes(value.total)}`}
function speed(value:unknown){return typeof value==='number'&&Number.isFinite(value)&&value>=0?bytes(value)+'/s':'暂无数据'}
async function load(){
 if(busy.value)return
 busy.value=true;now.value=Date.now()
 try{const data=await api('/api/admin/machines');if(active){rows.value=data;error.value=''}}catch(e){if(active)error.value=(e as Error).message}finally{busy.value=false}
}
function open(row?:Machine){editing.value=row?.id??null;name.value=row?.name||'';formError.value='';editor.value=true}
async function save(){
 if(saving.value)return
 const value=name.value.trim();if(!value){formError.value='请填写服务器名称。';return}
 saving.value=true;formError.value=''
 try{
  const data=await api('/api/admin/machines'+(editing.value?'/'+editing.value:''),{method:editing.value?'PATCH':'POST',body:JSON.stringify({name:value})})
  const created=!editing.value;editor.value=false;message.success(created?'服务器已创建，执行安装命令即可接入':'名称已更新')
  await load()
  if(created)accessMachine.value={id:data.id,name:value,node_count:0,last_report_at:null,status:null}
 }catch(e){formError.value=(e as Error).message}finally{saving.value=false}
}
async function remove(row:Machine){try{await api('/api/admin/machines/'+row.id,{method:'DELETE'});message.success('服务器已删除');if(detailsID.value===row.id)detailsID.value=null;await load()}catch(e){message.error((e as Error).message)}}
async function inspect(row:Machine){detailsID.value=row.id;nodes.value=[];await loadNodes()}
async function loadNodes(){
 nodesBusy.value=true;nodesError.value=''
 try{const data=await api('/api/admin/nodes');if(active)nodes.value=data}catch(e){if(active)nodesError.value=(e as Error).message}finally{nodesBusy.value=false}
}
const columns:any[]=[
 {title:'服务器名称',key:'name',minWidth:170,render:(r:Machine)=>h('div',{class:'server-name'},[h('strong',r.name),h('small',`ID ${r.id}`)])},
 {title:'状态',key:'state',width:90,render:status},
 {title:'服务器负载',key:'load',minWidth:265,render:(r:Machine)=>h('div',{class:'server-load'},[metric('CPU',cpuPercent(r.status?.cpu)),metric('MEM',storagePercent(r.status?.mem)),metric('DISK',storagePercent(r.status?.disk))])},
 {title:'节点数量',key:'node_count',width:105},
 {title:'最后心跳',key:'last_report_at',minWidth:185,render:(r:Machine)=>r.last_report_at?stamp(r.last_report_at):'尚未连接'},
 {title:'操作',key:'actions',width:265,render:(r:Machine)=>h('div',{class:'button-row server-actions'},[
  h(NButton,{size:'small',quaternary:true,type:'primary',onClick:()=>inspect(r)},()=> '详情'),
  h(NButton,{size:'small',quaternary:true,onClick:()=>accessMachine.value=r},()=> '安装'),
  h(NButton,{size:'small',quaternary:true,onClick:()=>open(r)},()=> '编辑'),
  h(NPopconfirm,{onPositiveClick:()=>remove(r)},{trigger:()=>h(NButton,{size:'small',quaternary:true,type:'error',disabled:r.node_count>0,title:r.node_count>0?'请先修改关联节点的归属服务器':''},()=> '删除'),default:()=> '确定删除此服务器？连接凭据将失效。请先在服务器上停止对应服务，面板不会远程卸载软件。'})
 ])}
]
onMounted(()=>{load();timer=setInterval(()=>{now.value=Date.now();load()},10000)})
onUnmounted(()=>{active=false;clearInterval(timer)})
</script>
<template>
 <div class="page-heading"><div><span class="eyebrow">SERVERS</span><h1>服务器管理</h1><p>复制安装命令接入服务器；创建节点时选择服务器，保存后自动同步。</p></div><div class="button-row"><n-button :loading="busy" @click="load"><template #icon><n-icon :component="Refresh"/></template>刷新</n-button><n-button type="primary" @click="open()"><template #icon><n-icon :component="Plus"/></template>添加服务器</n-button></div></div>
 <n-alert v-if="error" type="error" class="space-bottom">刷新失败：{{error}}。当前显示上次成功读取的数据。</n-alert>
 <article class="surface"><div class="section-heading"><div><h2>全部服务器</h2><p>{{rows.length}} 台服务器 · {{onlineCount}} 台在线</p></div><n-tag size="small" :bordered="false">每 10 秒刷新</n-tag></div><n-data-table :columns="columns" :data="rows" :loading="busy" :bordered="false" :scroll-x="1180" :pagination="{pageSize:15}" :row-key="row=>row.id"/><p class="table-note">服务器安装完成即可上报心跳和负载，无需先创建节点。45 秒内收到心跳显示在线；缺少采样时显示“暂无数据”。有关联节点的服务器需先调整节点归属才能删除。</p></article>
 <n-modal v-model:show="editor" preset="card" :title="editing?'编辑服务器':'添加服务器'" class="resource-modal" :bordered="false"><n-alert v-if="formError" type="error" class="space-bottom">{{formError}}</n-alert><n-form-item label="服务器名称"><n-input v-model:value="name" :maxlength="100" placeholder="例如：香港 · 01" @keydown.enter.prevent="save"/></n-form-item><p v-if="!editing" class="muted small">保存后会自动生成安装命令，在服务器上执行即可完成接入。</p><template #footer><div class="modal-footer"><n-button @click="editor=false">取消</n-button><n-button type="primary" :loading="saving" @click="save">{{editing?'保存名称':'创建并获取安装命令'}}</n-button></div></template></n-modal>
 <n-modal :show="!!details" preset="card" title="服务器详情" class="resource-modal" :bordered="false" @update:show="value=>{if(!value)detailsID=null}"><template v-if="details"><div class="section-heading"><div><h2>{{details.name}}</h2><p>服务器 ID {{details.id}}</p></div><component :is="status(details)"/></div><div class="detail-grid"><div><span>CPU</span><strong>{{percent(cpuPercent(details.status?.cpu))}}</strong></div><div><span>内存 · 已用 / 总量</span><strong>{{storage(details.status?.mem)}}</strong></div><div><span>磁盘 · 已用 / 总量</span><strong>{{storage(details.status?.disk)}}</strong></div><div><span>最近心跳</span><strong>{{details.last_report_at?stamp(details.last_report_at):'尚未连接'}}</strong></div><div><span>接收速率</span><strong>{{speed(details.status?.net?.in_speed)}}</strong></div><div><span>发送速率</span><strong>{{speed(details.status?.net?.out_speed)}}</strong></div></div><div class="section-heading"><h2>关联节点 · {{details.node_count}}</h2><n-button size="small" :loading="nodesBusy" @click="loadNodes">刷新节点</n-button></div><n-alert v-if="nodesError" type="error">{{nodesError}}</n-alert><n-empty v-else-if="!nodesBusy&&!associatedNodes.length" description="暂无关联节点，可到节点管理创建"/><div v-for="node in associatedNodes" :key="node.id" class="associated-node"><div><strong>{{node.name}}</strong><small>ID {{node.id}} · {{node.config?.protocol?.toUpperCase()||'—'}}</small></div><n-tag size="small" :bordered="false" :type="node.active?'success':'default'">{{node.active?'启用':'停用'}}</n-tag></div><p class="table-note">这里的在线状态表示服务器与面板的连接状态，节点是否提供代理服务还取决于各节点配置。</p></template><template #footer><div class="modal-footer"><n-button @click="detailsID=null">关闭</n-button><n-button v-if="details" type="primary" @click="accessMachine=details">安装 / 重新接入</n-button></div></template></n-modal>
 <MachineAccess v-if="accessMachine" :machine="accessMachine" @close="accessMachine=null"/>
</template>
<style scoped>
:deep(.server-name){display:grid;gap:5px}:deep(.server-name small){font-size:10px;color:var(--muted)}:deep(.server-load){display:grid;gap:8px;padding:7px 0}:deep(.server-metric){display:grid;grid-template-columns:36px 1fr 48px;align-items:center;gap:10px;font-size:10px}:deep(.metric-label){color:var(--muted)}:deep(.metric-value){text-align:right;font-variant-numeric:tabular-nums}:deep(.metric-missing){grid-column:2/4;color:var(--muted)}:deep(.server-actions){gap:2px;flex-wrap:wrap}.detail-grid{display:grid;grid-template-columns:1fr 1fr;gap:22px 16px;margin:24px 0 30px}.detail-grid>div{display:grid;gap:7px;min-width:0}.detail-grid span,.associated-node small{font-size:11px;color:var(--muted)}.detail-grid strong{font-size:13px;overflow-wrap:anywhere}.associated-node{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:14px 0;border-top:1px solid var(--line)}.associated-node>div{display:grid;gap:5px}@media(max-width:640px){.page-heading{flex-direction:column;align-items:stretch}.page-heading>.button-row{flex-wrap:wrap}.detail-grid{grid-template-columns:1fr}.modal-footer{flex-wrap:wrap}}
</style>
