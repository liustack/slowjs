// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Classes in the register loop through OLD (engine/regvm-ops-old.h):
// define_class and define_class_computed, define_method_computed,
// set_home_object, set_name_computed, set_proto, the home object, get_super,
// get_super_value and put_super_value, put_var_ref_check_init (super() in an
// arrow function), and each of them throwing with temps owned below it.
// Each target runs at least twice from the driver: the first call converts
// it, the next ones run in the register loop. The class constructors run in
// the old loop (new), until constructor frames are in.
// rv-expect-op: OLD
(function () {
  "use strict";
  var log = [];
  function L(s) { log.push(s); }
  var o = { k: "k", n: 1 };
  function notCtor() {}
  var arrow = () => 1;
  var badProto = function () {};
  badProto.prototype = 5;

  // a class declaration and a class expression, with and without heritage
  function plain() {
    class B { m() { return "m"; } static s() { return "s"; } }
    return B;
  }
  function expr(x) {
    return class extends x { get g() { return "g" + super.constructor.name; } };
  }
  function anon() {
    var C = class {};
    var D = class Named {};
    return C.name + "," + D.name;
  }
  // heritage that is not a constructor, a null heritage, a prototype that is
  // not an object: thrown by define_class with temps below it
  function heritage(x) {
    return [o.k, class extends x {}, o.n];
  }
  // computed class name: a class in a computed key (define_class_computed),
  // computed method keys of every kind (define_method_computed), getters and
  // setters with computed keys
  function computed(k) {
    var obj = { [k]: class {}, [k + "f"]: function () {}, [k + "a"]: () => 1 };
    class C {
      [k]() { return "method " + k; }
      static [k + "s"]() { return "static"; }
      get [k + "g"]() { return "getter"; }
      set [k + "g"](v) { this.sv = v; }
      [Symbol.iterator]() { return [1, 2][Symbol.iterator](); }
      [1 + 1]() { return "two"; }
    }
    var c = new C();
    c[k + "g"] = 9;
    return [obj[k].name, obj[k + "f"].name, obj[k + "a"].name, c[k](), C[k + "s"](),
            c[k + "g"], c.sv, [...c].join(), c[2](), Object.keys(C.prototype).length];
  }
  // a computed key whose conversion runs JS and throws
  function keyThrows(k) {
    return [o.k, { [k]() {} }, o.n];
  }
  function keyThrowsClass(k) {
    return [o.k, class { [k]() {} }, o.n];
  }
  // object literals: __proto__ (set_proto), methods with super
  // (set_home_object, the home object, get_super_value, put_super_value)
  function literal(p) {
    var obj = {
      __proto__: p,
      own: "own",
      m() { return super.who; },
      set s(v) { super.who = v; },
      get g() { return super.who + "!"; },
      call() { return super.hello(); },
      [o.k]() { return super.who; }
    };
    return obj;
  }
  function protoKinds() {
    var a = { __proto__: null }, b = { __proto__: 5 }, c = { __proto__: undefined };
    return [Object.getPrototypeOf(a), Object.getPrototypeOf(b) === Object.prototype,
            Object.getPrototypeOf(c) === Object.prototype];
  }
  function useLiteral(obj) {
    obj.s = "set";
    return [obj.m(), obj.g, obj.call(), obj.k(), Object.prototype.hasOwnProperty.call(obj, "who"), obj.who];
  }
  // super property reads and writes from class methods
  class Base {
    constructor() { this.b = 1; }
    get who() { return "base " + this.b; }
    hello() { return "hello " + this.b; }
    static sm() { return "base static"; }
  }
  class Derived extends Base {
    who2() { return super.who; }
    hello() { return "derived " + super.hello(); }
    set put(v) { super.b = v; }
    get keyed() { return super[o.k + "x"]; }
    callKeyed() { return super["hel" + "lo"](); }
    static sm() { return "derived " + super.sm(); }
    arrowSuper() { var f = () => super.hello(); return f(); }
  }
  var dm = Derived.prototype;
  var whoD = dm.who2, helloD = dm.hello, keyedD = Object.getOwnPropertyDescriptor(dm, "keyed").get;
  var putD = Object.getOwnPropertyDescriptor(dm, "put").set, callKeyedD = dm.callKeyed;
  var smD = Derived.sm, arrowSuperD = dm.arrowSuper;
  // super.x where the home object's prototype is null, and a getter that throws
  var nullHome = { m() { return [o.k, super.x, o.n]; } };
  Object.setPrototypeOf(nullHome, null);
  var throwingSuper = { __proto__: { get x() { throw new Error("getter " + this.k); } },
                        k: "tk", m() { return [o.k, super.x, o.n]; },
                        set w(v) { super.x = v; } };
  var frozenSuper = { __proto__: Object.freeze({ x: 1 }), m() { super.x = 2; return "no throw"; } };
  var nullHomeM = nullHome.m, throwingSuperM = throwingSuper.m, frozenSuperM = frozenSuper.m;
  var throwingSuperW = Object.getOwnPropertyDescriptor(throwingSuper, "w").set;

  // super() in an arrow function initializes the this of the constructor
  // through put_var_ref_check_init, and throws when it is already set
  var superArrow;
  class ArrowBase { constructor(v) { this.v = v; L("ArrowBase " + v); } }
  class ArrowDerived extends ArrowBase {
    constructor(v, twice) {
      var f = (x) => super(x);
      superArrow = f;
      f(v);
      if (twice)
        f(v + 1);
    }
  }
  return function () {
    function run(name, f) {
      for (var i = 0; i < 3; i++) {
        try {
          L(name + " " + i + ": " + JSON.stringify(f()));
        } catch (e) {
          L(name + " " + i + " threw " + e.name + ": " + e.message);
        }
      }
    }
    run("plain", function () { var B = plain(); return [B.name, new B().m(), B.s()]; });
    run("expr", function () { var C = expr(Base); return [C.name, new C().g, new C().b]; });
    run("expr object", function () { var C = expr(Object); return new C().g; });
    run("anon", anon);
    run("heritage null", function () { var C = heritage(null)[1]; return Object.getPrototypeOf(C.prototype); });
    run("heritage ok", function () { var r = heritage(Base); return [r[0], new r[1]().b, r[2]]; });
    run("heritage 5", function () { return heritage(5); });
    run("heritage arrow", function () { return heritage(arrow); });
    run("heritage bad proto", function () { return heritage(badProto); });
    run("heritage plain function", function () { return new (heritage(notCtor)[1])() instanceof notCtor; });
    run("computed", function () { return computed("x"); });
    run("computed number", function () { return computed(3); });
    run("key object", function () { return Object.keys(keyThrows({ toString: function () { L("toString"); return "kk"; } })[1]); });
    run("key throws", function () { return keyThrows({ toString: function () { throw new Error("key"); } }); });
    run("key throws class", function () { return keyThrowsClass({ toString: function () { throw new TypeError("ckey"); } }); });
    run("key symbol", function () { return keyThrows(Symbol("s"))[1] !== undefined; });
    run("literal", function () { return useLiteral(literal({ who: "proto", hello: function () { return "hi " + this.own; } })); });
    run("literal proto getter", function () {
      return useLiteral(literal({ get who() { return "getter of " + this.own; }, set who(v) { L("setter " + v); },
                                  hello: function () { return "h"; } }));
    });
    run("proto kinds", protoKinds);
    run("super who", function () { return whoD.call(new Derived()); });
    run("super hello", function () { return helloD.call(new Derived()); });
    run("super keyed", function () { return keyedD.call(new Derived()); });
    run("super put", function () { var d = new Derived(); putD.call(d, 7); return [d.b, d.hasOwnProperty("b")]; });
    run("super put this primitive", function () { return putD.call(5, 7); });
    run("super call keyed", function () { return callKeyedD.call(new Derived()); });
    run("super static", function () { return smD.call(Derived); });
    run("super arrow", function () { return arrowSuperD.call(new Derived()); });
    run("super null home", function () { return nullHomeM.call(nullHome); });
    run("super getter throws", function () { return throwingSuperM.call(throwingSuper); });
    run("super frozen", function () { return frozenSuperM.call(frozenSuper); });
    run("super setter missing", function () { return throwingSuperW.call(throwingSuper, 1); });
    run("super in arrow", function () { return new ArrowDerived(4).v; });
    run("super in arrow twice", function () { return new ArrowDerived(5, true).v; });
    rvExpect(plain, expr, anon, heritage, computed, keyThrows, keyThrowsClass, literal, protoKinds,
             useLiteral, whoD, helloD, keyedD, callKeyedD, smD, arrowSuperD, nullHomeM,
             throwingSuperM, superArrow);
    rvExpect(putD, frozenSuperM, throwingSuperW); // a store to super leaves its value with insert4
    rvExpect(Base, Derived);
    return log.join("\n");
  };
})()
