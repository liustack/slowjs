// Records every RegExp that the JavaScript engine of shiki 4.5.0 makes while
// it highlights the code blocks of input.md, and writes them to
// regexps.json. Run it in a folder where shiki@4.5.0 is installed:
//
//	pnpm add shiki@4.5.0
//	node record.mjs input.md regexps.json
import fs from 'node:fs'

// Every regexp is made through the RegExp global, shiki's subclass of it
// included, so a proxy of the global sees them all. It passes newTarget on,
// so that a subclass makes an object of its own class, and reads the
// pattern the engine compiled with the getter of RegExp, which the
// subclass overrides.
const seen = new Map()
const source = Object.getOwnPropertyDescriptor(RegExp.prototype, 'source').get
const flags = Object.getOwnPropertyDescriptor(RegExp.prototype, 'flags').get
globalThis.RegExp = new Proxy(globalThis.RegExp, {
  construct(t, a, newTarget) {
    const r = Reflect.construct(t, a, newTarget)
    const x = { source: source.call(r), flags: flags.call(r) }
    seen.set(x.source + '/' + x.flags, x)
    return r
  },
})
const { createHighlighterCoreSync } = await import('shiki/core')
const { createJavaScriptRegexEngine } = await import('shiki/engine/javascript')
const langs = await Promise.all(
  ['bash', 'css', 'go', 'html', 'json', 'typescript', 'yaml'].map((l) => import(`shiki/langs/${l}.mjs`).then((m) => m.default)),
)
const theme = (await import('shiki/themes/github-light.mjs')).default
const h = createHighlighterCoreSync({ themes: [theme], langs, engine: createJavaScriptRegexEngine() })
const [input, out] = process.argv.slice(2)
for (const m of fs.readFileSync(input, 'utf8').matchAll(/```(\w+)\n([\s\S]*?)```/g)) {
  h.codeToHtml(m[2], { lang: m[1] === 'ts' ? 'typescript' : m[1], theme: 'github-light' })
}
const list = [...seen.values()]
fs.writeFileSync(out, '[\n' + list.map((r) => JSON.stringify(r)).join(',\n') + '\n]\n')
console.log(list.length, 'regexps,', list.filter((r) => r.flags.includes('v')).length, 'with v')
