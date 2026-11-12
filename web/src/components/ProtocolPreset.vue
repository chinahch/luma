<script setup lang="ts">
import {computed,onMounted,reactive,ref,watch} from 'vue'
import {NAlert,NButton,NCheckbox,NFormItem,NInput,NInputNumber,NPopconfirm,NSelect} from 'naive-ui'
import {api} from '../api'
import {applyPresetDraft,newPresetDraft,presetDraft} from '../certificates'
const props=defineProps<{config:string;editing:boolean;machineId?:number|null}>()
const emit=defineEmits<{config:[value:string]}>(),catalog=ref<any[]>([]),error=ref(''),busy=ref(false),autoPort=ref(true)
const form=reactive(newPresetDraft())
const selected=computed(()=>catalog.value.find(p=>p.id===form.preset)),connector=computed(()=>['mtg','cf-tunnel'].includes(form.preset||'')),users=ref<any[]>([])
const current=computed(()=>{try{return JSON.parse(props.config)}catch{return {}}})
watch(()=>props.config,()=>{const cfg=current.value;Object.assign(form,!props.editing&&!cfg.preset?newPresetDraft():presetDraft(cfg));autoPort.value=!props.editing&&!cfg.preset},{immediate:true})
watch(()=>form.preset,()=>{if(selected.value?.reality&&!form.server_name){form.server_name='addons.mozilla.org';form.destination='addons.mozilla.org:443'}if(selected.value?.certificate&&form.server_name==='addons.mozilla.org'){form.server_name='';form.destination=''}})
const combinations=[{label:'X25519 · native',value:'x25519:native'},{label:'X25519 · xorpub',value:'x25519:xorpub'},{label:'X25519 · random',value:'x25519:random'},{label:'ML-KEM-768 · native',value:'mlkem768:native'},{label:'ML-KEM-768 · xorpub',value:'mlkem768:xorpub'},{label:'ML-KEM-768 · random',value:'mlkem768:random'}]
async function generate(force=false){error.value='';busy.value=true;try{
 if(!form.preset)throw new Error('请选择需要生成的协议')
 if(props.editing&&!force&&form.preset===presetDraft(current.value).preset){emit('config',JSON.stringify(applyPresetDraft(current.value,form),null,2));return}
 const {choice,...input}=form,[authentication,appearance]=choice.split(':')
 if(selected.value?.certificate&&!input.server_name)throw new Error('请填写你自己控制的证书域名，然后一键生成；DNS 凭据在下方 TLS 证书区域填写')
 const config=await api('/api/admin/protocols/preset',{method:'POST',body:JSON.stringify({...input,owner_id:input.owner_id||0,authentication,appearance,machine_id:props.machineId||0,auto_port:!props.editing&&autoPort.value})})
 emit('config',JSON.stringify(config,null,2));form.tunnel_token=''
 }catch(e){error.value=(e as Error).message}finally{busy.value=false}}
function buildConfig(){return applyPresetDraft(current.value,form)}
defineExpose({buildConfig})
onMounted(async()=>{try{[catalog.value,users.value]=await Promise.all([api('/api/admin/protocols'),api('/api/admin/users')])}catch(e){error.value=(e as Error).message}})
</script>
<template><section class="preset-box"><h3>协议配置</h3><n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert>
<n-form-item label="协议组合"><n-select v-model:value="form.preset" :options="catalog.map(p=>({label:p.name,value:p.id}))" placeholder="当前为自定义协议配置，可选择预设重新生成"/></n-form-item>
<div class="form-two"><n-form-item label="独立监听端口"><n-input-number v-model:value="form.port" :min="1" :max="65535" :precision="0" @update:value="autoPort=false"/></n-form-item>
<n-form-item v-if="selected?.reality||connector||(selected?.certificate&&(!editing||form.preset!==presetDraft(current).preset))" :label="selected?.certificate?'你自己控制的证书域名':'TLS / REALITY 服务器名称'"><n-input v-model:value="form.server_name" :placeholder="selected?.reality?'addons.mozilla.org':'cert.example.com'"/></n-form-item></div>
<n-checkbox v-if="!editing" v-model:checked="autoPort">生成时自动选择面板中未占用的监听端口</n-checkbox>
<n-form-item v-if="selected?.reality" label="REALITY 握手目标"><n-input v-model:value="form.destination" placeholder="addons.mozilla.org:443"/></n-form-item>
<n-form-item v-if="selected?.enc" label="ENC 认证 / 外观"><n-select v-model:value="form.choice" :options="combinations"/></n-form-item>
<n-form-item v-if="selected?.path" label="独立路径（生成时留空自动创建）"><n-input v-model:value="form.path" placeholder="/独立路径"/></n-form-item>
<template v-if="connector"><n-form-item label="本机指标端口（每个连接器需不同）"><n-input-number v-model:value="form.metrics_port" :min="1024" :max="65535" :precision="0"/></n-form-item>
<n-form-item v-if="form.preset==='cf-tunnel'" label="Cloudflare 命名隧道令牌"><n-input v-model:value="form.tunnel_token" type="password" show-password-on="click" :placeholder="editing?'留空保留原令牌':'填写 Cloudflare 提供的 Tunnel Token'" :input-props="{autocomplete:'off'}"/></n-form-item>
<n-form-item v-else label="唯一归属用户"><n-select v-model:value="form.owner_id" :options="users.map(u=>({label:u.email,value:u.id}))"/></n-form-item>
<n-alert type="info" class="space-bottom">连接器需要同机运行 luma-connect。CF Tunnel 请在 Cloudflare 配置相同域名并指向本机 WS 端口，客户端使用 443/TLS。MTProxy 每用户独立端口和密钥。</n-alert></template>
<p class="muted">内核：{{selected?.kernel||'自定义配置'}}。一键生成按所选协议填写正确参数，生成后仍可修改。已有节点的普通参数修改保留密钥；更换协议或显式重新生成时才创建新密钥。</p>
<div class="button-row"><n-button type="primary" :loading="busy" @click="generate()">{{editing&&form.preset===presetDraft(current).preset?'应用参数（保留密钥）':'一键生成配置'}}</n-button>
<n-popconfirm v-if="editing&&form.preset" @positive-click="generate(true)"><template #trigger><n-button :disabled="busy" type="warning" secondary>重新生成配置与密钥</n-button></template>重新生成将替换本节点协议密钥与相关参数，旧客户端配置可能失效，确定继续？</n-popconfirm></div>
</section></template>
<style scoped>.preset-box{padding:16px;margin-bottom:20px;border:1px solid var(--border,#dfe5e1);border-radius:10px}.preset-box h3{margin-top:0}.preset-box :deep(.n-checkbox){margin-bottom:16px}</style>
