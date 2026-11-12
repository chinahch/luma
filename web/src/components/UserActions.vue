<script setup lang="ts">
import { computed,nextTick,ref } from 'vue'
import { NAlert,NButton,NDropdown,NFormItem,NModal,NSelect,useMessage } from 'naive-ui'
import { api,bytes } from '../api'
import { cycleStamp } from '../featureState'
import CopyField from './CopyField.vue'
import Orders from './Orders.vue'
import Invites from './Invites.vue'
import Usage from './Usage.vue'
const props=defineProps<{user:any;plans:any[];me:any;zone:string}>()
const emit=defineEmits<{edit:[any];changed:[]}>(),message=useMessage()
const view=ref(''),busy=ref(false),error=ref(''),planID=ref<number|null>(null),requestID=ref(''),path=ref(''),needsRecovery=ref(false),copyField=ref<InstanceType<typeof CopyField>|null>(null)
const link=computed(()=>path.value?location.origin+path.value:'')
const titles:Record<string,string>={allocate:'分配订单',subscription:'订阅 URL',identity:'重置 UUID 及订阅 URL',reset:'重置流量',delete:'删除用户',orders:'TA 的订单',invites:'TA 的邀请',usage:'TA 的流量记录'}
const options=[{label:'编辑',key:'edit'},{label:'分配订单',key:'allocate'},{label:'复制订阅 URL',key:'subscription'},{label:'重置 UUID 及订阅 URL',key:'identity'},{type:'divider',key:'d1'},{label:'TA 的订单',key:'orders'},{label:'TA 的邀请',key:'invites'},{label:'TA 的流量记录',key:'usage'},{label:'重置流量',key:'reset'},{type:'divider',key:'d2'},{label:'删除用户',key:'delete'}]
function uniqueID(){const a=new Uint8Array(16);crypto.getRandomValues(a);return Array.from(a,x=>x.toString(16).padStart(2,'0')).join('')}
async function select(key:string){
 if(key==='edit'){emit('edit',props.user);return}
 error.value='';view.value=key;path.value='';needsRecovery.value=false
 if(key==='allocate'){planID.value=null;requestID.value=uniqueID()}
 if(key==='subscription'){
  busy.value=true
  try{const data=await api(`/api/admin/users/${props.user.id}/subscription`);path.value=data.path||'';needsRecovery.value=data.needs_recovery;await nextTick();if(link.value)await copyField.value?.copy(link.value)}catch(e){error.value=(e as Error).message}finally{busy.value=false}
 }
}
async function apply(){
 if(busy.value)return
 busy.value=true;error.value=''
 try{
  const base=`/api/admin/users/${props.user.id}`
  if(view.value==='allocate'){
   if(!planID.value)throw new Error('请选择套餐。')
   await api(base+'/orders',{method:'POST',body:JSON.stringify({plan_id:planID.value,request_id:requestID.value})})
   message.success('套餐已分配，订单已记录')
  }else if(view.value==='identity'){
   const data=await api(base+'/identity',{method:'POST',body:'{}'});path.value=data.path;view.value='subscription';message.success('UUID 和订阅 URL 已重置');emit('changed');return
  }else if(view.value==='reset'){await api(base+'/reset-usage',{method:'POST',body:'{}'});message.success('本周期用量已清零，历史记录保留')}
  else if(view.value==='delete'){await api(base,{method:'DELETE'});message.success('用户已删除，历史记录保留')}
  view.value='';emit('changed')
 }catch(e){error.value=(e as Error).message}finally{busy.value=false}
}
const confirmation=computed(()=>['allocate','identity','reset','delete'].includes(view.value))
</script>
<template><n-dropdown trigger="click" placement="bottom-end" :options="options" @select="select"><n-button size="small" secondary :disabled="busy">操作 ···</n-button></n-dropdown><n-modal :show="!!view" preset="card" :title="titles[view]+' · '+user.email" class="resource-modal" :class="{'user-records-modal':['orders','invites','usage'].includes(view)}" :bordered="false" @update:show="value=>{if(!value&&!busy)view=''}"><n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert><template v-if="view==='allocate'"><n-alert type="info" class="space-bottom">后台手工分配套餐并记录订单，无在线收款。同套餐未到期续费保留本周期用量；新开通或换套餐开启新周期。</n-alert><n-form-item label="选择套餐"><n-select v-model:value="planID" :options="plans.map(p=>({label:`${p.name} · ${bytes(p.quota_bytes)} · ${p.duration_days} 天 · ¥ ${(p.price_cents/100).toFixed(2)}`,value:p.id}))"/></n-form-item><p class="small muted">提交失败后可重试，同一次分配使用相同请求编号，避免重复开通。</p></template><template v-else-if="view==='subscription'"><n-alert v-if="needsRecovery" type="warning">此账号的历史链接仅保存摘要。请让用户到“我的订阅”粘贴原链接恢复；也可明确重置 UUID 和订阅 URL，旧凭据将失效。</n-alert><CopyField v-if="link" ref="copyField" :value="link" label="该用户订阅 URL"/><p v-if="busy" class="muted">正在读取原订阅链接…</p><p class="small muted">链接包含访问凭据，请妥善保管。</p></template><template v-else-if="view==='identity'"><n-alert type="warning">重置将同时撤销旧 UUID 和订阅 URL。此用户已有客户端凭据立即失效，需要重新导入。确定继续？</n-alert></template><template v-else-if="view==='reset'"><n-alert type="warning">将本周期计费用量 {{bytes(user.used_bytes)}} 清零；保留全部流量记录、账本与每月开通日。确定继续？</n-alert><p class="small muted">下次重置：{{user.next_reset_at?cycleStamp(user.next_reset_at,zone):'尚未开通'}} · {{zone}}</p></template><template v-else-if="view==='delete'"><n-alert type="error">删除将停用该账号、撤销登录和订阅并移除节点下发，历史订单和流量账本保留。删除后无法在此恢复。确定继续？</n-alert><p v-if="user.id===me.id" class="small muted">不能删除当前登录账号。</p></template><Orders v-else-if="view==='orders'" :user-id="user.id"/><Invites v-else-if="view==='invites'" :user-id="user.id"/><Usage v-else-if="view==='usage'" :me="me" :zone="zone" :user-id="user.id"/><template #footer><div class="modal-footer"><n-button :disabled="busy" @click="view=''">{{confirmation?'取消':'关闭'}}</n-button><n-button v-if="confirmation" :type="view==='delete'?'error':'primary'" :loading="busy" :disabled="(view==='allocate'&&!planID)||(view==='delete'&&user.id===me.id)" @click="apply">{{view==='allocate'?'确认分配':'确认'+titles[view]}}</n-button></div></template></n-modal></template>

<style scoped>.resource-modal.user-records-modal{width:min(1100px,calc(100vw - 30px))!important}</style>
