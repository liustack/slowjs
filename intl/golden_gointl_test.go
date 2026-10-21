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
	"sync"
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

var chromiumTagList = sync.OnceValue(func() []string {
	b, err := os.ReadFile(filepath.Join("..", "internal", "gointl", "qjsdata", "chromium.json"))
	if err != nil {
		panic(err)
	}

	var v struct{ Tags []string }
	if err := json.Unmarshal(b, &v); err != nil {
		panic(err)
	}

	return v.Tags
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
