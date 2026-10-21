// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	gointl "github.com/liustack/quickjs/internal/gointl"
)

// The corpora of the constructors built on go-intl. Their locales are a
// sample of the 1005 tags of internal/gointl/qjsdata/chromium.json: every
// language a constructor has data for in Chromium, some regions and scripts
// of them, tags it falls back from, and Unicode extension keywords.

// chromiumTags are every step-th of the tags Chromium knows, from
// internal/gointl/qjsdata/chromium.json, starting at the first.
func chromiumTags(step int) []string {
	all := chromiumTagList()
	var r []string
	for i := 0; i < len(all); i += step {
		r = append(r, all[i])
	}
	return r
}

// validTags is javascript for the tags that are tags in the engine that
// runs it: chromium.json has a few, such as root, that Chromium refuses.
func validTags(tags []string) string {
	return jsList(tags...) + `.filter((t) => {try {Intl.getCanonicalLocales(t); return true} catch {return false}})`
}

func chromiumTagList() []string { return chromiumJSON().Tags }

// chromiumLanguages are the languages, tags of a language subtag alone,
// Chromium has for service: those it resolves to themselves.
func chromiumLanguages(service string) []string {
	cj := chromiumJSON()
	var r []string
	for _, tag := range cj.Tags {
		if r2, ok := cj.Services[service][tag]; !strings.Contains(tag, "-") && tag != "root" && (!ok || r2 == tag) {
			r = append(r, tag)
		}
	}
	return r
}

var chromiumJSON = sync.OnceValue(func() (v struct {
	Tags     []string
	Services map[string]map[string]string
}) {
	b, err := os.ReadFile(filepath.Join("..", "internal", "gointl", "qjsdata", "chromium.json"))
	if err != nil {
		panic(err)
	}

	if err := json.Unmarshal(b, &v); err != nil {
		panic(err)
	}

	return v
})

// collatorWords are strings that sort apart differently in the languages
// below: accents, case, letters some languages add or join, digits, spaces
// and punctuation, and other scripts.
var collatorWords = []string{
	"a", "A", "á", "Á", "à", "â", "ä", "Ä", "å", "Å", "æ", "ã", "b", "B", "c", "C", "ç", "č", "ch", "Ch", "CH", "cz",
	"d", "đ", "ð", "dž", "e", "E", "é", "è", "ê", "ë", "ě", "f", "g", "ğ", "h", "i", "I", "í", "ı", "İ", "j", "k", "l",
	"ll", "ł", "ly", "m", "n", "ñ", "ň", "ng", "o", "O", "ó", "ô", "ö", "Ö", "ø", "œ", "õ", "p", "q", "r", "ř", "s",
	"ś", "š", "ş", "ß", "ss", "sz", "t", "þ", "u", "ú", "ü", "Ü", "ů", "v", "w", "x", "y", "ý", "z", "ž", "ź", "ż",
	"0", "1", "2", "9", "10", "02", "١", "１", "½", " ", "-", "_", ".", "a b", "a-b", "a.b", "ab", "aB", "Ab", "ab-",
	"@", "#", "€", "$", "%", "😀", "👍", "中", "国", "日", "本", "汉", "字", "一", "二", "三", "张", "李", "王",
	"한", "글", "가", "나", "あ", "い", "ア", "イ", "か", "が", "カ", "ガ", "ㄅ", "ㄆ", "α", "Ω", "ά", "б", "Б", "ё",
	"е", "я", "ї", "і", "ґ", "א", "ב", "ب", "پ", "ک", "ی", "ก", "ข", "เก", "แ", "क", "ख", "অ", "த", "ಕ", "ക", "ગ",
	"ते", "ኣ", "ა", "ա", "côte", "cote", "coté", "côté", "résumé", "resume", "Resume", "coop", "co-op",
}

// collatorPairs are pairs of strings, as javascript source, whose
// comparison shows which differences count: lone surrogates, a NUL and
// decomposed accents among them.
const collatorPairs = `[['a','b'],['a','á'],['a','A'],['a','Á'],['á','A'],['a','ä'],['ä','ae'],['ß','ss'],['æ','ae'],` +
	`['ı','i'],['İ','i'],['I','ı'],['ch','c'],['ch','h'],['ch','i'],['ll','l'],['ll','m'],['ñ','n'],['ñ','o'],` +
	`['å','z'],['ö','z'],['ø','o'],['v','w'],['a b','ab'],['a-b','ab'],['a.b','ab'],['','a'],['',''],['a','a'],` +
	`['2','10'],['02','2'],['1','１'],['١','1'],['½','1/2'],['e\u0301','é'],['\u212b','Å'],['ｶ','カ'],['か','カ'],` +
	`['か','が'],['中','国'],['汉','字'],['张','李'],['가','나'],['α','a'],['е','ё'],['Б','б'],['😀','👍'],['😀','a'],` +
	`['\ud800','a'],['\ud800','\udc00'],['\udc00','\ud800'],['x\u0000y','xy'],['côte','coté'],['resume','résumé'],` +
	`['Resume','resume']]`

// collatorLocales are the locales the sorts and comparisons are in.
var collatorLocales = []string{
	"en", "en-US", "en-GB", "de", "de-AT", "de-u-co-phonebk", "fr", "fr-CA", "es", "es-u-co-trad", "it", "pt",
	"nl", "sv", "sv-u-co-reformed", "da", "nb", "no", "fi", "et", "is", "pl", "cs", "sk", "sl", "hr", "sr",
	"sr-Latn", "bs", "hu", "ro", "lt", "lv", "tr", "az", "ru", "uk", "bg", "el", "he", "ar", "fa", "ur", "hi",
	"mr", "bn", "gu", "ta", "te", "kn", "ml", "th", "vi", "id", "ms", "fil", "sw", "af", "am", "ca", "ja",
	"ja-u-co-unihan", "ko", "ko-u-co-searchjl", "zh", "zh-TW", "zh-HK", "zh-u-co-stroke", "zh-u-co-zhuyin",
	"zh-Hant-u-co-pinyin", "zh-u-co-unihan", "en-u-co-emoji", "en-u-co-eor", "root", "und", "xx", "my", "ka",
}

// collatorTags are those whose resolved options the corpus records.
var collatorTags = []string{
	"af", "am", "ar", "ar-SA", "ar-EG", "bg", "bn", "ca", "cs", "da", "de", "de-AT", "de-CH", "el", "en", "en-US",
	"en-GB", "en-US-u-va-posix", "es", "es-MX", "et", "fa", "fa-AF", "fi", "fil", "fr", "fr-CA", "fr-BE", "gu",
	"he", "he-IL", "hi", "hr", "hu", "id", "id-ID", "in", "it", "iw", "ja", "ja-JP", "kn", "ko", "ko-KR", "lt",
	"lv", "ml", "mo", "mr", "ms", "nb", "nb-NO", "nn", "nl", "no", "pl", "pt", "pt-BR", "pt-PT", "ro", "root",
	"ru", "sh", "sk", "sl", "sr", "sr-Cyrl", "sr-Latn", "sr-ME", "sv", "sv-FI", "sw", "ta", "te", "th", "tl",
	"tr", "uk", "ur", "vi", "zh", "zh-CN", "zh-HK", "zh-Hans", "zh-Hant", "zh-Hant-TW", "zh-MO", "zh-SG",
	"zh-TW", "yue", "yue-Hant", "cy", "eu", "ga", "is", "ka", "kk", "km", "lo", "mn", "my", "ne", "pa", "si",
	"sq", "uz", "zu", "agq", "xx", "und", "x-private", "EN-us",
	"de-u-co-phonebk", "de-AT-u-co-phonebk", "de-u-co-eor", "es-u-co-trad", "sv-u-co-reformed", "sv-u-co-standard",
	"zh-u-co-pinyin", "zh-u-co-stroke", "zh-u-co-zhuyin", "zh-u-co-unihan", "zh-u-co-big5han", "zh-TW-u-co-pinyin",
	"ja-u-co-unihan", "ko-u-co-searchjl", "en-u-co-search", "en-u-co-standard", "en-u-co-emoji", "en-u-co-xyz",
	"en-u-kn", "en-u-kn-true", "en-u-kn-false", "en-u-kf-upper", "en-u-kf-lower", "en-u-kf-false",
	"en-u-kn-kf-upper", "de-u-co-phonebk-kn", "th-u-ka-noignore", "en-u-ks-level1", "en-u-ca-japanese",
	"en-US-u-co-emoji-kn-true-nu-arab",
}

// collatorOptions are option bags, each on its own and with the keywords
// above.
var collatorOptions = []string{
	`{}`, `{usage:'search'}`, `{usage:'sort'}`, `{sensitivity:'base'}`, `{sensitivity:'accent'}`,
	`{sensitivity:'case'}`, `{sensitivity:'variant'}`, `{numeric:true}`, `{numeric:false}`, `{numeric:0}`,
	`{caseFirst:'upper'}`, `{caseFirst:'lower'}`, `{caseFirst:'false'}`, `{ignorePunctuation:true}`,
	`{ignorePunctuation:false}`, `{collation:'phonebk'}`, `{collation:'pinyin'}`, `{collation:'stroke'}`,
	`{collation:'trad'}`, `{collation:'emoji'}`, `{collation:'eor'}`, `{collation:'standard'}`,
	`{collation:'search'}`, `{collation:'unknown'}`, `{usage:'search',sensitivity:'base'}`,
	`{usage:'search',collation:'phonebk'}`, `{numeric:true,caseFirst:'upper',sensitivity:'case'}`,
	`{localeMatcher:'lookup'}`,
}

func collatorExprs() (r []string) {
	add := func(format string, args ...any) { r = append(r, fmt.Sprintf(format, args...)) }
	for _, tag := range collatorTags {
		add(`JSON.stringify(new Intl.Collator(%q).resolvedOptions())`, tag)
	}
	for _, tag := range []string{"en", "de", "de-u-co-phonebk", "zh", "zh-TW", "ja", "th", "sv", "en-u-kn-kf-upper", "es-u-co-trad", "xx"} {
		for _, o := range collatorOptions[1:] {
			add(`JSON.stringify(new Intl.Collator(%q,%s).resolvedOptions())`, tag, o)
		}
	}

	words := jsList(collatorWords...)
	for _, l := range collatorLocales {
		for _, o := range []string{`{}`, `{sensitivity:'base'}`, `{numeric:true}`, `{caseFirst:'upper'}`, `{caseFirst:'lower'}`,
			`{ignorePunctuation:true}`, `{usage:'search'}`, `{sensitivity:'accent'}`, `{sensitivity:'case'}`} {
			add(`JSON.stringify(%s.sort(new Intl.Collator(%q,%s).compare))`, words, l, o)
		}
	}

	// Every language Chromium has collators for.
	for _, l := range chromiumLanguages("Collator") {
		add(`JSON.stringify(%s.sort(new Intl.Collator(%q).compare))`, words, l)
	}

	// Which differences count, pair by pair.
	for _, l := range []string{"en", "de", "de-u-co-phonebk", "sv", "da", "es", "es-u-co-trad", "cs", "tr", "fr-CA", "ja", "zh", "zh-TW", "th", "ar"} {
		for _, o := range []string{`{}`, `{sensitivity:'base'}`, `{sensitivity:'accent'}`, `{sensitivity:'case'}`, `{usage:'search'}`,
			`{usage:'search',sensitivity:'base'}`, `{numeric:true}`, `{ignorePunctuation:true}`} {
			add(`(() => {const c = new Intl.Collator(%q,%s).compare; return %s.map(([x, y]) => c(x, y)).join()})()`, l, o, collatorPairs)
		}
	}

	// localeCompare takes the Collator's arguments.
	for _, e := range []string{
		`'a'.localeCompare('b')`, `'b'.localeCompare('a')`, `'a'.localeCompare('a')`, `'a'.localeCompare('A')`,
		`'a'.localeCompare('á')`, `'a'.localeCompare('á', 'en', {sensitivity:'base'})`, `'ä'.localeCompare('z', 'de')`,
		`'ä'.localeCompare('z', 'sv')`, `'2'.localeCompare('10')`, `'2'.localeCompare('10', undefined, {numeric:true})`,
		`'2'.localeCompare('10', 'en-u-kn')`, `'a'.localeCompare('B', 'en', {caseFirst:'upper'})`, `'a'.localeCompare()`,
		`'undefined'.localeCompare()`, `'a'.localeCompare('b', ['xx', 'de'])`, `'ch'.localeCompare('h', 'cs')`,
		`String.prototype.localeCompare.call(1, 2)`, `String.prototype.localeCompare.call(true, 'true')`,
		`'中'.localeCompare('国', 'zh')`, `'中'.localeCompare('国', 'zh-u-co-stroke')`, `'a'.localeCompare('b', [])`,
		`['b', 'a', 'C', 'ä'].sort((x, y) => x.localeCompare(y)).join()`, `['b', 'a', 'C', 'ä'].sort((x, y) => x.localeCompare(y, 'sv')).join()`,
		`Intl.Collator().compare('a', 'b')`, `Intl.Collator('de') instanceof Intl.Collator`,
		`new Intl.Collator().compare.call(null, 'b', 'a')`, `new Intl.Collator().compare(undefined, 'undefined')`,
		`new Intl.Collator().compare(1, '1')`, `new Intl.Collator().compare(null, 'null')`,
	} {
		add(`%s`, e)
	}

	// supportedLocalesOf and errors.
	add(`JSON.stringify(Intl.Collator.supportedLocalesOf(%s))`, jsList(collatorTags[:40]...))
	add(`JSON.stringify(Intl.Collator.supportedLocalesOf(%s))`, jsList(collatorTags[40:]...))
	add(`JSON.stringify(Intl.Collator.supportedLocalesOf(%s,{localeMatcher:'lookup'}))`, jsList("de-AT", "xx", "zh-Hant-TW", "sr-Latn-RS", "nb-NO"))
	for _, src := range []string{
		`new Intl.Collator('en-')`, `new Intl.Collator(null)`, `new Intl.Collator('en', null)`, `new Intl.Collator('en', 5)`,
		`new Intl.Collator('en', {usage:'x'})`, `new Intl.Collator('en', {sensitivity:'x'})`, `new Intl.Collator('en', {caseFirst:'x'})`,
		`new Intl.Collator('en', {caseFirst:true})`, `new Intl.Collator('en', {localeMatcher:'x'})`, `new Intl.Collator('en', {collation:'ab'})`,
		`new Intl.Collator('en', {collation:'abc-de'})`, `new Intl.Collator('en', {collation:'abcdefghi'})`, `new Intl.Collator('en', {collation:''})`,
		`new Intl.Collator('en', {collation:'abc_def'})`, `Intl.Collator.prototype.compare`, `Intl.Collator.prototype.resolvedOptions.call({})`,
		`Object.getOwnPropertyDescriptor(Intl.Collator.prototype, 'compare').get.call(new Intl.NumberFormat())`,
		`String.prototype.localeCompare.call(null, 'a')`, `String.prototype.localeCompare.call(undefined, 'a')`,
		`'a'.localeCompare(Symbol())`, `'a'.localeCompare('b', 'en-')`, `'a'.localeCompare('b', 'en', {usage:'x'})`,
	} {
		add(`(() => {try {%s; return 'ok'} catch (e) {return e.constructor.name}})()`, src)
	}
	return r
}

// segmenterTexts are texts, as javascript source, whose breaks differ by
// granularity and language: punctuation and abbreviations, numbers, emoji
// sequences and flags, combining marks, scripts that break by dictionary
// (Thai, Lao, Khmer, Burmese, Chinese and Japanese), Indic conjuncts, line
// ends, and lone surrogates.
var segmenterTexts = []string{
	`''`, `'a'`, `'Hello, world! How are you?'`, `'The U.S.A. is big. Mr. Smith went to Washington. Dr. Who?'`,
	`'It costs $3.14, or 3,000.50 euros (approx.)'`, `"don't stop can't won't l'homme"`, `'e-mail co-op 2026-10-21 10:30 a_b'`,
	`'👨‍👩‍👧‍👦 family 🇺🇸🇩🇪 flags 👍🏽 thumbs ❤️'`, `'e\u0301 a\u0308 n\u0303 \u1100\u1161\u11a8'`,
	`'สวัสดีครับ ยินดีต้อนรับ'`, `'ສະບາຍດີ ຍິນດີຕ້ອນຮັບ'`, `'សួស្តី ពិភពលោក'`, `'မင်္ဂလာပါ ကမ္ဘာ'`,
	`'我们今天去北京大学看看。你好吗？'`, `'私は東京に住んでいます。カタカナとひらがな、漢字。'`, `'안녕하세요 세계. 반갑습니다!'`,
	`'नमस्ते दुनिया। क्षत्रिय स्त्री'`, `'হ্যালো বিশ্ব। ক্ষ'`, `'مرحبا بالعالم. كيف حالك؟'`, `'שלום עולם. מה שלומך?'`,
	`'Привет, мир! Как дела?'`, `'Γειά σου κόσμε; Τι κάνεις;'`, `'line one\r\nline two\nline three\r'`,
	`'tab\there  two  spaces'`, `'a\ud800b\udc00c'`, `'\ud83d'`, `'x\ud83d\ude00y'`, `'Straße Öl über'`,
	`'«Bonjour», dit-il. «Ça va?»'`, `'1. First. 2. Second.'`, `'Hello.World. Next'`, `'ABC123 abc_123 a1b2'`,
}

// segmenterLocales are the locales the texts are broken in.
var segmenterLocales = []string{
	"en", "en-US", "de", "fr", "it", "es", "ru", "el", "fi", "sv", "ja", "zh", "zh-Hant", "ko", "th", "lo", "km",
	"my", "hi", "bn", "ar", "he", "tr", "root", "xx", "en-u-ss-standard", "de-u-ss-standard",
}

func segmenterExprs() (r []string) {
	add := func(format string, args ...any) { r = append(r, fmt.Sprintf(format, args...)) }
	tags := append(chromiumTags(5), "und", "xx", "EN-us", "en-u-ss-standard", "en-u-co-phonebk", "zh-u-ca-chinese",
		"ja-JP-u-nu-arab", "x-private", "sr-ME", "iw", "in", "mo")
	for _, tag := range tags {
		add(`JSON.stringify(new Intl.Segmenter(%q).resolvedOptions())`, tag)
	}
	for _, tag := range []string{"en", "de", "ja", "zh", "th", "xx"} {
		for _, o := range []string{`{granularity:'grapheme'}`, `{granularity:'word'}`, `{granularity:'sentence'}`, `{localeMatcher:'lookup'}`,
			`{localeMatcher:'best fit',granularity:'word'}`, `{granularity:undefined}`, `undefined`} {
			add(`JSON.stringify(new Intl.Segmenter(%q,%s).resolvedOptions())`, tag, o)
		}
	}

	// Each text broken up in each locale, at each granularity.
	for _, l := range segmenterLocales {
		for _, g := range []string{"grapheme", "word", "sentence"} {
			for _, text := range segmenterTexts {
				add(`JSON.stringify([...new Intl.Segmenter(%q,{granularity:%q}).segment(%s)].map((s) => [s.segment, s.index, s.isWordLike]))`, l, g, text)
			}
		}
	}

	// containing at offsets inside and outside the text, and not numbers.
	for _, g := range []string{"grapheme", "word", "sentence"} {
		for _, text := range []string{`'Hello, world! How are you?'`, `'x\ud83d\ude00y 👨‍👩‍👧 z'`, `'我们今天去北京'`} {
			add(`(() => {const s = new Intl.Segmenter('en',{granularity:%q}).segment(%s); return JSON.stringify([-1, 0, 1, 2, 3, 5, 7, 8, 13, 14, 25, 26, 1.5, 2.9, -0.5, '3', NaN, undefined, null, true, Infinity, -Infinity, 1e10].map((i) => {const d = s.containing(i); return d === undefined ? 'undefined' : [d.segment, d.index, d.isWordLike, d.input === s.containing(0).input]}))})()`, g, text)
		}
	}
	for _, e := range []string{
		`new Intl.Segmenter().segment('ab').containing() .segment`,
		`new Intl.Segmenter('en', {granularity:'word'}).segment('ab cd').containing({valueOf() { return 3; }}).segment`,
		`(() => {const it = new Intl.Segmenter().segment('ab')[Symbol.iterator](); return JSON.stringify([it.next(), it.next(), it.next(), it.next()])})()`,
		`(() => {const s = new Intl.Segmenter().segment('abc'), a = s[Symbol.iterator](), b = s[Symbol.iterator](); a.next(); return a.next().value.segment + b.next().value.segment})()`,
		`(() => {const s = new Intl.Segmenter().segment('abc'); return [...s].length + [...s].length})()`,
		`(() => {const it = new Intl.Segmenter().segment('a')[Symbol.iterator](); return it[Symbol.iterator]() === it})()`,
		`Object.prototype.toString.call(new Intl.Segmenter().segment('a')[Symbol.iterator]())`,
		`Object.prototype.toString.call(new Intl.Segmenter().segment('a'))`,
		`Object.prototype.toString.call(new Intl.Segmenter())`,
		`new Intl.Segmenter().segment(12345).containing(2).input`,
		`new Intl.Segmenter().segment(undefined).containing(0).input`,
		`new Intl.Segmenter().segment(null).containing(0).segment`,
		`new Intl.Segmenter().segment({toString() { return 'xy'; }}).containing(1).segment`,
		`JSON.stringify(new Intl.Segmenter('en', {granularity:'word'}).segment('a b').containing(1))`,
		`JSON.stringify(new Intl.Segmenter('en', {granularity:'sentence'}).segment('a. b').containing(0))`,
		`JSON.stringify(Object.keys(new Intl.Segmenter('en', {granularity:'word'}).segment('a').containing(0)))`,
		`JSON.stringify(Object.keys(new Intl.Segmenter('en').segment('a').containing(0)))`,
		`(() => {const s = new Intl.Segmenter().segment('ab'); return Object.getPrototypeOf(s.containing(0)) === Object.prototype})()`,
		`JSON.stringify(Intl.Segmenter.supportedLocalesOf(['en', 'de', 'xx', 'th', 'zh-Hant-TW', 'ja-JP', 'km', 'tlh']))`,
		`JSON.stringify(Intl.Segmenter.supportedLocalesOf(['en-US', 'yue', 'zu'], {localeMatcher:'lookup'}))`,
		`JSON.stringify(Intl.Segmenter.supportedLocalesOf())`,
	} {
		add(`%s`, e)
	}
	add(`JSON.stringify(Intl.Segmenter.supportedLocalesOf(%s))`, validTags(chromiumTags(3)))

	for _, src := range []string{
		`Intl.Segmenter()`, `Intl.Segmenter('en')`, `new Intl.Segmenter('en-')`, `new Intl.Segmenter(null)`,
		`new Intl.Segmenter('en', null)`, `new Intl.Segmenter('en', 5)`, `new Intl.Segmenter('en', 'word')`, `new Intl.Segmenter('en', true)`,
		`new Intl.Segmenter('en', {granularity:'line'})`, `new Intl.Segmenter('en', {granularity:'x'})`, `new Intl.Segmenter('en', {localeMatcher:'x'})`,
		`new Intl.Segmenter('en', {granularity:{toString() { throw new RangeError() }}})`,
		`Intl.Segmenter.prototype.segment('a')`, `Intl.Segmenter.prototype.resolvedOptions()`,
		`Intl.Segmenter.prototype.segment.call(new Intl.Collator(), 'a')`, `new Intl.Segmenter().segment(Symbol())`,
		`new Intl.Segmenter().segment('a').containing(Symbol())`, `new Intl.Segmenter().segment('a').containing(1n)`,
		`Object.getPrototypeOf(new Intl.Segmenter().segment('a')).containing.call({}, 0)`,
		`Object.getPrototypeOf(new Intl.Segmenter().segment('a'))[Symbol.iterator].call({})`,
		`Object.getPrototypeOf(new Intl.Segmenter().segment('a')[Symbol.iterator]()).next.call({})`,
		`Intl.Segmenter.supportedLocalesOf('en', null)`, `Intl.Segmenter.supportedLocalesOf('en', {localeMatcher:'x'})`,
	} {
		add(`(() => {try {%s; return 'ok'} catch (e) {return e.constructor.name}})()`, src)
	}
	return r
}

// rtfLocales are the locales relative times are written in: every language
// tifa users are likely to write and a spread of the others.
var rtfLocales = []string{
	"en", "en-US", "en-GB", "en-IN", "en-AU", "de", "de-AT", "fr", "fr-CA", "es", "es-MX", "it", "pt", "pt-PT",
	"nl", "sv", "da", "nb", "fi", "pl", "cs", "sk", "sl", "hr", "sr", "sr-Latn", "hu", "ro", "lt", "lv", "et",
	"tr", "az", "ru", "uk", "be", "bg", "el", "he", "ar", "ar-EG", "fa", "ur", "hi", "mr", "bn", "ta", "te", "kn",
	"ml", "th", "vi", "id", "ms", "fil", "sw", "zu", "af", "am", "ca", "eu", "gl", "ga", "cy", "is", "ka", "hy",
	"kk", "uz", "mn", "km", "lo", "my", "ne", "si", "ja", "ko", "zh", "zh-TW", "zh-HK", "yue", "root", "xx",
	"en-u-nu-arab", "ar-u-nu-latn", "hi-u-nu-deva", "th-u-nu-thai", "zh-u-nu-hanidec",
	// Locales Chromium has for RelativeTimeFormat with no relative time data
	// of their own, or whose parent has none.
	"az-Cyrl", "bs", "ckb-IR", "ee-TG", "ha-GH", "kk-CN", "kok", "ne-IN", "om-KE", "pa", "sd", "so-KE", "sq-MK",
	"st-LS", "ti-ER", "uz-Arab", "uz-AF",
}

func rtfExprs() (r []string) {
	add := func(format string, args ...any) { r = append(r, fmt.Sprintf(format, args...)) }
	tags := append(chromiumTags(4), "und", "xx", "EN-us", "en-u-nu-arab", "en-u-nu-xyz", "en-u-ca-japanese", "ar-u-nu-latn",
		"zh-u-nu-hanidec", "x-private", "sr-ME", "iw", "in", "mo", "de-u-co-phonebk-nu-thai")
	for _, tag := range tags {
		add(`JSON.stringify(new Intl.RelativeTimeFormat(%q).resolvedOptions())`, tag)
	}
	for _, tag := range []string{"en", "ar", "zh", "hi", "xx"} {
		for _, o := range []string{`{style:'long'}`, `{style:'short'}`, `{style:'narrow'}`, `{numeric:'auto'}`, `{numeric:'always'}`,
			`{numberingSystem:'arab'}`, `{numberingSystem:'latn'}`, `{numberingSystem:'thai'}`, `{numberingSystem:'xyz'}`,
			`{numberingSystem:'hanidec'}`, `{localeMatcher:'lookup'}`, `{style:'narrow',numeric:'auto',numberingSystem:'deva'}`,
			`null`, `5`, `'x'`, `undefined`} {
			add(`JSON.stringify(new Intl.RelativeTimeFormat(%q,%s).resolvedOptions())`, tag, o)
		}
	}

	// Every unit, at offsets with and without words, in every style.
	const values = `[-2, -1, -0, 0, 1, 2, 3, 1.5, -1.5, 0.5, 10, 21, 100, 1000, 12345.678, -1e6, 0.001, 1.004, -1.006]`
	const units = `['year', 'quarter', 'month', 'week', 'day', 'hour', 'minute', 'second']`
	for _, l := range rtfLocales {
		for _, style := range []string{"long", "short", "narrow"} {
			for _, numeric := range []string{"always", "auto"} {
				add(`(() => {const f = new Intl.RelativeTimeFormat(%q,{style:%q,numeric:%q}); return %s.map((u) => %s.map((v) => f.format(v, u)).join('|')).join('\n')})()`,
					l, style, numeric, units, values)
			}
		}
	}

	// Every language Chromium has relative times for.
	for _, l := range chromiumLanguages("RelativeTimeFormat") {
		add(`(() => {const f = new Intl.RelativeTimeFormat(%q, {numeric: 'auto'}); return [f.format(-1, 'day'), f.format(2, 'hours'), f.format(-1234.5, 'months'), new Intl.RelativeTimeFormat(%q, {style: 'narrow'}).format(3, 'years')].join('|')})()`, l, l)
	}

	// The parts, for some locales.
	for _, l := range []string{"en", "de", "ar", "zh", "ja", "fr", "hi", "ru", "he", "en-u-nu-arab"} {
		for _, numeric := range []string{"always", "auto"} {
			add(`(() => {const f = new Intl.RelativeTimeFormat(%q,{numeric:%q}); return JSON.stringify([[-1,'day'],[0,'day'],[1,'days'],[2,'hours'],[1.5,'week'],[-1234.5,'minutes'],[100000,'seconds'],[-0,'year'],[3,'quarters'],[-2,'months']].map(([v, u]) => f.formatToParts(v, u)))})()`, l, numeric)
		}
	}

	for _, e := range []string{
		`new Intl.RelativeTimeFormat().format(1, 'days')`, `new Intl.RelativeTimeFormat().format('1', 'day')`,
		`new Intl.RelativeTimeFormat().format({valueOf() { return 2; }}, {toString() { return 'week'; }})`,
		`new Intl.RelativeTimeFormat().format(true, 'day')`, `new Intl.RelativeTimeFormat().format(null, 'day')`,
		`new Intl.RelativeTimeFormat().format(1e21, 'day')`, `new Intl.RelativeTimeFormat().format(-1e-7, 'day')`,
		`new Intl.RelativeTimeFormat('en', {numeric:'auto'}).format(-0, 'day')`, `new Intl.RelativeTimeFormat('en', {numeric:'auto'}).format(0, 'second')`,
		`new Intl.RelativeTimeFormat('en', {numeric:'auto'}).format(1.0, 'year')`, `new Intl.RelativeTimeFormat('en', {numeric:'auto'}).format(1.001, 'year')`,
		`new Intl.RelativeTimeFormat('en', {numeric:'auto'}).format(0.0001, 'day')`, `new Intl.RelativeTimeFormat('en', {numeric:'auto'}).format(-0.004, 'day')`,
		`JSON.stringify(Intl.RelativeTimeFormat.supportedLocalesOf(['en', 'de', 'xx', 'zh-Hant-TW', 'tlh', 'yue']))`,
		`JSON.stringify(Intl.RelativeTimeFormat.supportedLocalesOf(['en-US', 'ar-EG'], {localeMatcher:'lookup'}))`,
		`Object.prototype.toString.call(new Intl.RelativeTimeFormat())`,
	} {
		add(`%s`, e)
	}
	add(`JSON.stringify(Intl.RelativeTimeFormat.supportedLocalesOf(%s))`, validTags(chromiumTags(3)))

	for _, src := range []string{
		`Intl.RelativeTimeFormat()`, `new Intl.RelativeTimeFormat('en-')`, `new Intl.RelativeTimeFormat(null)`,
		`new Intl.RelativeTimeFormat('en', null)`, `new Intl.RelativeTimeFormat('en', {style:'x'})`, `new Intl.RelativeTimeFormat('en', {numeric:'x'})`,
		`new Intl.RelativeTimeFormat('en', {numberingSystem:'ab'})`, `new Intl.RelativeTimeFormat('en', {numberingSystem:'abc-defghijkl'})`,
		`new Intl.RelativeTimeFormat('en', {localeMatcher:'x'})`, `new Intl.RelativeTimeFormat().format(1, 'x')`,
		`new Intl.RelativeTimeFormat().format(1, 'days ')`, `new Intl.RelativeTimeFormat().format(1, 'Day')`, `new Intl.RelativeTimeFormat().format(1)`,
		`new Intl.RelativeTimeFormat().format(NaN, 'day')`, `new Intl.RelativeTimeFormat().format(Infinity, 'day')`,
		`new Intl.RelativeTimeFormat().format(1n, 'day')`, `new Intl.RelativeTimeFormat().format(Symbol(), 'day')`,
		`new Intl.RelativeTimeFormat().format(1, Symbol())`, `new Intl.RelativeTimeFormat().formatToParts(NaN, 'day')`,
		`new Intl.RelativeTimeFormat().format(1, 'millisecond')`, `new Intl.RelativeTimeFormat().format(1, 'weekss')`,
		`Intl.RelativeTimeFormat.prototype.format(1, 'day')`, `Intl.RelativeTimeFormat.prototype.resolvedOptions()`,
		`Intl.RelativeTimeFormat.prototype.formatToParts.call(new Intl.NumberFormat(), 1, 'day')`,
		`Intl.RelativeTimeFormat.supportedLocalesOf('en', null)`,
	} {
		add(`(() => {try {%s; return 'ok'} catch (e) {return e.constructor.name}})()`, src)
	}
	return r
}

// listLocales are the locales lists are joined in.
var listLocales = []string{
	"en", "en-US", "en-GB", "en-IN", "de", "fr", "fr-CA", "es", "es-MX", "it", "pt", "pt-PT", "nl", "sv", "da",
	"nb", "fi", "pl", "cs", "sk", "sl", "hr", "sr", "sr-Latn", "hu", "ro", "lt", "lv", "et", "tr", "ru", "uk",
	"bg", "el", "he", "ar", "fa", "ur", "hi", "mr", "bn", "ta", "te", "kn", "ml", "gu", "th", "vi", "id", "ms",
	"fil", "sw", "zu", "af", "am", "ca", "ja", "ko", "zh", "zh-TW", "zh-HK", "kk", "az", "uz", "km", "lo", "my",
	"ka", "hy", "mn", "ne", "si", "yue", "root", "xx",
}

func listExprs() (r []string) {
	add := func(format string, args ...any) { r = append(r, fmt.Sprintf(format, args...)) }
	tags := append(chromiumTags(4), "und", "xx", "EN-us", "en-u-nu-arab", "en-u-ca-japanese", "x-private", "sr-ME",
		"iw", "in", "mo", "es-u-co-trad")
	for _, tag := range tags {
		add(`JSON.stringify(new Intl.ListFormat(%q).resolvedOptions())`, tag)
	}
	for _, tag := range []string{"en", "es", "he", "xx"} {
		for _, o := range []string{`{type:'conjunction'}`, `{type:'disjunction'}`, `{type:'unit'}`, `{style:'long'}`,
			`{style:'short'}`, `{style:'narrow'}`, `{type:'unit',style:'narrow'}`, `{localeMatcher:'lookup'}`, `undefined`} {
			add(`JSON.stringify(new Intl.ListFormat(%q,%s).resolvedOptions())`, tag, o)
		}
	}

	// Lists of every length in every locale, type and style.
	const lists = `[[], ['a'], ['a', 'b'], ['a', 'b', 'c'], ['one', 'two', 'three', 'four'], ['1', '2', '3', '4', '5', '6']]`
	for _, l := range listLocales {
		for _, typ := range []string{"conjunction", "disjunction", "unit"} {
			for _, style := range []string{"long", "short", "narrow"} {
				add(`(() => {const f = new Intl.ListFormat(%q,{type:%q,style:%q}); return %s.map((l) => f.format(l)).join('|')})()`, l, typ, style, lists)
			}
		}
	}

	// Every language Chromium has lists for.
	for _, l := range chromiumLanguages("ListFormat") {
		add(`(() => {const l = ['a', 'b', 'c']; return ['conjunction', 'disjunction', 'unit'].map((type) => new Intl.ListFormat(%q, {type}).format(l)).join('|')})()`, l)
	}

	// Joins that depend on the next item, empty items, and lone surrogates.
	for _, l := range []string{"es", "he", "en", "de", "zh"} {
		for _, typ := range []string{"conjunction", "disjunction", "unit"} {
			add(`(() => {const f = new Intl.ListFormat(%q,{type:%q}); return JSON.stringify([['agua', 'hielo'], ['agua', 'Iglesia'], ['agua', 'hierro'], ['agua', 'hiato'], ['siete', 'ocho'], ['siete', 'Hora'], ['siete', 'otro', 'once'], ['a', '11'], ['a', '8'], ['a', '800'], ['a', 'ib'], ['שלום', 'world'], ['שלום', 'עולם'], ['שלום', '1'], ['', ''], ['a', '', 'b'], ['', 'a'], ['\ud800', 'x\udc00'], ['a\u0000', 'b']].map((l) => [f.format(l), f.formatToParts(l)]))})()`, l, typ)
		}
	}

	for _, e := range []string{
		`new Intl.ListFormat().format()`, `new Intl.ListFormat().format(undefined)`, `new Intl.ListFormat().format('abc')`,
		`new Intl.ListFormat().format(new Set(['x', 'y', 'x']))`, `new Intl.ListFormat().format(new Map([['a', 1]]).keys())`,
		`new Intl.ListFormat().format({[Symbol.iterator]: function* () { yield 'p'; yield 'q'; }})`,
		`JSON.stringify(new Intl.ListFormat().formatToParts(['a', 'b']))`, `JSON.stringify(new Intl.ListFormat().formatToParts([]))`,
		`JSON.stringify(Intl.ListFormat.supportedLocalesOf(['en', 'de', 'xx', 'zh-Hant-TW', 'kk', 'az']))`,
		`JSON.stringify(Intl.ListFormat.supportedLocalesOf(['en-US', 'ar-EG'], {localeMatcher:'lookup'}))`,
		`Object.prototype.toString.call(new Intl.ListFormat())`,
	} {
		add(`%s`, e)
	}
	add(`JSON.stringify(Intl.ListFormat.supportedLocalesOf(%s))`, validTags(chromiumTags(3)))

	for _, src := range []string{
		`Intl.ListFormat()`, `new Intl.ListFormat('en-')`, `new Intl.ListFormat(null)`, `new Intl.ListFormat('en', null)`,
		`new Intl.ListFormat('en', 5)`, `new Intl.ListFormat('en', 'x')`, `new Intl.ListFormat('en', {type:'x'})`,
		`new Intl.ListFormat('en', {style:'x'})`, `new Intl.ListFormat('en', {localeMatcher:'x'})`,
		`new Intl.ListFormat().format(null)`, `new Intl.ListFormat().format(5)`, `new Intl.ListFormat().format(['a', 1])`,
		`new Intl.ListFormat().format([undefined])`, `new Intl.ListFormat().format([new String('a')])`,
		`new Intl.ListFormat().format({})`, `new Intl.ListFormat().formatToParts(['a', null])`,
		`Intl.ListFormat.prototype.format([])`, `Intl.ListFormat.prototype.resolvedOptions()`,
		`Intl.ListFormat.prototype.formatToParts.call(new Intl.NumberFormat(), [])`,
		`Intl.ListFormat.supportedLocalesOf('en', null)`,
	} {
		add(`(() => {try {%s; return 'ok'} catch (e) {return e.constructor.name}})()`, src)
	}
	return r
}

// The codes DisplayNames names, of each type: common and rare ones, codes
// with no name, and codes in other cases and forms.
var (
	dnLanguages = `['en', 'en-US', 'en-GB', 'en-AU', 'en-CA', 'en-IN', 'fr', 'fr-CA', 'fr-CH', 'de', 'de-AT', 'de-CH', 'es', 'es-419', 'es-MX', ` +
		`'es-ES', 'pt', 'pt-BR', 'pt-PT', 'zh', 'zh-Hans', 'zh-Hant', 'zh-CN', 'zh-TW', 'zh-HK', 'zh-Hant-HK', 'yue', 'ja', 'ko', ` +
		`'ar', 'ar-001', 'he', 'iw', 'hi', 'bn', 'ru', 'uk', 'sr', 'sr-Latn', 'sr-Cyrl-ME', 'nl', 'nl-BE', 'sv', 'nb', 'no', 'nn', ` +
		`'mo', 'ro-MD', 'sw-CD', 'ca-ES-valencia', 'en-Latn-US', 'tlh', 'qaa', 'xyz', 'und', 'root', 'EN-us', 'zh-hant-tw', ` +
		`'fil', 'tl', 'in', 'ms-Arab', 'az-Arab', 'uz-Cyrl', 'pa-Arab', 'ks-Deva', 'en-US-posix', 'de-1996', 'sl-rozaj', 'arb', 'cmn']`
	dnRegions = `['US', 'GB', 'DE', 'FR', 'CN', 'TW', 'HK', 'MO', 'JP', 'KR', 'KP', 'IN', 'RU', 'UA', 'BR', 'MX', 'PS', 'EH', ` +
		`'XK', 'MK', 'CZ', 'SZ', 'CI', 'MM', 'TL', 'CD', 'CG', 'FK', 'AQ', 'EU', 'EZ', 'UN', 'ZZ', 'QO', 'AA', '001', '002', '019', '150', ` +
		`'419', '999', 'us', 'gB', 'XA', 'XB', 'BU', 'DD', 'SU', 'YU']`
	dnScripts = `['Latn', 'Cyrl', 'Arab', 'Hans', 'Hant', 'Jpan', 'Kore', 'Hira', 'Kana', 'Hang', 'Deva', 'Beng', 'Thai', 'Grek', ` +
		`'Hebr', 'Ethi', 'Mong', 'Tibt', 'Zyyy', 'Zzzz', 'Zinh', 'Zmth', 'Zsye', 'Brai', 'Qaaa', 'Xxxx', 'latn', 'LATN', 'hAnS']`
	dnCurrencies = `['USD', 'EUR', 'JPY', 'CNY', 'GBP', 'CHF', 'CAD', 'AUD', 'HKD', 'TWD', 'KRW', 'INR', 'RUB', 'BRL', 'MXN', ` +
		`'XAU', 'XXX', 'XTS', 'BTC', 'ABC', 'DEM', 'FRF', 'ESP', 'VEF', 'VES', 'usd', 'eUr', 'ZWL', 'SLE', 'SLL', 'CNH']`
	dnCalendars = `['gregory', 'buddhist', 'chinese', 'coptic', 'dangi', 'ethioaa', 'ethiopic', 'hebrew', 'indian', 'islamic', ` +
		`'islamic-civil', 'islamic-umalqura', 'islamic-tbla', 'islamic-rgsa', 'iso8601', 'japanese', 'persian', 'roc', 'gregorian', ` +
		`'islamicc', 'ethiopic-amete-alem', 'unknown', 'GREGORY']`
	dnFields = `['era', 'year', 'quarter', 'month', 'weekOfYear', 'weekday', 'day', 'dayPeriod', 'hour', 'minute', 'second', 'timeZoneName']`
)

var dnLocales = []string{
	"en", "en-GB", "en-AU", "en-IN", "de", "de-CH", "fr", "fr-CA", "es", "es-MX", "it", "pt", "pt-PT", "nl", "sv",
	"da", "nb", "fi", "pl", "cs", "hu", "ro", "tr", "ru", "uk", "el", "he", "ar", "fa", "hi", "bn", "ta", "th",
	"vi", "id", "ms", "fil", "sw", "ja", "ko", "zh", "zh-TW", "zh-HK", "yue", "kk", "az", "sr", "sr-Latn", "xx",
}

func dnExprs() (r []string) {
	add := func(format string, args ...any) { r = append(r, fmt.Sprintf(format, args...)) }
	tags := append(chromiumTags(4), "und", "xx", "EN-us", "en-u-nu-arab", "en-u-ca-japanese", "x-private", "sr-ME",
		"iw", "in", "mo", "de-u-co-phonebk")
	for _, tag := range tags {
		add(`JSON.stringify(new Intl.DisplayNames(%q,{type:'language'}).resolvedOptions())`, tag)
	}
	for _, tag := range []string{"en", "zh", "xx"} {
		for _, o := range []string{`{type:'region'}`, `{type:'script'}`, `{type:'currency'}`, `{type:'calendar'}`, `{type:'dateTimeField'}`,
			`{type:'language',style:'short'}`, `{type:'region',style:'narrow'}`, `{type:'language',fallback:'none'}`,
			`{type:'language',languageDisplay:'standard'}`, `{type:'region',languageDisplay:'standard'}`, `{type:'script',fallback:'code'}`,
			`{type:'language',localeMatcher:'lookup'}`} {
			add(`JSON.stringify(new Intl.DisplayNames(%q,%s).resolvedOptions())`, tag, o)
		}
	}

	kinds := []struct{ typ, codes string }{
		{"language", dnLanguages}, {"region", dnRegions}, {"script", dnScripts}, {"currency", dnCurrencies},
		{"calendar", dnCalendars}, {"dateTimeField", dnFields},
	}
	for _, l := range dnLocales {
		for _, k := range kinds {
			for _, o := range []string{`style:'long'`, `style:'short'`, `style:'narrow'`, `fallback:'none'`} {
				add(`(() => {const d = new Intl.DisplayNames(%q,{type:%q,%s}); return %s.map((c) => {try {return String(d.of(c))} catch (e) {return e.constructor.name}}).join('|')})()`, l, k.typ, o, k.codes)
			}
		}
		add(`(() => {const d = new Intl.DisplayNames(%q,{type:'language',languageDisplay:'standard'}); return %s.map((c) => {try {return String(d.of(c))} catch (e) {return e.constructor.name}}).join('|')})()`, l, dnLanguages)
	}

	// Every language Chromium has names for.
	for _, l := range chromiumLanguages("DisplayNames") {
		add(`[new Intl.DisplayNames(%q,{type:'language'}).of('de-CH'), new Intl.DisplayNames(%q,{type:'region'}).of('JP'), new Intl.DisplayNames(%q,{type:'script'}).of('Cyrl'), new Intl.DisplayNames(%q,{type:'currency'}).of('EUR'), new Intl.DisplayNames(%q,{type:'calendar'}).of('gregory'), new Intl.DisplayNames(%q,{type:'dateTimeField'}).of('weekday')].join('|')`, l, l, l, l, l, l)
	}

	for _, e := range []string{
		`new Intl.DisplayNames('en', {type:'language'}).of({toString() { return 'fr'; }})`,
		`String(new Intl.DisplayNames('en', {type:'region', fallback:'none'}).of('ZZ'))`,
		`String(new Intl.DisplayNames('en', {type:'language', fallback:'none'}).of('xyz'))`,
		`new Intl.DisplayNames('en', {type:'language'}).of('en-Latn-US-u-ca-gregory')`,
		`JSON.stringify(Intl.DisplayNames.supportedLocalesOf(['en', 'de', 'xx', 'zh-Hant-TW', 'kk', 'az']))`,
		`JSON.stringify(Intl.DisplayNames.supportedLocalesOf(['en-US', 'ar-EG'], {localeMatcher:'lookup'}))`,
		`Object.prototype.toString.call(new Intl.DisplayNames('en', {type:'region'}))`,
		`Intl.DisplayNames.length`,
	} {
		add(`%s`, e)
	}
	add(`JSON.stringify(Intl.DisplayNames.supportedLocalesOf(%s))`, validTags(chromiumTags(3)))

	for _, src := range []string{
		`Intl.DisplayNames('en', {type:'region'})`, `new Intl.DisplayNames('en')`, `new Intl.DisplayNames('en', {})`,
		`new Intl.DisplayNames('en', undefined)`, `new Intl.DisplayNames('en', null)`, `new Intl.DisplayNames('en', 5)`,
		`new Intl.DisplayNames('en', 'region')`, `new Intl.DisplayNames('en-', {type:'region'})`, `new Intl.DisplayNames(null, {type:'region'})`,
		`new Intl.DisplayNames('en', {type:'x'})`, `new Intl.DisplayNames('en', {type:'region', style:'x'})`,
		`new Intl.DisplayNames('en', {type:'region', fallback:'x'})`, `new Intl.DisplayNames('en', {type:'language', languageDisplay:'x'})`,
		`new Intl.DisplayNames('en', {type:'region', languageDisplay:'x'})`, `new Intl.DisplayNames('en', {type:'region', localeMatcher:'x'})`,
		`new Intl.DisplayNames('en', {type:'unit'})`, `new Intl.DisplayNames('en', {type:'region'}).of()`,
		`new Intl.DisplayNames('en', {type:'region'}).of('')`, `new Intl.DisplayNames('en', {type:'region'}).of('USA')`,
		`new Intl.DisplayNames('en', {type:'region'}).of('1')`, `new Intl.DisplayNames('en', {type:'script'}).of('Lat')`,
		`new Intl.DisplayNames('en', {type:'currency'}).of('US')`, `new Intl.DisplayNames('en', {type:'currency'}).of('US1')`,
		`new Intl.DisplayNames('en', {type:'language'}).of('en-')`, `new Intl.DisplayNames('en', {type:'language'}).of('en-u-ca-gregory')`,
		`new Intl.DisplayNames('en', {type:'language'}).of('en-x-private')`, `new Intl.DisplayNames('en', {type:'language'}).of('root')`,
		`new Intl.DisplayNames('en', {type:'language'}).of('en_US')`, `new Intl.DisplayNames('en', {type:'calendar'}).of('ab')`,
		`new Intl.DisplayNames('en', {type:'calendar'}).of('gregory-')`, `new Intl.DisplayNames('en', {type:'dateTimeField'}).of('week')`,
		`new Intl.DisplayNames('en', {type:'dateTimeField'}).of('Era')`, `new Intl.DisplayNames('en', {type:'region'}).of(Symbol())`,
		`Intl.DisplayNames.prototype.of('US')`, `Intl.DisplayNames.prototype.resolvedOptions()`,
		`Intl.DisplayNames.prototype.of.call(new Intl.NumberFormat(), 'US')`, `Intl.DisplayNames.supportedLocalesOf('en', null)`,
	} {
		add(`(() => {try {%s; return 'ok'} catch (e) {return e.constructor.name}})()`, src)
	}
	return r
}

// A dnItem is one name of a DisplayNames expression: in the locale and
// with the options of its expression, the code asked for and the two
// answers, ours and Chromium's.
type dnItem struct {
	loc, typ, style, code string
	none                  bool // fallback:'none', where Chromium has no name is undefined
	got, want             string
}

var (
	dnGrid  = regexp.MustCompile(`^\(\(\) => \{const d = new Intl\.DisplayNames\("([^"]+)",\{type:["'](\w+)["'],(?:style:'(\w+)'|fallback:'none'|languageDisplay:'standard')\}\); return \[(.*?)\]\.map`)
	dnSweep = regexp.MustCompile(`^\[new Intl\.DisplayNames\("([^"]+)",\{type:'language'\}\)\.of\('de-CH'\)`)
)

// dnItems are the names of expr that differ, or false when expr is not
// a list of names or the two answers do not list as many.
func dnItems(expr, got, want string) ([]dnItem, bool) {
	var loc, style string
	var typs, codes []string
	none := strings.Contains(expr, "fallback:'none'")
	if m := dnGrid.FindStringSubmatch(expr); m != nil {
		loc, style = m[1], m[3]
		for _, c := range strings.Split(m[4], ", ") {
			codes = append(codes, strings.Trim(c, "'"))
			typs = append(typs, m[2])
		}
	} else if m := dnSweep.FindStringSubmatch(expr); m != nil {
		loc = m[1]
		typs = []string{"language", "region", "script", "currency", "calendar", "dateTimeField"}
		codes = []string{"de-CH", "JP", "Cyrl", "EUR", "gregory", "weekday"}
	} else {
		return nil, false
	}
	if style == "" {
		style = "long"
	}
	g, w := strings.Split(got, "|"), strings.Split(want, "|")
	if len(g) != len(codes) || len(w) != len(codes) {
		return nil, false
	}

	var r []dnItem
	for i := range codes {
		if g[i] != w[i] {
			r = append(r, dnItem{loc, typs[i], style, codes[i], none, g[i], w[i]})
		}
	}
	return r, true
}

// dnRootData are the languages Chromium has the locales of but no names
// data for: their names are root's, which are codes, ICU's own keys for
// calendars and English for fields, but for regions and currencies, which
// go to English, the default locale. The sweep over every language shows
// them.
var dnRootData = []string{"ak", "az", "bs", "kk", "kok", "pa", "sd", "uz"}

// dnRootFields are root's names of the date-time fields, and
// dnRootCalendars ICU's keys of the calendars whose key is not the BCP 47
// type.
var (
	dnRootFields = map[string]string{
		"era": "Era", "year": "Year", "quarter": "Quarter", "month": "Month", "weekOfYear": "Week", "weekday": "Day of the Week",
		"day": "Day", "dayPeriod": "Dayperiod", "hour": "Hour", "minute": "Minute", "second": "Second", "timeZoneName": "Zone",
	}
	dnRootCalendars = map[string]string{"gregory": "gregorian", "ethioaa": "ethiopic-amete-alem"}
)

// dnChromiumNames turns go-intl's names in loc into Chromium's, which
// has the short names of Hong Kong, Macao and Palestine as their names:
// "Hong Kong" for "Hong Kong SAR China".
func dnChromiumNames(loc string) *strings.Replacer {
	if r, ok := dnChromiumNamesIn.Load(loc); ok {
		return r.(*strings.Replacer)
	}

	r := dnChromiumNamesOf(loc)
	dnChromiumNamesIn.Store(loc, r)
	return r
}

var dnChromiumNamesIn sync.Map

func dnChromiumNamesOf(loc string) *strings.Replacer {
	var pairs []string
	l, err := goResolve(gointl.ServiceDisplayNames, []string{loc})
	if err != nil {
		return strings.NewReplacer()
	}

	long, err1 := gointl.NewDisplayNames(l, gointl.DisplayNamesOptions{Kind: gointl.DisplayRegion, Compat: goCompat})
	short, err2 := gointl.NewDisplayNames(l, gointl.DisplayNamesOptions{Kind: gointl.DisplayRegion, Style: gointl.DisplayShort, Compat: goCompat})
	if err1 != nil || err2 != nil {
		return strings.NewReplacer()
	}

	for _, r := range []string{"HK", "MO", "PS"} {
		n, _ := long.Of(r)
		s, _ := short.Of(r)
		if n != s {
			// Beside a language, ICU writes its brackets as square ones.
			square := strings.NewReplacer("(", "[", ")", "]")
			pairs = append(pairs, n, s, square.Replace(n), square.Replace(s))
		}
	}
	return strings.NewReplacer(pairs...)
}

// dnFalklands reports whether want is a name of the Falkland Islands,
// which Chromium has CLDR's variant name for, with the Spanish name in it,
// and go-intl does not have: "Falkland Islands (Islas Malvinas)", "îles
// Falkland (Malouines)", "جزر فوكلاند - جزر مالفيناس". Any name but the
// code is that one.
func dnFalklands(it dnItem, want string) bool {
	return it.typ == "region" && it.code == "FK" && want != "FK" && want != "undefined"
}

// dnSeparators are what locales put between the parts beside a name.
var dnSeparators = regexp.MustCompile(`, |، |、|，`)

// dnBeside finds the parts beside a name, in brackets at its end, which
// some locales write with no space before them and some as full width.
var dnBeside = regexp.MustCompile(`^(.*?) ?[(（]([^()（）]*)[)）]$`)

// dnParts splits a name into the name and what is put beside it:
// "Catalan (Spain, Valencian)" is "Catalan", then "Spain" and "Valencian".
// The name may have its own: "anglais (É.-U.) (informatique)".
func dnParts(name string) []string {
	m := dnBeside.FindStringSubmatch(name)
	if m == nil {
		return []string{name}
	}

	return append([]string{m[1]}, dnSeparators.Split(m[2], -1)...)
}

// dnCodes are the codes Chromium may write in place of a name of code:
// code and its canonical form, whole and by subtags.
func dnCodes(code string) map[string]bool {
	r := map[string]bool{}
	add := func(s string) {
		r[strings.ToLower(s)] = true
		for _, sub := range strings.Split(s, "-") {
			r[strings.ToLower(sub)] = true
		}
	}
	add(code)
	if c, err := goCanonicalizer(); err == nil {
		if l, err := c.Canonicalize(code); err == nil {
			add(l.String())
		}
	}
	return r
}

// dnExplained reports whether Chromium's name differs from ours only as its
// ICU data does: a name, or a part of one, it has no name for is the code,
// a language with no names data has root's and English ones, and a few
// English names are Chromium's own. english are Chromium's English names
// for the same expression and of regions in the same style.
func dnExplained(it dnItem, english map[string]bool) bool {
	got := dnChromiumNames(it.loc).Replace(it.got)
	if got == it.want || dnFalklands(it, it.want) {
		return true
	}

	if it.none && it.want == "undefined" {
		// No name, where root's would be the code.
		return true
	}

	root := slices.Contains(dnRootData, strings.Split(it.loc, "-")[0])
	if root {
		switch {
		case english[it.want]:
			return true
		case it.typ == "dateTimeField":
			return it.want == dnRootFields[it.code]
		case it.typ == "calendar":
			return it.want == it.code || it.want == dnRootCalendars[it.code]
		}
	}
	codes := dnCodes(it.code)
	if codes[strings.ToLower(it.want)] {
		return true
	}

	g, w := dnParts(got), dnParts(it.want)
	if root {
		// Root's code for the language, or the locale's name of its own
		// language, the one name Chromium keeps, with codes and English
		// names beside it.
		if !codes[strings.ToLower(w[0])] && w[0] != g[0] && w[0] != dnOwnName(it) {
			return false
		}

		region := dnEnglishRegion(it)
		for _, part := range w[1:] {
			if !codes[strings.ToLower(part)] && !english[part] && part != region {
				return false
			}
		}
		return true
	}
	if len(g) != len(w) {
		return false
	}

	for i := range w {
		if g[i] != w[i] && !codes[strings.ToLower(w[i])] {
			return false
		}
	}
	return true
}

// dnOwnName is go-intl's name of the language of it's locale in it, when
// it's code is of that language.
func dnOwnName(it dnItem) string {
	lang := strings.Split(it.loc, "-")[0]
	if strings.Split(it.code, "-")[0] != lang {
		return ""
	}

	l, err := gointl.ParseLocale(lang)
	if err != nil {
		return ""
	}

	d, err := gointl.NewDisplayNames(l, gointl.DisplayNamesOptions{Kind: gointl.DisplayLanguage, Compat: goCompat})
	if err != nil {
		return ""
	}

	name, _ := d.Of(lang)
	return name
}

// dnEnglishRegions are the expressions of English names of regions, by
// style.
var dnEnglishRegions = sync.OnceValue(func() map[string][]string {
	r := map[string][]string{}
	for _, e := range dnExprs() {
		if m := dnGrid.FindStringSubmatch(e); m != nil && m[1] == "en" && m[2] == "region" {
			style := m[3]
			if style == "" {
				style = "long"
			}
			r[style] = append(r[style], e)
		}
	}
	return r
})

// dnEnglishRegion is the English name of the region of it's code, in its
// style, as Chromium has it.
func dnEnglishRegion(it dnItem) string {
	c, err := goCanonicalizer()
	if err != nil {
		return ""
	}

	l, err := c.Canonicalize(it.code)
	if err != nil || l.Region.String() == "" {
		return ""
	}

	opts := gointl.DisplayNamesOptions{Kind: gointl.DisplayRegion, Compat: goCompat}
	if it.style != "long" {
		opts.Style = gointl.DisplayShort
	}
	d, err := gointl.NewDisplayNames(goDefaultLocale, opts)
	if err != nil {
		return ""
	}

	name, _ := d.Of(l.Region.String())
	return dnChromiumNames("en").Replace(name)
}

// dnEnglish are Chromium's English names for expr's codes, and of regions
// in the style of expr, from the corpus.
func dnEnglish(expr string, recorded map[string]string) map[string]bool {
	r := map[string]bool{}
	m := dnGrid.FindStringSubmatch(expr)
	var en []string
	if m == nil {
		if m = dnSweep.FindStringSubmatch(expr); m == nil {
			return r
		}
		en = append(en, strings.ReplaceAll(expr, fmt.Sprintf("%q", m[1]), `"en"`))
	} else {
		en = append(en, strings.Replace(expr, fmt.Sprintf("%q", m[1]), `"en"`, 1))
		style := m[3]
		if style == "" {
			style = "long"
		}
		en = append(en, dnEnglishRegions()[style]...)
	}
	for _, e := range en {
		for _, name := range strings.Split(recorded[e], "|") {
			r[name] = true
		}
	}
	return r
}

// dnGoIntlFields are the names of fields go-intl has otherwise than ICU,
// which Node 77 writes as Chromium does: by locale, style and field.
var dnGoIntlFields = map[[3]string]bool{{"de-CH", "narrow", "era"}: true, {"th", "narrow", "timeZoneName"}: true}
