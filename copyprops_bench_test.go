// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import "testing"

// copyPropsBenchSource has createElement and cloneElement as React 19.3's production build writes them,
// and the same two as tifa's Vite plugin writes them for a VM with the function of RegisterCopyProps.
const copyPropsBenchSource = `
var hasOwnProperty = Object.prototype.hasOwnProperty, assign = Object.assign;
var REACT_ELEMENT_TYPE = Symbol.for("react.transitional.element");
function ReactElement(type, key, props) {
  var refProp = props.ref;
  return { $$typeof: REACT_ELEMENT_TYPE, type: type, key: key, ref: void 0 !== refProp ? refProp : null, props: props };
}
function createReact(type, config, children) {
  var propName,
    props = {},
    key = null;
  if (null != config)
    for (propName in (void 0 !== config.key && (key = "" + config.key), config))
      hasOwnProperty.call(config, propName) &&
        "key" !== propName &&
        "__self" !== propName &&
        "__source" !== propName &&
        (props[propName] = config[propName]);
  var childrenLength = arguments.length - 2;
  if (1 === childrenLength) props.children = children;
  else if (1 < childrenLength) {
    for (var childArray = Array(childrenLength), i = 0; i < childrenLength; i++)
      childArray[i] = arguments[i + 2];
    props.children = childArray;
  }
  if (type && type.defaultProps)
    for (propName in ((childrenLength = type.defaultProps), childrenLength))
      void 0 === props[propName] &&
        (props[propName] = childrenLength[propName]);
  return ReactElement(type, key, props);
}
function createNative(type, config, children) {
  var propName,
    props = {},
    key = null;
  if (null != config)
    if (
      (void 0 !== config.key && (key = "" + config.key),
      !tifaCopyProps || !tifaCopyProps(config, props, hasOwnProperty, false))
    )
      for (propName in config)
        hasOwnProperty.call(config, propName) &&
          "key" !== propName &&
          "__self" !== propName &&
          "__source" !== propName &&
          (props[propName] = config[propName]);
  var childrenLength = arguments.length - 2;
  if (1 === childrenLength) props.children = children;
  else if (1 < childrenLength) {
    for (var childArray = Array(childrenLength), i = 0; i < childrenLength; i++)
      childArray[i] = arguments[i + 2];
    props.children = childArray;
  }
  if (type && type.defaultProps)
    for (propName in ((childrenLength = type.defaultProps), childrenLength))
      void 0 === props[propName] &&
        (props[propName] = childrenLength[propName]);
  return ReactElement(type, key, props);
}
function cloneReact(element, config, children) {
  var props = assign({}, element.props),
    key = element.key;
  if (null != config)
    for (propName in (void 0 !== config.key && (key = "" + config.key), config))
      !hasOwnProperty.call(config, propName) ||
        "key" === propName ||
        "__self" === propName ||
        "__source" === propName ||
        ("ref" === propName && void 0 === config.ref) ||
        (props[propName] = config[propName]);
  var propName = arguments.length - 2;
  if (1 === propName) props.children = children;
  else if (1 < propName) {
    for (var childArray = Array(propName), i = 0; i < propName; i++)
      childArray[i] = arguments[i + 2];
    props.children = childArray;
  }
  return ReactElement(element.type, key, props);
}
function cloneNative(element, config, children) {
  var props = assign({}, element.props),
    key = element.key;
  if (null != config)
    if (
      (void 0 !== config.key && (key = "" + config.key),
      !tifaCopyProps || !tifaCopyProps(config, props, hasOwnProperty, true))
    )
      for (propName in config)
        !hasOwnProperty.call(config, propName) ||
          "key" === propName ||
          "__self" === propName ||
          "__source" === propName ||
          ("ref" === propName && void 0 === config.ref) ||
          (props[propName] = config[propName]);
  var propName = arguments.length - 2;
  if (1 === propName) props.children = children;
  else if (1 < propName) {
    for (var childArray = Array(propName), i = 0; i < propName; i++)
      childArray[i] = arguments[i + 2];
    props.children = childArray;
  }
  return ReactElement(element.type, key, props);
}
`

// BenchmarkCopyProps times one createElement (create) and one cloneElement (clone) of React with the loop
// that copies the config (react), with the code tifa's plugin writes in a VM that has no function (loop: the
// same code, tifaCopyProps null), and with the function of RegisterCopyProps (native), on configs of the
// size of those of a page: none, one key, a key and one or two more, five and ten keys, and for the clone
// an element with two props. Each call gets a new config, as JSX does. ns/op is per call, the loop and the
// literal of the config included. "empty" is that loop and literal alone.
func BenchmarkCopyProps(b *testing.B) {
	configs := []struct{ name, lit string }{
		{"0-keys", `{}`},
		{"1-key", `{ className: 'x' }`},
		{"key+1", `{ key: i, className: 'x' }`},
		{"key+2", `{ key: i, className: 'x', id: 'y' }`},
		{"3-keys", `{ className: 'x', id: 'y', title: 'z' }`},
		{"5-keys", `{ className: 'x', id: 'y', title: 'z', role: 'button', tabIndex: 0 }`},
		{"10-keys", `{ className: 'x', id: 'y', title: 'z', role: 'button', tabIndex: 0, onClick: f, onKeyDown: f, style: s, disabled: false, children: 'c' }`},
		{"key+5", `{ key: i, className: 'x', id: 'y', title: 'z', role: 'button', tabIndex: 0 }`},
		{"ref+2", `{ ref: f, className: 'x', id: 'y' }`},
	}
	for _, kind := range []string{"create", "clone"} {
		for _, c := range configs {
			for _, v := range []string{"empty", "react", "loop", "native"} {
				b.Run(kind+"/"+c.name+"/"+v, func(b *testing.B) {
					m, err := NewVM()
					if err != nil {
						b.Fatal(err)
					}
					b.Cleanup(func() { m.Close() })
					native := v == "native"
					if native {
						if err := m.RegisterCopyProps("__copy"); err != nil {
							b.Fatal(err)
						}
					}
					call := map[string]string{
						"create/empty": `empty(0, ` + c.lit + `)`, "clone/empty": `empty(0, ` + c.lit + `)`,
						"create/react": `createReact('div', ` + c.lit + `)`, "clone/react": `cloneReact(base, ` + c.lit + `)`,
						"create/loop": `createNative('div', ` + c.lit + `)`, "clone/loop": `cloneNative(base, ` + c.lit + `)`,
						"create/native": `createNative('div', ` + c.lit + `)`, "clone/native": `cloneNative(base, ` + c.lit + `)`,
					}[kind+"/"+v]
					tifa := "null"
					if native {
						tifa = "__copy"
					}
					if _, err := m.Eval(copyPropsBenchSource+`
var tifaCopyProps = `+tifa+`;
function empty(a, b) { return b }
function f() {}
var s = { color: 'red' };
var base = createReact('div', { a: 'base', z: 'last' });
function loop(n) { for (var i = 0; i < n; i++) `+call+` }
`, EvalGlobal); err != nil {
						b.Fatal(err)
					}
					b.ResetTimer()
					if _, err := m.Call("loop", b.N); err != nil {
						b.Fatal(err)
					}
				})
			}
		}
	}
}
