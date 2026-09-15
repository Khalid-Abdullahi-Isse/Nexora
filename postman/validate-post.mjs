// Validate the focused Post REST collection without relying on the legacy collection.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import {fileURLToPath} from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = p => fs.readFileSync(path.join(root, p), 'utf8');
const collection = JSON.parse(read('postman/collections/post-service.postman_collection.json'));
const source = read('services/post-service/internal/controller/http/routes.go');
const prefixes = {router: '', api: '/api/v1', protected: '/api/v1'};
const actual = new Set();
for (const [, owner, method, route] of source.matchAll(/(\w+)\.(GET|POST|PATCH|DELETE)\("([^"]+)"/g)) {
  assert(owner in prefixes, `Unknown route group ${owner}`);
  actual.add(`${method} ${prefixes[owner]}${route}`);
}
const covered = new Set();
for (const item of collection.item) {
  const r = item.request;
  assert(r.url.startsWith('{{post_base_url}}/'));
  const route = r.url.replace('{{post_base_url}}', '').split('?')[0]
    .replace('{{post_id}}', ':id').replace('{{user_id}}', ':userId');
  const key = `${r.method} ${route}`;
  assert(actual.has(key), `Unknown route ${key}`);
  covered.add(key);
  assert(['noauth', 'bearer'].includes(r.auth.type));
  if (r.body) JSON.parse(r.body.raw);
  assert(item.event.some(e => e.listen === 'test'));
  for (const event of item.event) new vm.Script(event.script.exec.join('\n'));
}
assert.deepEqual(covered, actual);
assert.equal(collection.variable.find(v => v.key === 'access_token').value, '');
console.log(`Post collection passed: ${actual.size} routes, ${collection.item.length} requests, test-script syntax and no embedded token.`);
