<script setup lang="ts">
import { ref } from 'vue'
import { NConfigProvider,NMessageProvider,zhCN,dateZhCN,darkTheme } from 'naive-ui'
import Panel from './Panel.vue'
const dark=ref(localStorage.getItem('luma-theme')==='dark')
function theme(){dark.value=!dark.value;localStorage.setItem('luma-theme',dark.value?'dark':'light');document.documentElement.dataset.theme=dark.value?'dark':'light'}
document.documentElement.dataset.theme=dark.value?'dark':'light'
const overrides={common:{primaryColor:'#3c60ce',primaryColorHover:'#5174e0',primaryColorPressed:'#2d4eac',borderRadius:'10px',fontFamily:'Inter, "Microsoft YaHei", system-ui, sans-serif'},Button:{fontWeight:'600'},Card:{borderRadius:'14px'}}
</script>
<template><n-config-provider :theme="dark?darkTheme:null" :theme-overrides="overrides" :locale="zhCN" :date-locale="dateZhCN"><n-message-provider><Panel :dark="dark" @theme="theme"/></n-message-provider></n-config-provider></template>
