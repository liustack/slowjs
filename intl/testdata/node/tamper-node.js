// Runs the scripts of tamper.js in Node, each in a realm of its own, and
// prints their answers as json.
const fs = require('fs');
const vm = require('vm');
const ctx = vm.createContext({});
vm.runInContext(fs.readFileSync(__dirname + '/tamper.js', 'utf8'), ctx);
const r = {};
for (const [name, src] of ctx.scripts) {
	r[name] = vm.runInNewContext(src);
}
console.log(JSON.stringify(r, null, '\t'));
