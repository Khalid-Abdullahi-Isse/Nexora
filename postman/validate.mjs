// Validate Postman coverage against the mounted Gin routes, without dependencies.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import {fileURLToPath} from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = p => fs.readFileSync(path.join(root, p), 'utf8');
const prefixes = {auth: 'auth', post: 'posts', chat: 'chats', notification: 'notifications'};
const actual = new Map();
const normalize = s => s.replace(/:[A-Za-z]\w*|{{\w+}}/g, ':param');
for (const [service, prefix] of Object.entries(prefixes)) {
  const source = read(`services/${service}-service/internal/controller/http/routes.go`);
  const groups = {router: '', r: ''};
  const routes = new Set();
  for (const line of source.split('\n')) {
    const group = line.match(/(\w+)\s*:=\s*(\w+)\.Group\("([^"]*)"/);
    if (group) {
      const [, name, parent, suffix] = group;
      assert(parent in groups, `Unknown group: ${parent}`);
      groups[name] = groups[parent] + suffix;
    }
    const route = line.match(/(\w+)\.(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\("([^"]*)"/);
    if (route) {
      const [, owner, method, suffix] = route;
      assert(owner in groups, `Unknown route owner: ${owner}`);
      let url = groups[owner] + suffix;
      if (url === '/health' || url === '/ready') url = `/api/v1/${prefix}${url}`;
      const key = `${method} ${normalize(url)}`;
      assert(!routes.has(key), `Duplicate route: ${key}`);
      routes.add(key);
    }
  }
  assert(!/\.(Any|Handle|Match)\(/.test(source), 'Extend validator for new routing construct');
  actual.set(service, routes);
}
const all = new Set([...actual.values()].flatMap(s => [...s]));
let requestCount = 0;
for (const filename of fs.readdirSync(path.join(root, 'postman/collections')).filter(n => n.endsWith('.json'))) {
  const c = JSON.parse(read(`postman/collections/${filename}`));
  const covered = new Set();
  const variables = Object.fromEntries(c.variable.map(v => [v.key, v.value]));
  for (const key of ['password', 'access_token', 'csrf_token', 'admin_access_token']) assert.equal(variables[key], '');
  function walk(node) {
    for (const e of node.event || []) new vm.Script(e.script.exec.join('\n'));
    for (const child of node.item || []) walk(child);
    if (!node.request) return;
    requestCount++;
    const r = node.request;
    assert(r.url.startsWith('{{base_url}}/'), `URL must use Kong: ${r.url}`);
    const key = `${r.method} ${normalize(r.url.replace('{{base_url}}', '').split('?')[0])}`;
    assert(all.has(key), `Nonexistent backend route: ${key}`);
    covered.add(key);
    assert(['noauth', 'bearer'].includes(r.auth?.type));
    assert(node.event?.some(e => e.listen === 'test'), `Missing assertions: ${node.name}`);
    const scripts = (node.event || []).map(e => e.script.exec.join('\n')).join('\n');
    for (const [, v] of JSON.stringify(r).matchAll(/{{(\w+)}}/g)) {
      assert(v in variables || scripts.includes(`set(\"${v}\"`), `Missing variable ${v}`);
    }
    if (r.body && !r.body.raw.startsWith('{{')) JSON.parse(r.body.raw);
  }
  walk(c);
  const service = filename.replace('-service.postman_collection.json', '');
  assert.deepEqual(covered, actual.get(service) || all, `Route coverage drift in ${filename}`);
}
for (const name of ['local','docker','development','staging','production']) {
  const env = JSON.parse(read(`postman/environments/${name}.postman_environment.json`));
  const vars = Object.fromEntries(env.values.map(v => [v.key, v.value]));
  assert.equal(Object.keys(vars).length, env.values.length);
  assert(new URL(vars.base_url).protocol.match(/^https?:$/));
  assert(new URL(vars.ws_base_url).protocol.match(/^wss?:$/));
  for (const key of ['email','password','new_password','admin_access_token']) assert.equal(vars[key], '');
  // Captured values stay collection-scoped; blank environment values would shadow them.
  for (const key of ['access_token','csrf_token','post_id','user_id','notification_id','conversation_id','message_id']) assert(!(key in vars));
}
console.log(`Validated ${all.size} backend routes across six collections (${requestCount} requests), five environments and script syntax.`);
