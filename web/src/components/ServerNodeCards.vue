<script setup lang="ts">
import {onUnmounted,ref} from 'vue'
import {NButton,NEmpty,NTag} from 'naive-ui'
import {countryLabel} from '../serverCards'
import {connectionState} from '../serverState'
const props=defineProps<{cards:any[];saving:boolean}>()
const emit=defineEmits<{select:[id:number];move:[from:number,to:number]}>()
const dragging=ref<number|null>(null)
function drop(id:number){if(dragging.value!=null&&!props.saving&&id>0)emit('move',dragging.value,id);dragging.value=null}
let stopPointerDrag:(()=>void)|undefined
function startPointerDrag(id:number,event:PointerEvent){
 if(props.saving||id<=0||event.button!==0)return
 stopPointerDrag?.();dragging.value=id
 const finish=(e:PointerEvent)=>{
  if(e.pointerId!==event.pointerId)return
  const target=document.elementFromPoint(e.clientX,e.clientY)?.closest('[data-machine-card-id]')
  const targetID=Number(target?.getAttribute('data-machine-card-id'))
  if(props.cards.some(card=>card.id===targetID)&&targetID!==id)drop(targetID)
  stopPointerDrag?.()
 }
 const cancel=()=>{window.removeEventListener('pointerup',finish);window.removeEventListener('pointercancel',cancel);dragging.value=null;stopPointerDrag=undefined}
 stopPointerDrag=cancel;window.addEventListener('pointerup',finish);window.addEventListener('pointercancel',cancel)
}
onUnmounted(()=>stopPointerDrag?.())
</script>
<template><div class="cards-intro"><p class="muted">按归属服务器管理节点。拖拽卡片调整布局，排序会保存到当前管理员账号。</p></div>
<n-empty v-if="!cards.length" description="请先到服务器管理添加并安装服务器"/>
<div class="server-node-grid"><article v-for="(card,index) in cards" :key="card.id" class="surface server-node-card" :data-machine-card-id="card.id" :class="{dragging:dragging===card.id}" :draggable="card.id>0&&!saving" @dragstart="dragging=card.id;$event.dataTransfer?.setData('text/plain',String(card.id))" @dragend="dragging=null" @dragover.prevent @drop.prevent="drop(card.id)">
<div class="card-status"><span>{{countryLabel(card.country||'')}}</span><n-tag size="small" :bordered="false" :type="card.id&&connectionState(card.last_report_at)==='online'?'success':'default'">{{card.id?(connectionState(card.last_report_at)==='online'?'服务器在线':'未连接'):'待分配'}}</n-tag></div>
<button class="card-open" @click="emit('select',card.id)"><h2>{{card.name}}</h2><p class="small muted">{{card.id?'服务器 #'+card.id:'请为这些节点选择归属服务器'}}</p><div class="card-count"><strong>{{card.nodes.length}}</strong><span>个节点 · {{card.enabled}} 个启用</span></div><p class="card-enter">进入节点管理 →</p></button>
<div v-if="card.id" class="card-layout"><button type="button" class="drag-handle small muted" :disabled="saving" :aria-label="'拖动排序 '+card.name" @pointerdown.prevent="startPointerDrag(card.id,$event)">⠿ 拖动排序</button><div class="button-row"><n-button size="tiny" quaternary :disabled="saving||index===0" :aria-label="'前移 '+card.name" @click="emit('move',card.id,cards[index-1]?.id)">前移</n-button><n-button size="tiny" quaternary :disabled="saving||index>=cards.length-1||!cards[index+1]?.id" :aria-label="'后移 '+card.name" @click="emit('move',cards[index+1]?.id,card.id)">后移</n-button></div></div>
</article></div></template>
<style scoped>.server-node-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:20px}.server-node-card{padding:22px;transition:box-shadow .15s,border-color .15s}.server-node-card:hover{border-color:var(--primary,#287d68);box-shadow:0 8px 22px #183d3210}.dragging{opacity:.45}.card-status,.card-layout{display:flex;align-items:center;justify-content:space-between;gap:10px}.card-open{display:block;border:0;background:transparent;color:inherit;text-align:left;width:100%;cursor:pointer;font:inherit;padding:18px 0 0}.card-open h2{font-size:21px;margin:0;overflow-wrap:anywhere}.card-count{display:flex;align-items:baseline;gap:12px;margin:24px 0}.card-count strong{font-size:38px}.card-count span{font-size:13px;color:var(--muted)}.card-enter{color:var(--primary,#287d68);font-weight:600}.card-layout{border-top:1px solid var(--border,#dfe5e1);padding-top:12px}.drag-handle{border:0;background:transparent;color:inherit;font:inherit;cursor:grab;touch-action:none;user-select:none;padding:6px 0}.drag-handle:active{cursor:grabbing}.cards-intro{margin-bottom:20px}@media(max-width:1100px){.server-node-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:650px){.server-node-grid{grid-template-columns:1fr}}</style>
