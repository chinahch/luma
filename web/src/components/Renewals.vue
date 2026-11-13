<script setup lang="ts">
import {computed,h,onMounted,ref} from 'vue'
import {NAlert,NButton,NDataTable,NPopconfirm,NTag,useMessage} from 'naive-ui'
import {api,bytes,stamp} from '../api'
import {reconcileRenewalKey,assertPendingRenewal} from '../renewalState'
const props=defineProps<{admin?:boolean;userId?:number}>(),emit=defineEmits<{changed:[]}>(),message=useMessage()
const rows=ref<any[]>([]),plan=ref<any>(null),expiry=ref<string|null>(null),busy=ref(false),working=ref(false),error=ref(''),key=ref('')
const pending=computed(()=>rows.value.some(r=>r.status==='pending'))
const page=ref(1),total=ref(0)
const pagination=computed(()=>props.admin?{page:page.value,pageSize:20,itemCount:total.value,onUpdatePage:(value:number)=>{page.value=value;load()}}:{pageSize:10})
const labels:Record<string,string>={pending:'待确认',approved:'已生效',rejected:'已拒绝',cancelled:'已取消'}
function newKey(){const b=new Uint8Array(16);crypto.getRandomValues(b);return Array.from(b,v=>v.toString(16).padStart(2,'0')).join('')}
async function load(){busy.value=true;error.value='';try{
 if(props.admin){const data=await api('/api/admin/renewals?page='+page.value);rows.value=data.rows;total.value=data.total;if(page.value>1&&!rows.value.length){page.value=Math.max(1,Math.ceil(total.value/20));await load()}}
 else if(props.userId)rows.value=await api(`/api/admin/users/${props.userId}/renewals`)
 else{const [data,list]=await Promise.all([api('/api/me/renewal-plan'),api('/api/me/renewals')]);plan.value=data.plan;expiry.value=data.expires_at;rows.value=list}
 key.value=reconcileRenewalKey(key.value,rows.value)
}catch(e){error.value=(e as Error).message}finally{busy.value=false}}
async function submit(){if(working.value)return;working.value=true;error.value='';try{if(!key.value)key.value=newKey();const result=await api('/api/me/renewals',{method:'POST',body:JSON.stringify({request_id:key.value})});key.value='';assertPendingRenewal(result);message.success('续期订单已提交，等待管理员确认');await load();emit('changed')}catch(e){error.value=(e as Error).message}finally{working.value=false}}
async function resolve(row:any,action:string){if(working.value)return;working.value=true;error.value='';try{await api(props.admin||props.userId?`/api/admin/renewals/${row.id}/${action}`:`/api/me/renewals/${row.id}/cancel`,{method:'POST',body:'{}'});key.value='';message.success(action==='approve'?'续期已生效':action==='reject'?'订单已拒绝':'订单已取消');await load();emit('changed')}catch(e){error.value=(e as Error).message}finally{working.value=false}}
const columns=computed<any[]>(()=>[
 {title:'订单',key:'id',width:80,render:(r:any)=>'#'+r.id},...(props.admin?[{title:'用户',key:'email',minWidth:200}]:[]),
 {title:'套餐',key:'plan_name',minWidth:130},{title:'额度 / 时长',key:'quota_bytes',minWidth:160,render:(r:any)=>`${bytes(r.quota_bytes)} / ${r.duration_days} 天`},
 {title:'金额',key:'price_cents',width:100,render:(r:any)=>'¥ '+(r.price_cents/100).toFixed(2)},
 {title:'状态',key:'status',width:100,render:(r:any)=>h(NTag,{size:'small',round:true,bordered:false,type:r.status==='approved'?'success':r.status==='pending'?'warning':'default'},()=>labels[r.status]||r.status)},
 {title:'提交时间',key:'created_at',minWidth:180,render:(r:any)=>stamp(r.created_at)},
 {title:'操作',key:'actions',width:props.admin||props.userId?180:90,render:(r:any)=>r.status!=='pending'?'—':h('div',{class:'button-row'},(props.admin||props.userId?['approve','reject']:['cancel']).map(action=>h(NPopconfirm,{onPositiveClick:()=>resolve(r,action)},{trigger:()=>h(NButton,{size:'small',quaternary:true,type:action==='approve'?'primary':'default',disabled:working.value},()=>action==='approve'?'确认续期':action==='reject'?'拒绝':'取消'),default:()=>action==='approve'?`确认按此订单套餐续期 ${r.duration_days} 天，金额 ¥ ${(r.price_cents/100).toFixed(2)}？`:'确定'+(action==='reject'?'拒绝':'取消')+'此续期订单？'})))}
])
onMounted(load)
</script>
<template>
 <n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert>
 <article v-if="!admin&&!userId" class="surface renewal-plan"><div><span class="small muted">当前套餐</span><h2>{{plan?.name||'尚未分配套餐'}}</h2><p v-if="plan" class="muted">{{bytes(plan.quota_bytes)}} / {{plan.duration_days}} 天 <span class="renewal-price">¥ {{(plan.price_cents/100).toFixed(2)}}</span></p><p class="small muted">{{expiry?'到期时间：'+stamp(expiry):plan?'当前无到期时间，无需续期':'请联系管理员分配套餐'}}</p></div><div><n-popconfirm v-if="plan&&expiry&&!pending" @positive-click="submit"><template #trigger><n-button type="primary" :loading="working" :disabled="busy">申请续期</n-button></template>按 {{plan.name}} 续期 {{plan.duration_days}} 天，金额 ¥ {{(plan.price_cents/100).toFixed(2)}}，提交后由管理员确认。</n-popconfirm><n-tag v-else-if="pending" type="warning" :bordered="false">等待管理员确认</n-tag></div></article>
 <article class="surface"><div class="section-heading"><h2>{{admin?'续期订单':'续期申请'}}</h2><n-button size="small" :loading="busy" @click="load">刷新</n-button></div><n-data-table :columns="columns" :data="rows" :loading="busy" :bordered="false" :scroll-x="admin?1100:850" :remote="!!admin" :pagination="pagination"/></article>
</template>
<style scoped>.renewal-plan{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-bottom:20px}.renewal-plan h2{margin:8px 0}.renewal-plan p{margin:8px 0 0}.renewal-price{margin-left:18px;color:var(--green);font-weight:600}.section-heading{margin-bottom:18px;gap:16px}.section-heading h2{margin:0}@media(max-width:650px){.renewal-plan{align-items:flex-start;flex-direction:column}}</style>
