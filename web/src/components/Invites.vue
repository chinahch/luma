<script setup lang="ts">
import { computed,onMounted,ref } from 'vue'
import { NAlert,NButton,NDataTable } from 'naive-ui'
import { api,stamp } from '../api'
import CopyField from './CopyField.vue'
const props=defineProps<{userId?:number}>(),data=ref<any>(null),busy=ref(false),error=ref('')
const rows=computed(()=>props.userId?(data.value||[]):data.value?.invites||[])
const link=computed(()=>data.value?.path?new URL(data.value.path,location.origin).href:'')
const columns=[{title:'受邀用户',key:'email',minWidth:220},{title:'注册时间',key:'created_at',minWidth:185,render:(r:any)=>stamp(r.created_at)}]
async function load(){busy.value=true;error.value='';try{data.value=await api(props.userId?`/api/admin/users/${props.userId}/invites`:'/api/me/invite')}catch(e){error.value=(e as Error).message}finally{busy.value=false}}
onMounted(load)
</script>
<template><div class="page-heading"><div><span class="eyebrow">INVITATIONS</span><h1>{{userId?'TA 的邀请':'邀请好友'}}</h1></div><n-button :loading="busy" @click="load">刷新</n-button></div><n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert><article v-if="!userId&&link" class="surface"><h2>我的邀请链接</h2><CopyField :value="link" label="邀请注册链接"/><p class="muted small">已邀请 {{data.invited_count}} 人</p></article><article class="surface"><h2>邀请记录</h2><n-data-table :columns="columns" :data="rows" :loading="busy" :bordered="false" :scroll-x="450" :pagination="{pageSize:10}"/></article></template>
