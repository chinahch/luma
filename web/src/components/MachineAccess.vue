<script setup lang="ts">
import { computed,onMounted,onUnmounted,ref } from 'vue'
import { NAlert,NButton,NFormItem,NInput,NModal,useMessage } from 'naive-ui'
import { api,stamp } from '../api'
import { machineCommand } from '../machineCommands'
const props=defineProps<{machine:{id:number,name:string}}>()
const emit=defineEmits<{close:[]}>(),message=useMessage()
const origin=ref(window.location.origin),code=ref(''),expiresAt=ref(''),busy=ref(false),error=ref(''),now=ref(Date.now()),commandField=ref<HTMLTextAreaElement|null>(null)
let timer:ReturnType<typeof setInterval>|undefined,active=true
const originError=computed(()=>{try{machineCommand(origin.value,props.machine.id,'A'.repeat(43));return ''}catch(e){return (e as Error).message}})
const remaining=computed(()=>Math.max(0,Math.ceil((Date.parse(expiresAt.value)-now.value)/1000))||0)
const command=computed(()=>code.value&&remaining.value&&!originError.value?machineCommand(origin.value,props.machine.id,code.value):'')
const countdown=computed(()=>`${Math.floor(remaining.value/60)} 分 ${remaining.value%60} 秒`)
async function generate(){
 if(originError.value||busy.value)return
 busy.value=true;error.value='';code.value=''
 try{
  const data=await api(`/api/admin/machines/${props.machine.id}/enrollment`,{method:'POST',body:'{}'})
  if(!active)return
  machineCommand(origin.value,props.machine.id,data.code)
  if(!Number.isFinite(Date.parse(data.expires_at))||Date.parse(data.expires_at)<=Date.now())throw new Error('安装命令已过期，请重新生成。')
  code.value=data.code;expiresAt.value=data.expires_at;now.value=Date.now()
 }catch(e){if(active)error.value=(e as Error).message}finally{busy.value=false}
}
async function copy(){
 if(!command.value)return
 try{if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(command.value);message.success('安装命令已复制');return}}catch{}
 const field=commandField.value
 field?.focus();field?.select()
 try{if(field&&document.execCommand('copy')){message.success('安装命令已复制');return}}catch{}
 message.info('命令已选中，请按 Ctrl+C 或长按复制。')
}
onMounted(()=>{timer=setInterval(()=>now.value=Date.now(),1000);generate()})
onUnmounted(()=>{active=false;clearInterval(timer);code.value=''})
</script>
<template><n-modal :show="true" preset="card" title="安装服务器" class="resource-modal" :bordered="false" @update:show="value=>{if(!value)emit('close')}">
 <p><strong>{{machine.name}}</strong> · 服务器 ID {{machine.id}}</p>
 <p class="muted">在这台服务器以 root 执行一条命令，即可安装原生服务并自动接入。支持 Linux amd64 / arm64，无需 Docker。已有安装会自动识别。</p>
 <n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert>
 <n-form-item label="面板地址（服务器必须能访问）"><n-input v-model:value="origin" placeholder="https://panel.example.com"/></n-form-item>
 <n-alert v-if="originError" type="warning" class="space-bottom">{{originError}}</n-alert>
 <n-alert v-if="code&&remaining" type="warning" class="space-bottom">此命令可将服务器绑定到当前面板，请妥善保管。剩余 {{countdown}}，到期时间 {{stamp(expiresAt)}}。重新生成会使上一条命令失效，已连接的服务器继续运行。</n-alert>
 <n-alert v-else-if="expiresAt&&!remaining" type="warning" class="space-bottom">命令已过期，请重新生成后执行。</n-alert>
 <label v-if="command" class="command-label" for="server-install-command">安装命令</label>
 <textarea v-if="command" id="server-install-command" ref="commandField" :value="command" readonly spellcheck="false" class="install-command" rows="7" @click="commandField?.select()"></textarea>
 <div class="machine-copy"><n-button type="primary" :disabled="!command" @click="copy">复制安装命令</n-button><n-button :disabled="!!originError" :loading="busy" @click="generate">{{code||expiresAt?'重新生成':'生成安装命令'}}</n-button></div>
 <n-alert type="info">安装完成后，服务器会自动显示在线和负载，即使还没有节点。到“节点管理”新建节点并选择这台服务器，保存后会自动同步配置。</n-alert>
 <template #footer><n-button @click="emit('close')">关闭</n-button></template>
</n-modal></template>
<style scoped>.machine-copy{display:flex;gap:10px;flex-wrap:wrap;margin:16px 0 20px}.command-label{display:block;margin-bottom:8px}.install-command{width:100%;box-sizing:border-box;resize:vertical;min-height:150px;border:1px solid var(--line);border-radius:8px;background:var(--canvas);color:var(--text);padding:12px;font:12px/1.7 Consolas,monospace;overflow-wrap:anywhere;word-break:break-all}.install-command:focus{outline:2px solid var(--green);outline-offset:2px}</style>
