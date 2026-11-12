<script setup lang="ts">
import { computed,onUnmounted,ref,watch } from 'vue'
import { NButton,NTag } from 'naive-ui'
import { api,stamp } from '../api'
import { probeLabel,probePending } from '../featureState'
import type { Probe } from '../featureState'
const props=defineProps<{nodeId?:number;exitId?:number|null;machineId?:number|null;kernelType?:string;initial?:Probe|null;disabled?:boolean}>()
const emit=defineEmits<{result:[Probe]}>()
const result=ref<Probe|null>(props.initial||null),busy=ref(false),error=ref('')
let timer:ReturnType<typeof setTimeout>|undefined,version=0,active=true
const ready=computed(()=>!props.disabled&&(props.nodeId!=null||(props.exitId!=null&&!!props.machineId&&['xray','singbox'].includes(props.kernelType||''))))
const errorLabels:Record<string,string>={connector_only:'此连接器不支持网页代理检测，请查看连接器状态。',unsupported:'节点版本不支持检测，请升级原生服务。',timeout:'请求超时',authentication_failed:'认证失败',handshake_failed:'握手失败',account_unavailable:'账户不满足使用条件',machine_unavailable:'服务器未连接',icmp_timeout:'落地未响应 ICMP，代理连通性已通过。',icmp_unavailable:'节点机无法创建 ICMP 套接字，请检查原生服务的网络权限。',icmp_resolve_failed:'无法解析落地地址以发送 Ping。'}
async function poll(id:string,run:number){
 try{const data=await api<Probe>(`/api/${props.nodeId!=null?'me':'admin'}/probes/${encodeURIComponent(id)}`);if(!active||run!==version)return;result.value=data;emit('result',data);if(probePending(data))timer=setTimeout(()=>poll(id,run),1800)}catch(e){if(active&&run===version)error.value='结果刷新失败：'+(e as Error).message}
}
async function start(){
 if(!ready.value||busy.value||probePending(result.value))return
 const run=++version;clearTimeout(timer);busy.value=true;error.value='';result.value=null
 try{const data=await api<Probe>(props.nodeId!=null?`/api/me/nodes/${props.nodeId}/probes`:`/api/admin/exits/${props.exitId}/probes`,{method:'POST',body:JSON.stringify(props.nodeId!=null?{}:{machine_id:props.machineId,kernel_type:props.kernelType})});if(!active||run!==version)return;result.value=data;emit('result',data);if(probePending(data)&&data.id)timer=setTimeout(()=>poll(data.id!,run),1500)}catch(e){if(active&&run===version)error.value=(e as Error).message}finally{if(active&&run===version)busy.value=false}
}
function reset(){version++;clearTimeout(timer);result.value=props.initial||null;error.value='';busy.value=false;if(probePending(result.value)&&result.value?.id)poll(result.value.id,version)}
watch(()=>[props.nodeId,props.exitId,props.machineId,props.kernelType],reset)
watch(()=>props.initial,()=>{if(props.initial===result.value)return;clearTimeout(timer);result.value=props.initial||null;if(probePending(result.value)&&result.value?.id)poll(result.value.id,version)},{immediate:true})
if(probePending(result.value)&&result.value?.id)poll(result.value.id,version)
defineExpose({start})
onUnmounted(()=>{active=false;version++;clearTimeout(timer)})
</script>
<template><div class="probe"><div class="button-row"><n-button size="small" :loading="busy||probePending(result)" :disabled="!ready" @click="start">测试连通性</n-button><n-tag size="small" :bordered="false" :type="result?.state==='succeeded'?'success':result?.state==='failed'?'error':'default'">{{probeLabel(result)}}</n-tag></div><small v-if="!ready&&!nodeId" class="muted">先选择服务器与 xray / singbox 内核。</small><p v-if="error" class="probe-error">{{error}} <n-button v-if="result?.id" text size="tiny" @click="poll(result.id!,version)">重试读取</n-button></p><p v-if="result" class="probe-detail">来源：{{result.source||'节点机'}} · {{stamp(result.finished_at||result.created_at)}}<span v-if="result.ping_ms!=null"><br>Ping：节点机 → 落地，ICMP 往返延迟</span><span v-if="result.ping_error_code"><br>{{errorLabels[result.ping_error_code]||result.ping_error_code}}</span><span v-if="result.error_code"><br>{{errorLabels[result.error_code]||result.error_code}}</span></p></div></template>
<style scoped>.probe{margin:10px 0}.probe-detail,.probe-error{font-size:11px;line-height:1.7;margin:7px 0;overflow-wrap:anywhere}.probe-detail{color:var(--muted)}.probe-error{color:#c45656}.button-row{flex-wrap:wrap}</style>
