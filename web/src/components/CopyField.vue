<script setup lang="ts">
import { ref } from 'vue'
import { NButton,useMessage } from 'naive-ui'
defineProps<{value:string;label?:string}>()
const field=ref<HTMLTextAreaElement|null>(null),message=useMessage()
async function copy(value:string){
 try{if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(value);message.success('已复制');return}}catch{}
 field.value?.focus();field.value?.select()
 try{if(document.execCommand('copy')){message.success('已复制');return}}catch{}
 message.info('链接已选中，请按 Ctrl+C 或长按复制。')
}
defineExpose({copy})
</script>
<template><div class="copy-field"><textarea ref="field" :aria-label="label||'链接'" :value="value" readonly rows="2" @click="field?.select()"></textarea><n-button secondary :disabled="!value" @click="copy(value)">复制</n-button></div></template>
<style scoped>.copy-field{display:flex;align-items:stretch;gap:10px;margin:14px 0}.copy-field textarea{flex:1;min-width:0;border:1px solid var(--line);border-radius:8px;background:var(--canvas);color:var(--text);padding:10px;font:12px/1.6 Consolas,monospace;resize:vertical;overflow-wrap:anywhere}.copy-field textarea:focus{outline:2px solid var(--green)}</style>
