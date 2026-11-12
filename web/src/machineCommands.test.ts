import test from 'node:test'
import assert from 'node:assert/strict'
import { machineCommand } from './machineCommands.ts'

test('native command pins installer, preserves IPv6 and never needs Docker or embeds a token',()=>{
 const cmd=machineCommand('https://[2001:db8::1]:8443',12,'install')
 assert.match(cmd,/--machine-id/);assert.match(cmd,/12/);assert.match(cmd,/\/([a-f0-9]{40})\/install-v2.sh/)
 assert.match(cmd,/--prompt-token/);assert.doesNotMatch(cmd,/docker|--token |latest/)
 const rebind=machineCommand('https://panel.example',12,'rebind')
 assert.match(rebind,/--rebind/)
})
test('invalid or loopback origins cannot produce an install command',()=>{
 for(const url of ['http://localhost:8088','http://127.0.0.1','http://[::1]','https://u:p@example.com','https://panel.example/path','https://panel.example?a=1','https://panel.example/#x','https://p.example;touch /tmp/bad','file:///etc/passwd'])assert.throws(()=>machineCommand(url,1,'install'),url)
 assert.throws(()=>machineCommand('https://panel.example',0,'install'))
})
