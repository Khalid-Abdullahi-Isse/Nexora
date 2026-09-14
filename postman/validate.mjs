// Dependency-free validation of this repository's current simple Gin route declarations.
// Deliberately fails on new routing constructs so coverage cannot silently drift.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import {fileURLToPath} from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = p => fs.readFileSync(path.join(root, p), 'utf8');
const collection = JSON.parse(read('postman/collections/social-media-backend.postman_collection.json'));
const serviceNames = fs.readdirSync(path.join(root, 'services')).filter(s => fs.existsSync(path.join(root, 'services', s, 'cmd/server/main.go')));
const actual = new Set();
for (const service of serviceNames) {
  const source = read(`services/${service}/internal/controller/http/routes.go`);
  const groups = {router: ''};
  for (const [, name, parent, prefix] of source.matchAll(/(\w+)\s*:=\s*(\w+)\.Group\("([^"]+)"\)/g)) {
    assert(parent in groups, `Unknown group parent: ${parent}`);
    groups[name] = groups[parent] + prefix;
  }
  for (const [, owner, method, route] of source.matchAll(/(\w+)\.(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\("([^"]+)"/g)) {
    assert(owner in groups, `Unknown route owner: ${owner}`);
    const key = `${service} ${method} ${groups[owner]}${route}`;
    assert(!actual.has(key), `Duplicate backend route: ${key}`);
    actual.add(key);
  }
  assert(!/\.(Any|Handle|Match)\(/.test(source), 'Extend route validation for new Gin routing constructs');
}
const requests = [];
function walk(node, folder) {
  for (const e of node.event || []) new vm.Script(e.script.exec.join('\n'));
  if (node.request) requests.push({node, folder});
  for (const child of node.item || []) walk(child, folder || child.name);
}
walk(collection);
const represented = new Set();
for (const {node, folder} of requests) {
  const r = node.request;
  assert.equal(r.auth.type, 'noauth');
  assert(node.event.some(e => e.listen === 'test'));
  const match = r.url.match(/^{{(\w+)BaseUrl}}(\/.*)$/);
  assert(match, `Unparameterized URL: ${r.url}`);
  const service = match[1] + '-service';
  assert(serviceNames.includes(service));
  if (!folder.includes('Health Checks')) assert(folder.toLowerCase().includes(match[1] + ' service'));
  const key = `${service} ${r.method} ${match[2].replace('{{apiVersion}}', 'v1')}`;
  assert(actual.has(key), `Nonexistent route: ${key}`);
  represented.add(key);
  if (r.body) assert(['{}', '{{registerBody}}'].includes(r.body.raw), 'Recheck body against DTO');
}
assert.deepEqual(represented, actual, 'Collection route coverage differs from backend');
for (const name of ['local','docker','development','staging','production']) {
  const env = JSON.parse(read(`postman/environments/${name}.postman_environment.json`));
  const values = Object.fromEntries(env.values.map(v => [v.key,v.value]));
  assert.equal(Object.keys(values).length, env.values.length, 'Duplicate environment keys');
  for (const key of ['registerEmail','registerPassword','accessToken','refreshToken','adminAccessToken','userAccessToken','userId']) assert.equal(values[key], '', `Populated private value: ${key}`);
  for (const service of serviceNames) assert(values[service.replace('-service','') + 'BaseUrl']);
  function resolve(s, stack=[]) {
    return s.replace(/{{(\w+)}}/g, (_, key) => {
      assert(key in values, `Missing ${key}`);
      assert(!stack.includes(key), `Circular ${key}`);
      return resolve(values[key], [...stack,key]);
    });
  }
  for (const {node} of requests) {
    const url = new URL(resolve(node.request.url));
    assert(['http:','https:'].includes(url.protocol));
    assert(!url.pathname.includes('//'));
  }
}
console.log(`Validated ${serviceNames.length} services, ${actual.size} distinct routes, ${requests.length} requests, five environments and JavaScript syntax.`);
