// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// startTag and endTag (JS_NewStartTagFunction, JS_NewEndTagFunction) against
// the functions of React DOM 19.3 they stand in for: pushStartGenericElement,
// with what it calls, and endChunkForTag. The functions between the two
// "React DOM" lines are those of react-dom-server-legacy.browser.production.js
// of react-dom 19.3.1, as they are (MIT, Copyright (c) Meta Platforms, Inc. and
// affiliates). For every call, listed or random, startTag either pushes onto
// target one string that is the text React's function pushes, and returns
// what it returns, or returns decline and leaves target as it was. It never
// takes a call where React throws, and it throws nothing here.
'use strict'
// ---- React DOM 19.3.1 ----
function formatProdErrorMessage(code) {
  var url = "https://react.dev/errors/" + code;
  if (1 < arguments.length) {
    url += "?args[]=" + encodeURIComponent(arguments[1]);
    for (var i = 2; i < arguments.length; i++)
      url += "&args[]=" + encodeURIComponent(arguments[i]);
  }
  return (
    "Minified React error #" +
    code +
    "; visit " +
    url +
    " for the full message or use the non-minified dev environment for full errors and additional helpful warnings."
  );
}
var assign = Object.assign,
  hasOwnProperty = Object.prototype.hasOwnProperty,
  VALID_ATTRIBUTE_NAME_REGEX = RegExp(
    "^[:A-Z_a-z\\u00C0-\\u00D6\\u00D8-\\u00F6\\u00F8-\\u02FF\\u0370-\\u037D\\u037F-\\u1FFF\\u200C-\\u200D\\u2070-\\u218F\\u2C00-\\u2FEF\\u3001-\\uD7FF\\uF900-\\uFDCF\\uFDF0-\\uFFFD][:A-Z_a-z\\u00C0-\\u00D6\\u00D8-\\u00F6\\u00F8-\\u02FF\\u0370-\\u037D\\u037F-\\u1FFF\\u200C-\\u200D\\u2070-\\u218F\\u2C00-\\u2FEF\\u3001-\\uD7FF\\uF900-\\uFDCF\\uFDF0-\\uFFFD\\-.0-9\\u00B7\\u0300-\\u036F\\u203F-\\u2040]*$"
  ),
  illegalAttributeNameCache = {},
  validatedAttributeNameCache = {};
function isAttributeNameSafe(attributeName) {
  if (hasOwnProperty.call(validatedAttributeNameCache, attributeName))
    return !0;
  if (hasOwnProperty.call(illegalAttributeNameCache, attributeName)) return !1;
  if (VALID_ATTRIBUTE_NAME_REGEX.test(attributeName))
    return (validatedAttributeNameCache[attributeName] = !0);
  illegalAttributeNameCache[attributeName] = !0;
  return !1;
}
var unitlessNumbers = new Set(
    "animationIterationCount aspectRatio borderImageOutset borderImageSlice borderImageWidth boxFlex boxFlexGroup boxOrdinalGroup columnCount columns flex flexGrow flexPositive flexShrink flexNegative flexOrder gridArea gridRow gridRowEnd gridRowSpan gridRowStart gridColumn gridColumnEnd gridColumnSpan gridColumnStart fontWeight lineClamp lineHeight opacity order orphans scale tabSize widows zIndex zoom fillOpacity floodOpacity stopOpacity strokeDasharray strokeDashoffset strokeMiterlimit strokeOpacity strokeWidth MozAnimationIterationCount MozBoxFlex MozBoxFlexGroup MozLineClamp msAnimationIterationCount msFlex msZoom msFlexGrow msFlexNegative msFlexOrder msFlexPositive msFlexShrink msGridColumn msGridColumnSpan msGridRow msGridRowSpan WebkitAnimationIterationCount WebkitBoxFlex WebKitBoxFlexGroup WebkitBoxOrdinalGroup WebkitColumnCount WebkitColumns WebkitFlex WebkitFlexGrow WebkitFlexPositive WebkitFlexShrink WebkitLineClamp".split(
      " "
    )
  ),
  aliases = new Map([
    ["acceptCharset", "accept-charset"],
    ["htmlFor", "for"],
    ["httpEquiv", "http-equiv"],
    ["crossOrigin", "crossorigin"],
    ["accentHeight", "accent-height"],
    ["alignmentBaseline", "alignment-baseline"],
    ["arabicForm", "arabic-form"],
    ["baselineShift", "baseline-shift"],
    ["capHeight", "cap-height"],
    ["clipPath", "clip-path"],
    ["clipRule", "clip-rule"],
    ["colorInterpolation", "color-interpolation"],
    ["colorInterpolationFilters", "color-interpolation-filters"],
    ["colorProfile", "color-profile"],
    ["colorRendering", "color-rendering"],
    ["dominantBaseline", "dominant-baseline"],
    ["enableBackground", "enable-background"],
    ["fillOpacity", "fill-opacity"],
    ["fillRule", "fill-rule"],
    ["floodColor", "flood-color"],
    ["floodOpacity", "flood-opacity"],
    ["fontFamily", "font-family"],
    ["fontSize", "font-size"],
    ["fontSizeAdjust", "font-size-adjust"],
    ["fontStretch", "font-stretch"],
    ["fontStyle", "font-style"],
    ["fontVariant", "font-variant"],
    ["fontWeight", "font-weight"],
    ["glyphName", "glyph-name"],
    ["glyphOrientationHorizontal", "glyph-orientation-horizontal"],
    ["glyphOrientationVertical", "glyph-orientation-vertical"],
    ["horizAdvX", "horiz-adv-x"],
    ["horizOriginX", "horiz-origin-x"],
    ["imageRendering", "image-rendering"],
    ["letterSpacing", "letter-spacing"],
    ["lightingColor", "lighting-color"],
    ["markerEnd", "marker-end"],
    ["markerMid", "marker-mid"],
    ["markerStart", "marker-start"],
    ["maskType", "mask-type"],
    ["overlinePosition", "overline-position"],
    ["overlineThickness", "overline-thickness"],
    ["paintOrder", "paint-order"],
    ["panose-1", "panose-1"],
    ["pointerEvents", "pointer-events"],
    ["renderingIntent", "rendering-intent"],
    ["shapeRendering", "shape-rendering"],
    ["stopColor", "stop-color"],
    ["stopOpacity", "stop-opacity"],
    ["strikethroughPosition", "strikethrough-position"],
    ["strikethroughThickness", "strikethrough-thickness"],
    ["strokeDasharray", "stroke-dasharray"],
    ["strokeDashoffset", "stroke-dashoffset"],
    ["strokeLinecap", "stroke-linecap"],
    ["strokeLinejoin", "stroke-linejoin"],
    ["strokeMiterlimit", "stroke-miterlimit"],
    ["strokeOpacity", "stroke-opacity"],
    ["strokeWidth", "stroke-width"],
    ["textAnchor", "text-anchor"],
    ["textDecoration", "text-decoration"],
    ["textRendering", "text-rendering"],
    ["transformOrigin", "transform-origin"],
    ["underlinePosition", "underline-position"],
    ["underlineThickness", "underline-thickness"],
    ["unicodeBidi", "unicode-bidi"],
    ["unicodeRange", "unicode-range"],
    ["unitsPerEm", "units-per-em"],
    ["vAlphabetic", "v-alphabetic"],
    ["vHanging", "v-hanging"],
    ["vIdeographic", "v-ideographic"],
    ["vMathematical", "v-mathematical"],
    ["vectorEffect", "vector-effect"],
    ["vertAdvY", "vert-adv-y"],
    ["vertOriginX", "vert-origin-x"],
    ["vertOriginY", "vert-origin-y"],
    ["wordSpacing", "word-spacing"],
    ["writingMode", "writing-mode"],
    ["xmlnsXlink", "xmlns:xlink"],
    ["xHeight", "x-height"]
  ]),
  matchHtmlRegExp = /["'&<>]/;
function escapeTextForBrowser(text) {
  if (
    "boolean" === typeof text ||
    "number" === typeof text ||
    "bigint" === typeof text
  )
    return "" + text;
  text = "" + text;
  var match = matchHtmlRegExp.exec(text);
  if (match) {
    var html = "",
      index,
      lastIndex = 0;
    for (index = match.index; index < text.length; index++) {
      switch (text.charCodeAt(index)) {
        case 34:
          match = "&quot;";
          break;
        case 38:
          match = "&amp;";
          break;
        case 39:
          match = "&#x27;";
          break;
        case 60:
          match = "&lt;";
          break;
        case 62:
          match = "&gt;";
          break;
        default:
          continue;
      }
      lastIndex !== index && (html += text.slice(lastIndex, index));
      lastIndex = index + 1;
      html += match;
    }
    text = lastIndex !== index ? html + text.slice(lastIndex, index) : html;
  }
  return text;
}
var uppercasePattern = /([A-Z])/g,
  msPattern = /^ms-/,
  isJavaScriptProtocol =
    /^[\u0000-\u001F ]*j[\r\n\t]*a[\r\n\t]*v[\r\n\t]*a[\r\n\t]*s[\r\n\t]*c[\r\n\t]*r[\r\n\t]*i[\r\n\t]*p[\r\n\t]*t[\r\n\t]*:/i;
function sanitizeURL(url) {
  return isJavaScriptProtocol.test("" + url)
    ? "javascript:throw new Error('React has blocked a javascript: URL as a security precaution.')"
    : url;
}
function pushViewTransitionAttributes(target, formatContext) {
  formatContext = formatContext.viewTransition;
  null !== formatContext &&
    ("auto" !== formatContext.name &&
      (pushStringAttribute(
        target,
        "vt-name",
        0 === formatContext.nameIdx
          ? formatContext.name
          : formatContext.name + "_" + formatContext.nameIdx
      ),
      formatContext.nameIdx++),
    pushStringAttribute(target, "vt-update", formatContext.update),
    "none" !== formatContext.enter &&
      pushStringAttribute(target, "vt-enter", formatContext.enter),
    "none" !== formatContext.exit &&
      pushStringAttribute(target, "vt-exit", formatContext.exit),
    "none" !== formatContext.share &&
      pushStringAttribute(target, "vt-share", formatContext.share));
}
var styleNameCache = new Map();
function pushStyleAttribute(target, style) {
  if ("object" !== typeof style) throw Error(formatProdErrorMessage(62));
  var isFirst = !0,
    styleName;
  for (styleName in style)
    if (hasOwnProperty.call(style, styleName)) {
      var styleValue = style[styleName];
      if (
        null != styleValue &&
        "boolean" !== typeof styleValue &&
        "" !== styleValue
      ) {
        if (0 === styleName.indexOf("--")) {
          var nameChunk = escapeTextForBrowser(styleName);
          styleValue = escapeTextForBrowser(("" + styleValue).trim());
        } else
          (nameChunk = styleNameCache.get(styleName)),
            void 0 === nameChunk &&
              ((nameChunk = escapeTextForBrowser(
                styleName
                  .replace(uppercasePattern, "-$1")
                  .toLowerCase()
                  .replace(msPattern, "-ms-")
              )),
              styleNameCache.set(styleName, nameChunk)),
            (styleValue =
              "number" === typeof styleValue
                ? 0 === styleValue || unitlessNumbers.has(styleName)
                  ? "" + styleValue
                  : styleValue + "px"
                : escapeTextForBrowser(("" + styleValue).trim()));
        isFirst
          ? ((isFirst = !1),
            target.push(' style="', nameChunk, ":", styleValue))
          : target.push(";", nameChunk, ":", styleValue);
      }
    }
  isFirst || target.push('"');
}
function pushBooleanAttribute(target, name, value) {
  value &&
    "function" !== typeof value &&
    "symbol" !== typeof value &&
    target.push(" ", name, '=""');
}
function pushStringAttribute(target, name, value) {
  "function" !== typeof value &&
    "symbol" !== typeof value &&
    "boolean" !== typeof value &&
    target.push(" ", name, '="', escapeTextForBrowser(value), '"');
}
function pushAttribute(target, name, value) {
  switch (name) {
    case "className":
      pushStringAttribute(target, "class", value);
      break;
    case "tabIndex":
      pushStringAttribute(target, "tabindex", value);
      break;
    case "dir":
    case "role":
    case "viewBox":
    case "width":
    case "height":
      pushStringAttribute(target, name, value);
      break;
    case "style":
      pushStyleAttribute(target, value);
      break;
    case "src":
    case "href":
      if ("" === value) break;
    case "action":
    case "formAction":
      if (
        null == value ||
        "function" === typeof value ||
        "symbol" === typeof value ||
        "boolean" === typeof value
      )
        break;
      value = sanitizeURL("" + value);
      target.push(" ", name, '="', escapeTextForBrowser(value), '"');
      break;
    case "defaultValue":
    case "defaultChecked":
    case "innerHTML":
    case "suppressContentEditableWarning":
    case "suppressHydrationWarning":
    case "ref":
      break;
    case "autoFocus":
    case "multiple":
    case "muted":
      pushBooleanAttribute(target, name.toLowerCase(), value);
      break;
    case "xlinkHref":
      if (
        "function" === typeof value ||
        "symbol" === typeof value ||
        "boolean" === typeof value
      )
        break;
      value = sanitizeURL("" + value);
      target.push(" ", "xlink:href", '="', escapeTextForBrowser(value), '"');
      break;
    case "contentEditable":
    case "spellCheck":
    case "draggable":
    case "value":
    case "autoReverse":
    case "externalResourcesRequired":
    case "focusable":
    case "preserveAlpha":
      "function" !== typeof value &&
        "symbol" !== typeof value &&
        target.push(" ", name, '="', escapeTextForBrowser(value), '"');
      break;
    case "inert":
    case "allowFullScreen":
    case "async":
    case "autoPlay":
    case "controls":
    case "credentialless":
    case "default":
    case "defer":
    case "disabled":
    case "disablePictureInPicture":
    case "disableRemotePlayback":
    case "formNoValidate":
    case "hidden":
    case "loop":
    case "noModule":
    case "noValidate":
    case "open":
    case "playsInline":
    case "readOnly":
    case "required":
    case "reversed":
    case "scoped":
    case "seamless":
    case "itemScope":
      value &&
        "function" !== typeof value &&
        "symbol" !== typeof value &&
        target.push(" ", name, '=""');
      break;
    case "capture":
    case "download":
      !0 === value
        ? target.push(" ", name, '=""')
        : !1 !== value &&
          "function" !== typeof value &&
          "symbol" !== typeof value &&
          target.push(" ", name, '="', escapeTextForBrowser(value), '"');
      break;
    case "cols":
    case "rows":
    case "size":
    case "span":
      "function" !== typeof value &&
        "symbol" !== typeof value &&
        !isNaN(value) &&
        1 <= value &&
        target.push(" ", name, '="', escapeTextForBrowser(value), '"');
      break;
    case "rowSpan":
    case "start":
      "function" === typeof value ||
        "symbol" === typeof value ||
        isNaN(value) ||
        target.push(" ", name, '="', escapeTextForBrowser(value), '"');
      break;
    case "xlinkActuate":
      pushStringAttribute(target, "xlink:actuate", value);
      break;
    case "xlinkArcrole":
      pushStringAttribute(target, "xlink:arcrole", value);
      break;
    case "xlinkRole":
      pushStringAttribute(target, "xlink:role", value);
      break;
    case "xlinkShow":
      pushStringAttribute(target, "xlink:show", value);
      break;
    case "xlinkTitle":
      pushStringAttribute(target, "xlink:title", value);
      break;
    case "xlinkType":
      pushStringAttribute(target, "xlink:type", value);
      break;
    case "xmlBase":
      pushStringAttribute(target, "xml:base", value);
      break;
    case "xmlLang":
      pushStringAttribute(target, "xml:lang", value);
      break;
    case "xmlSpace":
      pushStringAttribute(target, "xml:space", value);
      break;
    default:
      if (
        !(2 < name.length) ||
        ("o" !== name[0] && "O" !== name[0]) ||
        ("n" !== name[1] && "N" !== name[1])
      )
        if (((name = aliases.get(name) || name), isAttributeNameSafe(name))) {
          switch (typeof value) {
            case "function":
            case "symbol":
              return;
            case "boolean":
              var prefix$8 = name.toLowerCase().slice(0, 5);
              if ("data-" !== prefix$8 && "aria-" !== prefix$8) return;
          }
          target.push(" ", name, '="', escapeTextForBrowser(value), '"');
        }
  }
}
function pushInnerHTML(target, innerHTML, children) {
  if (null != innerHTML) {
    if (null != children) throw Error(formatProdErrorMessage(60));
    if ("object" !== typeof innerHTML || !("__html" in innerHTML))
      throw Error(formatProdErrorMessage(61));
    innerHTML = innerHTML.__html;
    null !== innerHTML && void 0 !== innerHTML && target.push("" + innerHTML);
  }
}
function pushStartGenericElement(target, props, tag, formatContext) {
  target.push(startChunkForTag(tag));
  var innerHTML = (tag = null),
    propKey;
  for (propKey in props)
    if (hasOwnProperty.call(props, propKey)) {
      var propValue = props[propKey];
      if (null != propValue)
        switch (propKey) {
          case "children":
            tag = propValue;
            break;
          case "dangerouslySetInnerHTML":
            innerHTML = propValue;
            break;
          default:
            pushAttribute(target, propKey, propValue);
        }
    }
  pushViewTransitionAttributes(target, formatContext);
  target.push(">");
  pushInnerHTML(target, innerHTML, tag);
  return "string" === typeof tag
    ? (target.push(escapeTextForBrowser(tag)), null)
    : tag;
}
var VALID_TAG_REGEX = /^[a-zA-Z][a-zA-Z:_\.\-\d]*$/,
  validatedTagCache = new Map();
function startChunkForTag(tag) {
  var tagStartChunk = validatedTagCache.get(tag);
  if (void 0 === tagStartChunk) {
    if (!VALID_TAG_REGEX.test(tag))
      throw Error(formatProdErrorMessage(65, tag));
    tagStartChunk = "<" + tag;
    validatedTagCache.set(tag, tagStartChunk);
  }
  return tagStartChunk;
}
var endTagCache = new Map();
function endChunkForTag(tag) {
  var chunk = endTagCache.get(tag);
  void 0 === chunk && ((chunk = "</" + tag + ">"), endTagCache.set(tag, chunk));
  return chunk;
}
// ---- end of React DOM ----

const D = {}
const FC = { insertionMode: 2, selectedValue: null, tagScope: 0, viewTransition: null }
const out = []
let bad = 0, count = 0, taken = 0
function fail(s) {
  bad++
  if (bad <= 20) out.push('FAIL ' + s)
}
function show(v) {
  try { return typeof v === 'string' ? JSON.stringify(v) : String(v) } catch (e) { return typeof v }
}

// pushStartGenericElement and startTag on the same arguments. want is 'take',
// 'decline' or undefined (either is right).
function check(name, tag, props, want, fc, hop, target) {
  fc = fc || FC
  hop = hop || hasOwnProperty
  count++
  const o = []
  let ro, oerr = null
  try { ro = pushStartGenericElement(o, props, tag, fc) } catch (e) { oerr = e }
  const c = target || ['before']
  const before = c.slice()
  let rc
  try {
    rc = startTag(c, props, tag, fc, hop, D)
  } catch (e) {
    fail(name + ': threw ' + e)
    return
  }
  if (rc === D) {
    if (c.length !== before.length || c.some((v, i) => v !== before[i]))
      fail(name + ': declined and changed target')
    if (want === 'take')
      fail(name + ': declined')
    return
  }
  taken++
  if (want === 'decline')
    fail(name + ': taken')
  if (oerr) {
    fail(name + ': taken where React throws ' + oerr)
    return
  }
  const html = o.join('')
  if (c.length !== before.length + 1 || before.some((v, i) => v !== c[i]))
    fail(name + ': pushed ' + (c.length - before.length) + ' strings')
  else if (c[before.length] !== html)
    fail(name + ': wrote ' + show(c[before.length]) + ', React ' + show(html))
  if (rc !== ro && !(rc !== rc && ro !== ro))
    fail(name + ': returned ' + show(rc) + ', React ' + show(ro))
}

const cls60 = 'flex items-center justify-between gap-2 rounded-md px-3 py-2'
const cls200 = 'group relative flex w-full min-w-0 items-center justify-between gap-3 rounded-lg border border-zinc-200 bg-white px-4 py-3 text-left text-sm font-medium text-zinc-900 shadow-sm transition-colors hover:bg-zinc-50 focus-visible:outline-none'
function fn() {}
const sym = Symbol('s')
const kids = ['a', 'b']

// what pages have, and each branch of pushAttribute and pushStyleAttribute
const cases = [
  ['div', { className: cls60, children: 'Order SH-1042' }, 'take'],
  ['div', { className: cls60, id: 'row-12', role: 'button', tabIndex: 0, 'aria-label': 'Open order', 'data-state': 'closed', children: 'x' }, 'take'],
  ['div', { style: { display: 'flex', marginTop: 4, width: '50%', backgroundColor: '#fafafa' }, children: 'x' }, 'take'],
  ['button', { className: cls60, onClick: fn, children: [] }, 'take'],
  ['span', { className: cls200, children: 'Long class' }, 'take'],
  ['div', {}, 'take'],
  ['div', { children: null }, 'take'],
  ['div', { children: undefined, className: undefined }, 'take'],
  ['div', { children: 42 }, 'take'],
  ['div', { children: kids }, 'take'],
  ['div', { children: '<b>&\'"x"</b>' }, 'take'],
  ['div', { children: '中文 <i>\ud800' }, 'take'],
  ['div', { children: 'a'.repeat(30) + 'b'.repeat(40) }, 'take'],
  ['div', { children: '&'.repeat(9000) + 'x'.repeat(9000) + '中' }, 'take'],
  ['div', { className: 'a<b>&"\'' }, 'take'],
  ['div', { className: 12 }, 'take'],
  ['div', { className: 1.5 }, 'take'],
  ['div', { className: -0 }, 'take'],
  ['div', { className: NaN }, 'take'],
  ['div', { className: -7 }, 'take'],
  ['div', { className: 1e21 }, 'take'],
  ['div', { className: -2147483648 }, 'take'],
  ['div', { className: true }, 'take'],
  ['div', { className: false }, 'take'],
  ['div', { className: fn }, 'take'],
  ['div', { className: sym }, 'take'],
  ['div', { className: '' }, 'take'],
  ['div', { className: '中文类' }, 'take'],
  ['div', { tabIndex: -1, dir: 'rtl', role: 'tab', viewBox: '0 0 24 24', width: 10, height: '20' }, 'take'],
  ['svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', xmlns: 'http://www.w3.org/2000/svg' }, 'take'],
  ['path', { d: 'M5 12h14', fillRule: 'evenodd', clipRule: 'evenodd', fillOpacity: 0.5 }, 'take'],
  ['div', { htmlFor: 'x', acceptCharset: 'utf-8', httpEquiv: 'refresh', crossOrigin: 'anonymous', xmlnsXlink: 'http://www.w3.org/1999/xlink', 'panose-1': '2' }, 'take'],
  ['div', { 'data-x': true, 'data-y': false, 'aria-hidden': true, 'aria-busy': false, 'DATA-z': true, 'Aria-Q': true, other: true, flag: false }, 'take'],
  ['div', { 'data-n': 0, 'data-f': 0.25, 'aria-level': 3, 'data-': true, 'aria': true }, 'take'],
  ['div', { onClick: fn, onclick: 'x', ON: 'y', on: 'z', onX: 1, One: 'o', Onx: 2, oN: 3, o: 4 }, 'take'],
  ['div', { '': 'e', '1a': 'x', 'a b': 'x', 'a"b': 'x', ':ok': 'y', _ok: 'y', 'a-b.c:d_9': 'z', '-x': 'x', '.x': 'x' }, 'take'],
  ['div', { 'children\u0000x': 'hello' }, 'take'],
  ['div', { 'id\u0000x': 'a', 'style\u0000': 'b', 'on\u0000': 'c', '\u0000': 'd', 'dangerouslySetInnerHTML\u0000': 'e', 'x\u0000': 'f', 'children\u0000': 'g' }, 'take'],
  ['a', { href: '/x?a=1&b=2', src: '', action: '/post', formAction: '/f' }, 'take'],
  ['div', { href: '', src: '/img.png' }, 'take'],
  ['div', { href: 12, src: 0, action: 1.5 }, 'take'],
  ['div', { href: true, src: fn, action: false, formAction: sym }, 'take'],
  ['div', { href: 'javascript:alert(1)' }, 'decline'],
  ['div', { href: ' \tJaVa\nScRiPt:x' }, 'decline'],
  ['div', { action: '\u0001javascript:x' }, 'decline'],
  ['div', { formAction: 'JAVASCRIPT:' }, 'decline'],
  ['div', { href: 'java script:x' }, 'take'],
  ['div', { href: 'javascript' }, 'take'],
  ['div', { href: '\u007fjavascript:x' }, 'take'],
  ['div', { href: 'jſvascript:x' }, 'take'],
  ['div', { href: '　javascript:x' }, 'take'],
  ['div', { defaultValue: 'x', defaultChecked: true, innerHTML: '<b>', suppressContentEditableWarning: true, suppressHydrationWarning: true, ref: {} }, 'take'],
  ['div', { autoFocus: true, multiple: 1, muted: '' }, 'take'],
  ['div', { autoFocus: false, multiple: 0, muted: 'yes' }, 'take'],
  ['div', { autoFocus: fn, multiple: sym, muted: {} }, 'take'],
  ['div', { disabled: 0n, hidden: 1n, open: NaN, inert: -0 }, 'take'],
  ['div', { contentEditable: true, spellCheck: false, draggable: 'true', value: 5, autoReverse: 'x', externalResourcesRequired: false, focusable: 'false', preserveAlpha: true }, 'take'],
  ['div', { value: fn, draggable: sym }, 'take'],
  ['div', { inert: true, allowFullScreen: 1, async: 'a', autoPlay: false, controls: 0, credentialless: '', default: NaN, defer: [], disabled: true, hidden: 'until-found', loop: true, noModule: true, noValidate: true, open: true, playsInline: true, readOnly: true, required: true, reversed: true, scoped: true, seamless: true, itemScope: true, disablePictureInPicture: true, disableRemotePlayback: true, formNoValidate: true }, 'take'],
  ['div', { xlinkActuate: 'a', xlinkArcrole: 'b', xlinkRole: 'c', xlinkShow: 'd', xlinkTitle: 'e', xlinkType: 'f', xmlBase: 'g', xmlLang: 'h', xmlSpace: 'i' }, 'take'],
  ['div', { xlinkActuate: true, xmlLang: fn }, 'take'],
  ['div', { style: {} }, 'take'],
  ['div', { style: { color: null, margin: undefined, padding: true, border: false, top: '' } }, 'take'],
  ['div', { style: { marginTop: 0, marginLeft: -0, zIndex: 3, opacity: 0.5, flex: 1, lineHeight: 1.5, width: 12.5, height: -3, top: NaN, left: Infinity, right: -1e-7 } }, 'take'],
  ['div', { style: { WebkitTransform: 'x', msTransform: 'y', MozBoxFlex: 2, msFlex: 1, MsFoo: 3, ms: 4, mS: 5, msx: 6, 'ms-x': 7, ms1: 8 } }, 'take'],
  ['div', { style: { '--brand': ' #f00 ', '--n': 4, '--z': 0, '--f': -0.5, '--x<': 'a&b', '--中': 'v', '-x': 1, '--': 2 } }, 'take'],
  ['div', { style: { color: '  red  ', font: '  12px 　', content: '\'"<&>\'', backgroundImage: 'url("a.png")', margin: ' \n\t', padding: '﻿x ' } }, 'take'],
  ['div', { style: { color: 'red' }, className: 'c', id: 'i' }, 'take'],
  ['div', { className: 'x', style: { color: 'red' }, title: 't' }, 'take'],
  ['div', { style: { 'a<b': 1, 'X&Y': '2', '': 3 } }, 'take'],
  ['div', { style: { 颜色: 'red' } }, 'decline'],
  ['div', { style: { color: {} } }, 'decline'],
  ['div', { style: { color: fn } }, 'decline'],
  ['div', { style: { color: sym } }, 'decline'],
  ['div', { style: { color: 1n } }, 'decline'],
  ['div', { style: 'color:red' }, 'decline'],
  ['div', { style: fn }, 'decline'],
  ['div', { style: [1, 2] }, 'decline'],
  ['div', { style: Object.create(null) }, 'decline'],
  ['div', { style: { get color() { return 'red' } } }, 'decline'],
  ['div', { style: { 0: 'x' } }, 'decline'],
  ['div', { style: Object.fromEntries(Array.from({ length: 33 }, (_, i) => ['k' + i, i])) }, 'decline'],
  ['div', { style: Object.fromEntries(Array.from({ length: 32 }, (_, i) => ['k' + i, i])) }, 'take'],
  ['div', { style: Object.defineProperty({ color: 'red' }, 'margin', { value: 1, enumerable: false }) }, 'take'],
  ['div', { style: { [sym]: 1, color: 'red' } }, 'take'],
  ['div', { title: {} }, 'decline'],
  ['div', { title: 1n }, 'decline'],
  ['div', { title: [1, 2] }, 'decline'],
  ['div', { title: new String('s') }, 'decline'],
  ['div', { dangerouslySetInnerHTML: { __html: '<b>x</b>' } }, 'decline'],
  ['div', { dangerouslySetInnerHTML: null, children: 'x' }, 'take'],
  ['div', { dangerouslySetInnerHTML: { __html: 'x' }, children: 'x' }, 'decline'],
  ['div', { xlinkHref: '#a' }, 'decline'],
  ['div', { capture: true }, 'decline'],
  ['div', { download: 'f' }, 'decline'],
  ['div', { cols: 3 }, 'decline'],
  ['div', { rows: 2 }, 'decline'],
  ['div', { size: 1 }, 'decline'],
  ['div', { span: 2 }, 'decline'],
  ['div', { rowSpan: 2 }, 'decline'],
  ['div', { start: 1 }, 'decline'],
  ['div', { start: null, cols: undefined, a: 'x' }, 'take'],
  ['div', { 0: 'zero', a: 'x' }, 'decline'],
  ['div', { 4294967295: 'not an index', a: 'x' }, 'take'],
  ['div', Object.create({ inherited: 'x' }, { own: { value: 'o', enumerable: true } }), 'decline'],
  ['div', Object.defineProperty({ a: 'x' }, 'hidden', { value: 'h', enumerable: false }), 'take'],
  ['div', Object.defineProperty({ a: 'x' }, 'g', { get: function () { return 'g' }, enumerable: true }), 'decline'],
  ['div', Object.defineProperty({ a: 'x' }, 'g', { get: function () { return 'g' }, enumerable: false }), 'take'],
  ['div', Object.create(null), 'decline'],
  ['div', (function () { const o = { a: '1', b: '2', c: '3' }; delete o.b; o.d = '4'; return o })(), 'take'],
  ['div', (function () { const o = {}; o[sym] = 's'; o.a = '1'; return o })(), 'take'],
  ['div', Object.freeze({ a: '1', children: 'c' }), 'take'],
  ['div', Object.fromEntries(Array.from({ length: 32 }, (_, i) => ['k' + i, i])), 'take'],
  ['div', Object.fromEntries(Array.from({ length: 33 }, (_, i) => ['k' + i, i])), 'decline'],
  ['div', new Proxy({ a: 'x' }, {}), 'decline'],
  ['div', ['x'], 'decline'],
  ['div', null, 'decline'],
  ['div', { className: 'abc-' + 'd'.repeat(300) }, 'take'],
  ['div', { 中文: 'x' }, 'decline'],
  ['div', { é: 'x' }, 'decline'],
  ['div', { ['x'.repeat(63)]: 'long' }, 'take'],
  ['div', { ['x'.repeat(64)]: 'long' }, 'decline'],
  ['div', { ['on' + 'x'.repeat(80)]: 'long' }, undefined],
  ['div', { ['Āx']: 'wide' }, 'decline'],
  ['div', { className: cls200 + ' ' + cls200 + ' ' + cls60, children: cls200 + '<' + cls200 }, 'take'],
  ['div', { title: cls200 + cls200 + cls200 + cls200 + cls200 + cls200 + cls200 }, 'take'],
  ['div', { href: cls200 + cls200 + cls200 + cls200 + cls200 + cls200 + cls200 }, undefined],
  ['div', { style: { color: cls60 + ' ' + cls200 + cls200 + cls200 + cls200 + cls200 } }, 'take'],
  ['my-element', { className: 'c', foo: 'bar' }, 'take'],
  ['x:y.z-1_2', { a: 'b' }, 'take'],
  ['A', { a: 'b' }, 'take'],
  ['1div', { a: 'b' }, 'decline'],
  ['di v', { a: 'b' }, 'decline'],
  ['', { a: 'b' }, 'decline'],
  ['dïv', { a: 'b' }, 'decline'],
  ['div', { a: 'b', children: 't' }, 'take'],
  [5, { a: 'b' }, 'decline'],
  [null, {}, 'decline'],
]
cases.forEach(([tag, props, want], i) => check('case ' + i + ' <' + show(tag) + '>', tag, props, want))

// children comes back as it is
{
  const t = [], c = { children: kids }
  if (startTag(t, c, 'ul', FC, hasOwnProperty, D) !== kids) fail('children not returned')
  const f = { children: fn }
  if (startTag(t, f, 'ul', FC, hasOwnProperty, D) !== fn) fail('function children not returned')
}

// what the call finds around it
check('another hasOwnProperty', 'div', { a: 1 }, 'decline', FC, function () {})
check('a bound hasOwnProperty', 'div', { a: 1 }, 'decline', FC, hasOwnProperty.bind(null))
check('Object.hasOwn as hasOwnProperty', 'div', { a: 1 }, 'decline', FC, Object.hasOwn)
check('hasOwnProperty a number', 'div', { a: 1 }, 'decline', FC, 5)
check('formatContext with no viewTransition', 'div', { a: 1 }, 'decline', { insertionMode: 2 })
check('viewTransition undefined', 'div', { a: 1 }, 'decline', { viewTransition: undefined })
check('viewTransition inherited', 'div', { a: 1 }, 'decline', Object.create({ viewTransition: null }))
check('viewTransition a getter', 'div', { a: 1 }, 'decline', { get viewTransition() { return null } })
check('viewTransition set', 'div', { a: 1 }, 'decline', { viewTransition: { name: 'auto', nameIdx: 0, update: 'u', enter: 'none', exit: 'none', share: 'none' } })
check('formatContext a number', 'div', { a: 1 }, 'decline', 5)
check('target frozen', 'div', { a: 1 }, 'decline', FC, null, Object.freeze([]))
check('target not extensible', 'div', { a: 1 }, 'decline', FC, null, Object.preventExtensions(['x']))
check('target sealed', 'div', { a: 1 }, 'decline', FC, null, Object.seal(['x']))
check('target with a property', 'div', { a: 1 }, 'decline', FC, null, Object.assign(['x'], { extra: 1 }))
check('target longer than its count', 'div', { a: 1 }, 'decline', FC, null, (() => { const a = ['x']; a.length = 3; return a })())
check('target with a hole', 'div', { a: 1 }, undefined, FC, null, [, 'x'])
check('target of a subclass', 'div', { a: 1 }, 'decline', FC, null, new (class extends Array {})())
check('target with another prototype', 'div', { a: 1 }, 'decline', FC, null, Object.setPrototypeOf([], Object.create(Array.prototype)))
check('target length read-only', 'div', { a: 1 }, 'decline', FC, null, Object.defineProperty([], 'length', { writable: false }))
check('target an array that grew', 'div', { a: 1 }, 'take', FC, null, Array.from({ length: 1000 }, (_, i) => 's' + i))
check('target made by new Array', 'div', { a: 1 }, 'take', FC, null, new Array())
{
  let grown = []
  for (let i = 0; i < 100; i++) check('target growing ' + i, 'li', { id: i }, 'take', FC, null, grown)
  if (grown.length !== 100 || grown[99] !== '<li id="99">') fail('target growing: ' + grown.length)
}
for (const t of [{ length: 0 }, new Uint8Array(2), 'abc', null]) {
  count++
  let r
  try { r = startTag(t, { a: 1 }, 'div', FC, hasOwnProperty, D) } catch (e) { r = e }
  if (r !== D) fail('target ' + show(t) + ': ' + show(r))
}
{
  const call = Function.prototype.call
  Function.prototype.call = function () { return call.apply(this, arguments) }
  check('Function.prototype.call replaced', 'div', { a: 1 }, 'decline')
  Object.defineProperty(Function.prototype, 'call', { get() { return call }, configurable: true })
  check('Function.prototype.call a getter', 'div', { a: 1 }, 'decline')
  Object.defineProperty(Function.prototype, 'call', { value: call, writable: true, configurable: true })
  check('Function.prototype.call put back', 'div', { a: 1 }, 'take')
  const push = Array.prototype.push
  Array.prototype.push = function () { return push.apply(this, arguments) }
  check('Array.prototype.push replaced', 'div', { a: 1 }, 'decline')
  Array.prototype.push = Array.prototype.unshift
  check('Array.prototype.push another built-in', 'div', { a: 1 }, 'decline')
  Array.prototype.push = push
  check('Array.prototype.push put back', 'div', { a: 1 }, 'take')
  Object.defineProperty(Object.prototype, 'title', { set(v) { fail('setter ran') }, configurable: true })
  check('Object.prototype with a setter', 'div', { title: 't' }, 'take')
  delete Object.prototype.title
  Object.prototype.inherited = 'x'
  check('Object.prototype with an enumerable key', 'div', { a: 1 }, 'take')
  delete Object.prototype.inherited
}

// many names, more than the caches hold, twice
for (let round = 0; round < 2; round++) {
  for (let i = 0; i < 1200; i++)
    check('name ' + i, 'div', { ['n' + i]: i, style: { ['s' + i]: i, ['--c' + i]: 'v', ['msW' + i]: 1 } }, 'take')
}

// random props, the same each run
let seed = 0x2545f491
function rand(n) {
  seed ^= seed << 13; seed >>>= 0
  seed ^= seed >>> 17
  seed ^= seed << 5; seed >>>= 0
  return seed % n
}
const pick = a => a[rand(a.length)]
const pieces = ['a', 'b c', '<', '>', '&', '"', '\'', '中', '\ud800', '  ', '\t', 'javascript:', 'JaVa\tscript:x', ' ', 'x'.repeat(40), 'é', ' ']
function str() {
  let s = ''
  for (let i = rand(5); i > 0; i--) s += pick(pieces)
  return s
}
const numbers = [0, -0, 1, -1, 0.5, NaN, Infinity, -1.25e-7, 1e21, 2 ** 31, -(2 ** 31), 42]
function value() {
  switch (rand(17)) {
  case 0: return null
  case 1: return undefined
  case 2: return true
  case 3: return false
  case 4: return rand(200) - 100
  case 5: return pick(numbers)
  case 6: return fn
  case 7: return sym
  case 8: return {}
  case 9: return 1n
  case 10: return ''
  case 11: return [1, 2]
  case 12: return str() + str()
  default: return str()
  }
}
const names = ['className', 'tabIndex', 'dir', 'role', 'viewBox', 'width', 'height', 'src', 'href', 'action', 'formAction',
  'defaultValue', 'ref', 'autoFocus', 'multiple', 'muted', 'contentEditable', 'value', 'draggable', 'inert', 'disabled',
  'hidden', 'open', 'xlinkActuate', 'xmlLang', 'htmlFor', 'crossOrigin', 'strokeWidth', 'fillRule', 'panose-1', 'data-id',
  'data-x', 'aria-label', 'aria-hidden', 'DATA-up', 'id', 'title', 'alt', 'type', 'name', 'onClick', 'onclick', 'on', 'One',
  'a b', '1a', 'x"y', 'children', 'é', '中', 'xlinkHref', 'download', 'cols', 'dangerouslySetInnerHTML', '_x', ':x', 'a.b-c',
  'innerHTML', 'suppressHydrationWarning', 'style', 'style', 'children\u0000x', 'id\u0000x']
const styleNames = ['color', 'marginTop', 'zIndex', 'opacity', 'lineHeight', 'WebkitTransform', 'msTransform', 'msFlex',
  'MozBoxFlex', '--brand', '--x<', 'fontFamily', '颜色', 'ms', 'mSx', 'content', 'width', 'flexGrow', 'a&b', '--中']
function styleValue() {
  switch (rand(10)) {
  case 0: return null
  case 1: return rand(2) === 0
  case 2: return ''
  case 3: return rand(100) - 50
  case 4: return pick(numbers)
  case 5: return rand(20) === 0 ? pick([{}, fn, sym, 2n]) : 'red'
  default: return str()
  }
}
function style() {
  switch (rand(12)) {
  case 0: return 'color:red'
  case 1: return null
  case 2: return [1]
  default: {
    const s = {}
    for (let i = rand(6); i > 0; i--) s[pick(styleNames)] = styleValue()
    return s
  }
  }
}
function props() {
  const p = rand(30) === 0 ? Object.create(rand(2) ? null : { x: 1 }) : {}
  for (let i = rand(9); i > 0; i--) {
    const k = pick(names)
    p[k] = k === 'style' ? style() : k === 'children' && rand(2) ? str() : value()
  }
  switch (rand(40)) {
  case 0: Object.defineProperty(p, 'g', { get() { return 'g' }, enumerable: rand(2) === 0 }); break
  case 1: Object.defineProperty(p, 'hid', { value: 'h', enumerable: false }); break
  case 2: p[sym] = 1; break
  case 3: p[rand(3)] = 'i'; break
  case 4: for (const k in p) { delete p[k]; break } break
  }
  return p
}
for (let t = 0; t < 4000; t++)
  check('random ' + t, rand(20) === 0 ? pick(['1x', 'my-el', 'svg', 'x:y', '']) : 'div', props())

// An index on Array.prototype makes push set it there, and React's push runs
// the setter. startTag declines from then on, as push leaves its fast path.
// Last of the calls, since the engine never takes back that Array.prototype
// has had an index.
{
  Object.defineProperty(Array.prototype, '0', { set(v) { throw new Error('setter ran') }, configurable: true })
  check('Array.prototype with a setter at 0', 'div', { a: 1 }, 'decline', FC, null, [])
  delete Array.prototype[0]
  check('Array.prototype that had an index', 'div', { a: 1 }, 'decline', FC, null, [])
}

out.unshift(count + ' calls: ' + (bad ? bad + ' wrong' : 'all as React') + '; startTag took ' + taken)

// endTag against endChunkForTag
{
  const tags = ['div', 'span', 'x:y', '中文', '\ud800', '', 'a'.repeat(1000), 5, -0, 1.5, true, null, undefined, kids, 1n,
    { toString() { return 'obj' } }, { valueOf() { return 7 }, toString() { return 'no' } },
    { [Symbol.toPrimitive](h) { return 'prim-' + h } }, { [Symbol.toPrimitive]() { return 3 } }]
  let ends = 0
  for (const t of tags) {
    for (let k = 0; k < 2; k++) {
      ends++
      const want = endChunkForTag(t)
      endTagCache.clear()
      if (endTag(t) !== want) fail('endTag ' + show(t) + ': ' + show(endTag(t)) + ', React ' + show(want))
    }
  }
  for (let round = 0; round < 2; round++) {
    for (let i = 0; i < 300; i++) {
      ends++
      const t = 'tag' + i
      if (endTag(t) !== '</' + t + '>') fail('endTag ' + t)
    }
  }
  for (const t of [sym, { toString() { throw new RangeError('no') } }, { toString: null, valueOf: null }]) {
    ends++
    let a, b
    try { endChunkForTag(t); a = 'none' } catch (e) { a = e.name + ': ' + e.message }
    try { endTag(t); b = 'none' } catch (e) { b = e.name + ': ' + e.message }
    if (a !== b) fail('endTag throws ' + b + ', React ' + a)
  }
  out.push(ends + ' end tags: ' + (bad ? 'see above' : 'all as React'))
}

// the functions are built-ins
for (const [f, name, length] of [[startTag, 'startTag', 6], [endTag, 'endTag', 1]]) {
  const shape = JSON.stringify([f.name, f.length, Object.hasOwn(f, 'prototype'), Object.getPrototypeOf(f) === Function.prototype,
    (() => { try { new f([], {}, 'div', FC, hasOwnProperty, D); return 'made' } catch (e) { return e.name } })(), String(f).includes('[native code]')])
  if (shape !== JSON.stringify([name, length, false, true, 'TypeError', true])) fail('shape of ' + name + ': ' + shape)
}
out.push(bad ? bad + ' failed' : 'ok')
out.join('\n')
