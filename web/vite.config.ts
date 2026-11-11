import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({plugins:[vue()],server:{port:15173,strictPort:true,proxy:{'/api':'http://127.0.0.1:18080','/healthz':'http://127.0.0.1:18080','/sub':'http://127.0.0.1:18080'}}})
