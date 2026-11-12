import test from 'node:test'
import assert from 'node:assert/strict'
import { machineCommand } from './machineCommands.ts'

const code='A'.repeat(43)
test('one native enrollment command pins the installer and includes the short-lived binding code',()=>{
 const cmd=machineCommand('https://[2001:db8::1]:8443',12,code)
 assert.match(cmd,/--machine-id 12/)
 assert.match(cmd,/\/([a-f0-9]{40})\/install-v2.sh/)
 assert.match(cmd,/--enrollment-code/)
 assert.match(cmd,new RegExp(code))
 assert.match(cmd,/--mode machine/)
 assert.doesNotMatch(cmd,/docker|--token |--prompt-token|--rebind|latest/)
})
test('invalid binding codes and IDs cannot produce a command',()=>{
 for(const value of ['', 'install', 'x; touch /tmp/bad', "'".repeat(43), 'A'.repeat(42)])assert.throws(()=>machineCommand('https://panel.example',1,value),value)
 for(const id of [0,-1,1.5,NaN])assert.throws(()=>machineCommand('https://panel.example',id,code))
})
test('invalid or loopback origins cannot produce an install command',()=>{
 for(const url of ['http://localhost.','http://test.localhost','http://[::ffff:127.0.0.1]','http://localhost:8088','http://127.0.0.1','http://[::1]','http://0.0.0.0','https://u:p@example.com','https://panel.example/path','https://panel.example?a=1','https://panel.example/#x','https://p.example;touch /tmp/bad','file:///etc/passwd'])assert.throws(()=>machineCommand(url,1,code),url)
 assert.match(machineCommand('http://192.0.2.20:8088',1,code),/192.0.2.20:8088/)
})
