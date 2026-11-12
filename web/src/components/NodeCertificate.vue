<script setup lang="ts">
import {computed,onMounted,reactive,ref,watch} from 'vue'
import {NAlert,NCheckbox,NFormItem,NInput,NSelect} from 'naive-ui'
import {api} from '../api'
import {applyCertificateToConfig,certificateDraft,credentialRequirement} from '../certificates'
const props=defineProps<{config:string}>()
const draft=reactive(certificateDraft({})),providers=ref<any[]>([]),loadError=ref(''),showSecrets=ref(false)
const config=computed(()=>{try{return JSON.parse(props.config)}catch{return {}}})
const visible=computed(()=>Number(config.value?.tls)===1)
const provider=computed(()=>providers.value.find(p=>p.name===draft.provider||p.aliases?.includes(draft.provider)))
const saved=computed(()=>draft.savedEnv&&draft.originalMode==='dns'&&draft.originalProvider===draft.provider)
watch(()=>props.config,()=>{Object.assign(draft,certificateDraft(config.value));showSecrets.value=false},{immediate:true})
onMounted(async()=>{try{providers.value=await api('/api/admin/certificates/providers')}catch(e){loadError.value=(e as Error).message}})
function buildConfig(current:Record<string,any>){return applyCertificateToConfig(current,draft)}
defineExpose({buildConfig})
</script>
<template><section v-if="visible" class="certificate-box"><h3>TLS 证书</h3><p class="muted">随节点一起保存，自动同步到归属服务器。服务器负责申请和续期。</p>
<n-alert v-if="loadError" type="error" class="space-bottom">{{loadError}}</n-alert>
<n-form-item label="证书模式"><n-select v-model:value="draft.mode" :options="[{label:'DNS-01（ACME，自动申请与续期）',value:'dns'},{label:'HTTP-01（ACME，自动申请与续期）',value:'http'},{label:'节点上的证书文件',value:'file'},{label:'粘贴 PEM 证书与私钥',value:'content'}]"/></n-form-item>
<div class="form-two"><n-form-item label="证书域名"><n-input v-model:value="draft.domain" placeholder="cert.example.com"/></n-form-item><n-form-item label="TLS 服务器名称（SNI）"><n-input v-model:value="draft.sni" placeholder="必须是证书覆盖的具体域名"/></n-form-item></div>
<n-form-item v-if="['dns','http'].includes(draft.mode)" label="通知邮箱"><n-input v-model:value="draft.email" placeholder="admin@example.com"/></n-form-item>
<template v-if="draft.mode==='dns'"><n-form-item label="DNS 提供商"><n-select v-model:value="draft.provider" filterable :options="providers.map(p=>({label:p.name,value:p.name}))" placeholder="选择域名的 DNS 提供商"/></n-form-item>
<p v-if="provider" class="small muted">凭据要求：{{credentialRequirement(provider)}}<br>支持变量：{{provider.keys.join('、')}}</p>
<p v-if="saved" class="small saved-secret">DNS 凭据已加密保存。留空保留，输入完整凭据可替换。</p>
<n-alert v-else-if="draft.savedEnv" type="warning" class="space-bottom">已更换 DNS 提供商，请填写新提供商的完整凭据。</n-alert>
<n-form-item label="环境变量（API 凭据，每行 KEY=VALUE）"><n-input v-model:value="draft.envText" type="textarea" :class="{'masked-secret':!showSecrets}" :autosize="{minRows:4,maxRows:8}" :input-props="{autocomplete:'off',spellcheck:false}" :placeholder="saved?'留空保留已保存的 DNS 凭据':'CF_API_TOKEN=你的新令牌'"/></n-form-item>
<n-checkbox v-model:checked="showSecrets">显示本次输入的凭据</n-checkbox>
<p class="small muted">DNS-01 不需要开放 80 端口，支持泛域名证书；DNS 凭据需要对应域名的 DNS 编辑权限。系统不会执行输入中的命令。</p></template>
<n-alert v-if="draft.mode==='http'" type="info" class="space-bottom">证书域名必须解析到归属节点机，公网 TCP 80 必须可访问且未被其他服务占用。HTTP-01 不支持泛域名。</n-alert>
<div v-if="draft.mode==='file'" class="form-two"><n-form-item label="节点上的证书绝对路径"><n-input v-model:value="draft.certFile" placeholder="/etc/caddy/cert.pem"/></n-form-item><n-form-item label="节点上的私钥绝对路径"><n-input v-model:value="draft.keyFile" placeholder="/etc/caddy/key.pem"/></n-form-item></div>
<template v-if="draft.mode==='content'"><n-form-item label="PEM 证书（含完整证书链）"><n-input v-model:value="draft.certContent" type="textarea" :autosize="{minRows:4,maxRows:8}"/></n-form-item><p v-if="draft.savedKey" class="small saved-secret">私钥已加密保存，留空保留；更换证书时必须与私钥匹配。</p><n-form-item label="PEM 私钥"><n-input v-model:value="draft.keyContent" type="textarea" :class="{'masked-secret':!showSecrets}" :autosize="{minRows:4,maxRows:8}" :input-props="{autocomplete:'off',spellcheck:false}"/></n-form-item><n-checkbox v-model:checked="showSecrets">显示本次输入的私钥</n-checkbox></template>
<n-alert type="info" class="certificate-note">保存后等待服务器同步并完成证书申请。申请成功后才启动 TLS 监听；申请失败可在节点列表点击“故障查询”。</n-alert>
</section></template>
<style scoped>.certificate-box{border:1px solid var(--border,#dfe5e1);border-radius:10px;padding:18px;margin:20px 0}.certificate-box h3{margin-top:0}.certificate-note{margin-top:18px}.saved-secret{color:var(--primary,#287d68)}.masked-secret :deep(textarea){-webkit-text-security:disc}.small{overflow-wrap:anywhere}</style>
