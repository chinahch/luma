<script setup lang="ts">
import { computed,ref } from 'vue'
import { NAlert,NButton,NFormItem,NInput,NModal,NPopconfirm,NSelect,useMessage } from 'naive-ui'
import { api } from '../api'
import { machineCommand } from '../machineCommands'
const props=defineProps<{machine:{id:number,name:string},token?:string}>()
const emit=defineEmits<{close:[]}>(),message=useMessage()
const token=ref(props.token||''),origin=ref(window.location.origin),action=ref<'install'|'rebind'>('install'),busy=ref(false),error=ref('')
const generated=computed(()=>{try{return {command:machineCommand(origin.value,props.machine.id,action.value),error:''}}catch(e){return {command:'',error:(e as Error).message}}})
async function copy(value:string){try{await navigator.clipboard.writeText(value);message.success('已复制')}catch{message.warning('复制失败，请手动选择并复制')}}
async function rotate(){busy.value=true;error.value='';try{const data=await api(`/api/admin/machines/${props.machine.id}/token`,{method:'POST',body:'{}'});token.value=data.token;message.success('新令牌已生成，旧令牌已失效')}catch(e){error.value=(e as Error).message}finally{busy.value=false}}
</script>
<template><n-modal :show="true" preset="card" title="安装 / 接入机器" class="resource-modal" :bordered="false" @update:show="value=>{if(!value)emit('close')}">
 <p><strong>{{machine.name}}</strong> · 机器 ID {{machine.id}}</p>
 <n-alert v-if="token" type="warning" class="space-bottom">请先保存令牌。关闭窗口后无法再次读取；安装命令不包含令牌，执行时需按提示粘贴。</n-alert>
 <n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert>
 <n-form-item label="面板地址（节点服务器必须能访问）"><n-input v-model:value="origin" placeholder="https://panel.example.com"/></n-form-item>
 <n-alert v-if="generated.error" type="warning" class="space-bottom">{{generated.error}} 正式部署请设置 PUBLIC_URL 并配置 HTTPS。</n-alert>
 <n-form-item label="操作"><n-select v-model:value="action" :options="[{label:'首次安装：下载原生二进制并启动服务',value:'install'},{label:'已有 Caddy：只更新本机器令牌并重启服务',value:'rebind'}]"/></n-form-item>
 <p class="muted">在节点服务器以 root 执行下方命令，无需 Docker 或编译环境。首次安装支持 Linux amd64/arm64；执行时会提示粘贴机器令牌，输入不会显示。</p>
 <n-input v-if="generated.command" :value="generated.command" type="textarea" readonly class="code-input" :autosize="{minRows:4,maxRows:8}"/>
 <div class="machine-copy"><n-button :disabled="!generated.command" @click="copy(generated.command)">复制执行命令</n-button></div>
 <n-form-item label="机器令牌（仅保存在当前窗口）"><n-input v-model:value="token" type="password" show-password-on="click" :input-props="{autocomplete:'off'}" placeholder="粘贴之前保存的令牌，丢失后可重置"/></n-form-item>
 <div class="machine-copy"><n-button :disabled="!token" @click="copy(token)">复制令牌</n-button><n-popconfirm @positive-click="rotate"><template #trigger><n-button type="warning" :loading="busy">重置令牌</n-button></template>重置后旧机器和同身份连接器将无法继续认证，需要更新它们的令牌。确定重置？</n-popconfirm></div>
 <n-alert type="info">机器需要关联逻辑节点才能提供代理服务：到“节点管理”新建节点并选择本机器。CF Tunnel / MTProxy 还需独立连接器。旧安装更新令牌只修改私有凭据并重启服务，不升级节点二进制。</n-alert>
 <template #footer><n-button @click="emit('close')">已保存，关闭</n-button></template>
</n-modal></template>
<style scoped>.machine-copy{display:flex;gap:10px;flex-wrap:wrap;margin:12px 0 20px}</style>
