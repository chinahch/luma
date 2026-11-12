<script setup lang="ts">
import { computed } from 'vue'
import { NButton,NFormItem,NInput,NSelect } from 'naive-ui'
import type { RouteDraft } from '../forms'
const props=defineProps<{exits:{id:number,name:string,active:boolean}[]}>()
const defaultExit=defineModel<number>('defaultExit',{required:true})
const rules=defineModel<RouteDraft[]>('rules',{required:true})
const options=computed(()=>{
 const choices=props.exits.map(x=>({label:x.name+(x.active?'':'（已停用）'),value:x.id,disabled:!x.active}))
 const selected=[defaultExit.value,...rules.value.map(r=>r.exit_id)].filter((id):id is number=>!!id)
 for(const id of selected)if(!choices.some(x=>x.value===id))choices.push({label:`落地 #${id}（不可用）`,value:id,disabled:true})
 return choices
})
function move(index:number,delta:number){const next=[...rules.value];const [item]=next.splice(index,1);next.splice(index+delta,0,item!);rules.value=next}
</script>
<template><section class="node-routing"><div class="section-heading"><div><h2>落地与分流</h2><p>随节点一起保存并自动同步到服务器。</p></div></div>
 <n-form-item label="默认出站"><n-select v-model:value="defaultExit" :options="[{label:'直连（保留节点原有路由）',value:0},...options]"/></n-form-item>
 <p v-if="!exits.length" class="muted small">暂无落地，可先到“落地与路由”导入；当前可使用直连。</p>
 <div v-for="(rule,index) in rules" :key="index" class="route-rule"><div class="section-heading"><strong>分流 {{index+1}}</strong><div class="button-row"><n-button size="small" quaternary :disabled="index===0" @click="move(index,-1)">上移</n-button><n-button size="small" quaternary :disabled="index===rules.length-1" @click="move(index,1)">下移</n-button><n-button size="small" quaternary type="error" @click="rules.splice(index,1)">移除</n-button></div></div>
 <n-form-item label="使用落地"><n-select v-model:value="rule.exit_id" :options="options" placeholder="选择已启用的落地"/></n-form-item>
 <div class="route-inputs"><n-form-item label="域名后缀（每行一条）"><n-input v-model:value="rule.domains" type="textarea" :autosize="{minRows:3,maxRows:7}" placeholder="example.com"/></n-form-item><n-form-item label="IP 网段（CIDR，每行一条）"><n-input v-model:value="rule.cidrs" type="textarea" :autosize="{minRows:3,maxRows:7}" placeholder="203.0.113.0/24"/></n-form-item></div></div>
 <n-button dashed block @click="rules.push({exit_id:null,domains:'',cidrs:''})">添加分流规则</n-button><p class="table-note">从上到下匹配，域名或 IP 任一命中即可。私网拦截优先，未命中时使用默认出站。已停用的落地需替换或移除后再保存。</p>
</section></template>
<style scoped>.node-routing{margin:24px 0;padding-top:24px;border-top:1px solid var(--line)}.route-rule{padding:15px;border:1px solid var(--line);border-radius:10px;margin:12px 0}.route-rule .section-heading{margin-bottom:14px;flex-wrap:wrap;gap:8px}.route-inputs{display:grid;grid-template-columns:1fr 1fr;gap:14px}@media(max-width:600px){.route-inputs{grid-template-columns:1fr;gap:0}.route-rule{padding:12px}.route-rule .button-row{gap:2px}}</style>
