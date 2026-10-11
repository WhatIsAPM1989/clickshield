import {copyFileSync, readFileSync, writeFileSync} from 'node:fs';
const copies=[['@rrweb/record/dist/record.umd.min.cjs','rrweb-record.js'],['@rrweb/replay/dist/replay.umd.min.cjs','rrweb-replay.js'],['@rrweb/replay/dist/style.css','rrweb-replay.css']];
for(const [source,target] of copies)copyFileSync(`node_modules/${source}`,`internal/httpapi/assets/${target}`);
// Upstream MIT notice is kept in assets/rrweb-LICENSE.txt alongside the vendored bundles.
JSON.parse(readFileSync('package-lock.json','utf8'));
