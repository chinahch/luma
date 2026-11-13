<script setup lang="ts">
import { computed,defineAsyncComponent,onMounted,onUnmounted,ref } from 'vue'
import { NAlert,NButton,NDropdown,NIcon,NInput,NSpin,NTag,useMessage } from 'naive-ui'
import { Dashboard,ChartBar,Users,Server,LayersLinked as Layers,Box,Link,Sun,Moon,Logout,ArrowRight,Menu2,Refresh,Mail,Lock,User,Apps } from '@vicons/tabler'
import { api,ApiError } from './api'
import { createAccountState } from './account'
import { inviteCode } from './featureState'
const Overview=defineAsyncComponent(()=>import('./components/Overview.vue'))
const UserDashboard=defineAsyncComponent(()=>import('./components/UserDashboard.vue'))
const Usage=defineAsyncComponent(()=>import('./components/Usage.vue'))
const Resources=defineAsyncComponent(()=>import('./components/Resources.vue'))
const Subscription=defineAsyncComponent(()=>import('./components/Subscription.vue'))
const Servers=defineAsyncComponent(()=>import('./components/Servers.vue'))
const Exits=defineAsyncComponent(()=>import('./components/Exits.vue'))
const Orders=defineAsyncComponent(()=>import('./components/Orders.vue'))
const Renewals=defineAsyncComponent(()=>import('./components/Renewals.vue'))
const Invites=defineAsyncComponent(()=>import('./components/Invites.vue'))
const NodeHealth=defineAsyncComponent(()=>import('./components/NodeHealth.vue'))
defineProps<{dark:boolean}>();defineEmits(['theme'])
const account=createAccountState<any>();const me=account.user,zone=account.timezone
const loading=ref(true),error=ref(''),page=ref('overview'),email=ref(''),password=ref(''),signing=ref(false),mobile=ref(false),accountError=ref('');let accountTimer:ReturnType<typeof setInterval>|undefined
const message=useMessage()
const customerMode=ref(false)
const customerShell=computed(()=>me.value?.role!=='admin'||customerMode.value)
const customerTabs=[{key:'overview',label:'仪表盘',icon:Dashboard},{key:'subscription',label:'我的订阅',icon:Link},{key:'orders',label:'套餐续期',icon:Layers},{key:'invite',label:'邀请好友',icon:Users}]
const moreOptions=[{label:'节点在线情况',key:'connectivity'},{label:'流量明细',key:'usage'}]
const profileOptions=computed(()=>[{label:me.value?.email||'',key:'email',disabled:true},...(me.value?.role==='admin'?[{label:customerMode.value?'管理后台':'用户中心',key:'switch'}]:[]),{label:'退出登录',key:'logout'}])
function profileAction(key:string){if(key==='logout')logout();else if(key==='switch'){customerMode.value=!customerMode.value;select('overview')}}
const invitation=inviteCode(location.search),registering=ref(!!invitation),inviteValid=ref(false),inviteLoading=ref(!!invitation),registrationNotice=ref('')
async function verifyInvite(){if(!invitation)return;inviteLoading.value=true;try{const data=await api('/api/invites/'+encodeURIComponent(invitation));inviteValid.value=data.valid===true;if(!inviteValid.value)error.value='邀请链接已失效。'}catch(e){error.value=(e as Error).message;inviteValid.value=false}finally{inviteLoading.value=false}}
async function register(){if(!inviteValid.value||signing.value)return;signing.value=true;error.value='';try{await api('/api/register',{method:'POST',body:JSON.stringify({email:email.value,password:password.value,invite_code:invitation})});password.value='';registering.value=false;registrationNotice.value='账号已注册，请登录。'}catch(e){error.value=(e as Error).message}finally{signing.value=false}}
const all=[{key:'overview',label:'运行概览',icon:Dashboard},{key:'usage',label:'流量统计',icon:ChartBar},{key:'users',label:'用户管理',icon:Users,admin:true},{key:'plans',label:'套餐管理',icon:Layers,admin:true},{key:'nodes',label:'节点管理',icon:Server,admin:true},{key:'machines',label:'服务器管理',icon:Box,admin:true},{key:'exits',label:'落地与路由',icon:Link,admin:true},{key:'subscription',label:'我的订阅',icon:Link},{key:'connectivity',label:'节点在线情况',icon:Server},{key:'orders',label:'订单与续期',icon:Layers},{key:'renewals',label:'续期订单',icon:Layers,admin:true},{key:'invite',label:'邀请好友',icon:Users}]
const items=computed(()=>all.filter(x=>!x.admin||me.value?.role==='admin'))
const title=computed(()=>all.find(x=>x.key===page.value)?.label||'Luma')
async function load(){loading.value=true;error.value='';try{await account.refresh(()=>api('/api/me'))}catch(e){if(e instanceof ApiError&&e.status===401)me.value=null;else error.value=(e as Error).message}finally{loading.value=false}}
async function login(){signing.value=true;error.value='';try{await api('/api/login',{method:'POST',body:JSON.stringify({email:email.value,password:password.value})});password.value='';await load()}catch(e){error.value=(e as Error).message}finally{signing.value=false}}
async function logout(){try{await api('/api/logout',{method:'POST'});me.value=null;page.value='overview';customerMode.value=false}catch(e){message.error((e as Error).message)}}
async function refreshAccount(){if(!me.value)return;try{await account.refresh(()=>api('/api/me'));accountError.value=''}catch(e){if(e instanceof ApiError&&e.status===401)me.value=null;else accountError.value='账户信息刷新失败，余额可能滞后：'+(e as Error).message}}
function select(key:string){page.value=key;mobile.value=false;refreshAccount()}
onMounted(()=>{load();verifyInvite();accountTimer=setInterval(refreshAccount,30000)});onUnmounted(()=>clearInterval(accountTimer))
</script>
<template>
 <div v-if="loading" class="screen-center"><n-spin size="large"/><span>正在连接 Luma</span></div>
 <div v-else-if="!me" class="auth-page">
  <n-button class="auth-theme" circle secondary :title="dark?'浅色模式':'深色模式'" @click="$emit('theme')"><n-icon :component="dark?Sun:Moon"/></n-button>
  <section class="auth-card"><a class="brand auth-brand" href="#"><span class="brand-mark">l</span>Luma</a><h1>{{registering?'创建账户':'登录账户'}}</h1><p class="muted auth-subtitle">{{registering?'使用邀请加入 Luma':'欢迎回来，请登录你的账户'}}</p>
  <n-alert v-if="registrationNotice" type="success" class="space-bottom">{{registrationNotice}}</n-alert><n-alert v-if="registering&&(!inviteValid||inviteLoading)" type="info" class="space-bottom">{{inviteLoading?'正在验证邀请…':'此邀请不可用，请联系邀请人。'}}</n-alert><n-alert v-if="error" type="error" class="space-bottom">{{error}}</n-alert>
  <form class="auth-form" @submit.prevent="registering?register():login()"><label for="login-email">邮箱</label><n-input v-model:value="email" type="text" placeholder="请输入邮箱地址" size="large" :input-props="{id:'login-email',type:'email',autocomplete:'username',required:true}"><template #prefix><n-icon :component="Mail"/></template></n-input><label for="login-password">密码</label><n-input v-model:value="password" type="password" show-password-on="click" placeholder="请输入密码" size="large" :input-props="{id:'login-password',autocomplete:registering?'new-password':'current-password',required:true,minlength:registering?12:undefined}"><template #prefix><n-icon :component="Lock"/></template></n-input><n-button type="primary" size="large" block attr-type="submit" :loading="signing" :disabled="registering&&(!inviteValid||inviteLoading)">{{registering?'创建账户':'登录'}} <template #icon><n-icon :component="ArrowRight"/></template></n-button></form><p v-if="registering" class="small muted auth-note">密码至少 12 字节</p><n-button v-if="invitation" class="auth-register" secondary block @click="registering=!registering;error=''">{{registering?'已有账户，返回登录':'使用邀请创建账户'}}</n-button></section>
 </div>
 <div v-else class="app-shell" :class="{'customer-shell':customerShell}">
  <div v-if="mobile&&!customerShell" class="mobile-overlay" @click="mobile=false"></div>
  <aside v-if="!customerShell" class="sidebar" :class="{open:mobile}"><a class="brand" href="#" @click.prevent="select('overview')"><span class="brand-mark">l</span>Luma<span class="brand-dot">.</span></a><div class="workspace-chip"><span class="workspace-avatar">L</span><div><strong>我的工作空间</strong><small>{{me.role==='admin'?'管理员控制台':'用户中心'}}</small></div><span class="status-dot"></span></div><span class="nav-label">工作空间</span><nav><button v-for="item in items" :key="item.key" :class="{active:page===item.key}" @click="select(item.key)"><n-icon :component="item.icon" size="20"/><span>{{item.label}}</span><span v-if="page===item.key" class="nav-current"></span></button></nav></aside>
  <div class="main-wrap"><header v-if="customerShell" class="customer-topbar"><a class="brand customer-brand" href="#" @click.prevent="select('overview')"><span class="brand-mark">l</span>Luma</a><nav class="customer-nav"><button v-for="item in customerTabs" :key="item.key" :class="{active:page===item.key}" @click="select(item.key)"><n-icon :component="item.icon" size="18"/><span>{{item.label}}</span></button><n-dropdown trigger="click" :options="moreOptions" @select="select"><button :class="{active:['connectivity','usage'].includes(page)}"><n-icon :component="Apps" size="18"/><span>更多</span></button></n-dropdown></nav><div class="customer-tools"><n-button circle secondary :title="dark?'浅色模式':'深色模式'" @click="$emit('theme')"><n-icon :component="dark?Sun:Moon"/></n-button><n-dropdown trigger="click" :options="profileOptions" @select="profileAction"><n-button circle secondary title="账户菜单"><n-icon :component="User"/></n-button></n-dropdown></div></header><header v-else class="topbar"><div class="breadcrumb"><n-button class="mobile-menu" title="打开导航" quaternary @click="mobile=true"><n-icon :component="Menu2"/></n-button><span>工作空间</span><span>/</span><strong>{{title}}</strong></div><div class="top-actions"><n-button secondary size="small" @click="profileAction('switch')">用户中心</n-button><n-button quaternary circle :title="dark?'浅色模式':'深色模式'" @click="$emit('theme')"><n-icon :component="dark?Sun:Moon"/></n-button><span class="account-avatar">{{me.email[0].toUpperCase()}}</span><div class="account-info"><strong>{{me.email}}</strong><small>{{me.role==='admin'?'管理员':'用户'}}</small></div><n-button quaternary circle title="退出登录" @click="logout"><n-icon :component="Logout"/></n-button></div></header>
  <main class="main-content"><n-alert v-if="accountError" type="warning" class="space-bottom">{{accountError}}</n-alert><UserDashboard v-if="page==='overview'&&customerShell" :me="me" :zone="zone" @navigate="select"/><Overview v-else-if="page==='overview'" :me="me" :zone="zone" @usage="select('usage')" @refresh-account="refreshAccount"/><Usage v-else-if="page==='usage'" :me="me" :zone="zone"/><Servers v-else-if="page==='machines'"/><Resources v-else-if="['users','plans','nodes'].includes(page)" :key="page" :kind="page" :me="me" :zone="zone"/><Exits v-else-if="page==='exits'"/><NodeHealth v-else-if="page==='connectivity'"/><Orders v-else-if="page==='orders'" @changed="refreshAccount"/><div v-else-if="page==='renewals'"><div class="page-heading"><div><span class="eyebrow">RENEWALS</span><h1>续期订单</h1></div></div><Renewals admin @changed="refreshAccount"/></div><Invites v-else-if="page==='invite'"/><Subscription v-else :me="me" :zone="zone" @renew="select('orders')"/></main><footer class="main-footer"><span>Luma · 0.1.0-preview</span></footer></div>
 </div>
</template>
