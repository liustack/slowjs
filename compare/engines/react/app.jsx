// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A catalog page of a few hundred elements, rendered with React 19's
// renderToString. The data is generated from a fixed seed, so every engine
// renders the same page, and Node's output is the reference.
import { createContext, useContext, useId, useMemo, useState, Fragment } from "react";
import { renderToString } from "react-dom/server";

function rng(seed) {
  let s = seed >>> 0;
  return () => {
    s = (Math.imul(s, 1664525) + 1013904223) >>> 0;
    return s / 4294967296;
  };
}

const WORDS = ["oak", "linen", "ceramic", "steel", "wool", "walnut", "glass", "copper",
  "cotton", "maple", "slate", "brass", "<tag>", "\"quoted\"", "R&D", "café", "日本", "naïve"];

function makeData(seed, n) {
  const r = rng(seed);
  const pick = () => WORDS[Math.floor(r() * WORDS.length)];
  const products = [];
  for (let i = 0; i < n; i++) {
    products.push({
      id: "p" + (1000 + i),
      name: pick() + " " + pick() + " " + (i + 1),
      price: Math.round(r() * 100000) / 100,
      rating: Math.round(r() * 50) / 10,
      stock: Math.floor(r() * 20),
      tags: [pick(), pick(), pick()],
      sale: r() < 0.3,
    });
  }
  const categories = [];
  for (let i = 0; i < 16; i++) categories.push({ id: "c" + i, label: pick() + " " + i, count: Math.floor(r() * 300) });
  return { products, categories, user: { name: "Ada <Lovelace>", cart: 3 } };
}

const Locale = createContext({ currency: "$", sep: "," });

function money(locale, v) {
  const [int, frac] = v.toFixed(2).split(".");
  let out = "";
  for (let i = 0; i < int.length; i++) {
    if (i > 0 && (int.length - i) % 3 === 0) out += locale.sep;
    out += int[i];
  }
  return locale.currency + out + "." + frac;
}

function Stars({ rating }) {
  const full = Math.floor(rating);
  return (
    <span className="stars" aria-label={rating + " out of 5"}>
      {"★".repeat(full)}{"☆".repeat(5 - full)}
    </span>
  );
}

function Price({ value, sale }) {
  const locale = useContext(Locale);
  return sale ? (
    <p className="price sale"><del>{money(locale, value * 1.25)}</del> <strong>{money(locale, value)}</strong></p>
  ) : (
    <p className="price">{money(locale, value)}</p>
  );
}

function ProductCard({ p }) {
  const id = useId();
  const [qty] = useState(1);
  return (
    <article className={"card" + (p.stock === 0 ? " out" : "")} data-id={p.id} aria-labelledby={id}>
      <img src={"/img/" + p.id + ".webp"} alt={p.name} width={240} height={180} loading="lazy" />
      <h3 id={id}>{p.name}</h3>
      <Price value={p.price} sale={p.sale} />
      <Stars rating={p.rating} />
      <ul className="tags">
        {p.tags.map((t, i) => <li key={i}>{t}</li>)}
      </ul>
      {p.stock > 0
        ? <button type="button" value={qty} disabled={false}>Add to cart</button>
        : <button type="button" disabled>Sold out</button>}
    </article>
  );
}

function Sidebar({ categories }) {
  return (
    <aside className="filters">
      <h2>Categories</h2>
      <ul>
        {categories.map(c => (
          <li key={c.id}>
            <label><input type="checkbox" name="cat" value={c.id} defaultChecked={c.count > 150} /> {c.label} <small>({c.count})</small></label>
          </li>
        ))}
      </ul>
    </aside>
  );
}

function Header({ user }) {
  const links = ["Home", "New", "Living", "Kitchen", "Office", "Outdoor", "Sale", "About"];
  return (
    <header style={{ display: "flex", justifyContent: "space-between", padding: 12 }}>
      <nav>{links.map(l => <a key={l} href={"/" + l.toLowerCase()}>{l}</a>)}</nav>
      <div className="user">Hello, {user.name} · <a href="/cart">Cart ({user.cart})</a></div>
    </header>
  );
}

function App({ data }) {
  const inStock = useMemo(() => data.products.filter(p => p.stock > 0).length, [data]);
  return (
    <Locale.Provider value={{ currency: "$", sep: "," }}>
      <div id="app">
        <Header user={data.user} />
        <main>
          <Sidebar categories={data.categories} />
          <section className="grid">
            <h1>Catalog <small>{inStock} of {data.products.length} in stock</small></h1>
            {data.products.map(p => <ProductCard key={p.id} p={p} />)}
          </section>
        </main>
        <footer><Fragment>© 2026 <em>jsbench</em></Fragment><div dangerouslySetInnerHTML={{ __html: "<!-- static -->" }} /></footer>
      </div>
    </Locale.Provider>
  );
}

const DATA = makeData(42, 60);

globalThis.render = function () {
  return renderToString(<App data={DATA} />);
};
