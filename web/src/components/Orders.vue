<script setup lang="ts">
import { computed,h,onMounted,ref } from 'vue'
import { NAlert,NButton,NDataTable,NTag } from 'naive-ui'
import { api,bytes,stamp } from '../api'
import Renewals from './Renewals.vue'
const emit=defineEmits<{changed:[]}>()
const props=defineProps<{userId?:number}>(),rows=ref<any[]>([]),busy=ref(false),error=ref('')
async function changed(){await load();emit('changed')}
async function load(){busy.value=true;error.value='';try{rows.value=await api(props.userId?`/api/admin/users/${props.userId}/orders`:'/api/me/orders')}catch(e){error.value=(e as Error).message}finally{busy.value=false}}
const columns=computed(()=>[{title:'订单',key:'id',minWidth:150},{title:'套餐快照',key:'plan_name',minWidth:150},{title:'额度',key:'quota_bytes',minWidth:120,render:(r:any)=>bytes(r.quota_bytes)},{title:'时长',key:'duration_days',minWidth:95,render:(r:any)=>r.duration_days+' 天'},{title:'标价',key:'price_cents',minWidth:110,render:(r:any)=>'¥ '+(Number(r.price_cents)/100).toFixed(2)},{title:'状态',key:'status',minWidth:100,render:(r:any)=>h(NTag,{size:'small',bordered:false},()=>r.status==='allocated'?'已分配':r.status||'已记录')},{title:'创建时间',key:'created_at',minWidth:185,render:(r:any)=>stamp(r.created_at)}])
onMounted(load)
</script>
<template><div class="page-heading"><div><span class="eyebrow">ORDERS</span><h1>{{userId?'TA 的订单':'订单与续期'}}</h1></div><n-button :loading="busy" @click="load">刷新</n-button></div><n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert><Renewals :user-id="userId" @changed="changed"/><article class="surface" style="margin-top:20px"><h2>已生效订单</h2><n-data-table :columns="columns" :data="rows" :loading="busy" :bordered="false" :scroll-x="1000" :pagination="{pageSize:10}"/></article></template>
