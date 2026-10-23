### Pages without Intl

| App | Page | Same page with Intl removed | Intl it reaches (first caller) |
|---|---|---|---|
| bench | spiral | yes | nothing |
| bench | table | yes | nothing |
| mui | home | no | `Intl.NumberFormat` ×19, `Number.prototype.toLocaleString` ×2, `Intl.DateTimeFormat` ×12 |
| mui | order | no | `Intl.NumberFormat` ×5, `Intl.DateTimeFormat` ×1 |
| mui | orders | no | `Intl.NumberFormat` ×27, `Intl.DateTimeFormat` ×25 |
| shop | store-en-US-cart | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×15, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×5 |
| shop | store-en-US-checkout | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×11, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×5 |
| shop | store-en-US-home | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×20, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×3 |
| shop | store-en-US-journal | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×3 |
| shop | store-en-US-post | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×3 |
| shop | store-en-US-product | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×13, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×4 |
| shop | store-en-US-products | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×32, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1 |
| shop | store-zh-CN-cart | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×15, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×5 |
| shop | store-zh-CN-checkout | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×11, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×5 |
| shop | store-zh-CN-home | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×20, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×3 |
| shop | store-zh-CN-journal | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×3 |
| shop | store-zh-CN-post | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×3 |
| shop | store-zh-CN-product | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×13, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1, `Intl.DateTimeFormat` ×4 |
| shop | store-zh-CN-products | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×31, `Intl.getCanonicalLocales` ×1, `Intl.PluralRules` ×1 |
| shop | wb-order-studio | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.DateTimeFormat` ×1 |
| shop | wb-order-weekend | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.DateTimeFormat` ×1 |
| shop | wb-stats-studio | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.DateTimeFormat` ×1 |
| shop | wb-stats-weekend | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.DateTimeFormat` ×1 |
| shop | wb-studio | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.DateTimeFormat` ×1 |
| shop | wb-weekend | no: load: ReferenceError: 'Intl' is not defined | `Intl.NumberFormat` ×3, `Intl.DateTimeFormat` ×1 |

### Pages without Intl (bench)

| Engine | Load ms | Pages that render | Same page as liustack-quickjs | Time per render vs liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs | 65 | 2 of 2 | 2 of 2 | 1.00 (1.00–1.00) |
| liustack-tifa | 65 | 2 of 2 | 2 of 2 | 0.99 (0.98–1.00) |
| modernc-quickjs | not run | | | |
| go-quickjs-interp | not run | | | |
| go-quickjs-jit | not run | | | |
| goant-interp | not run | | | |
| goant-jit | not run | | | |
| moejs | not run | | | |
| C QuickJS 2026-06-04 (reference) | 41 | 2 of 2 | 2 of 2 | 0.67 (0.65–0.69) |
| C QuickJS master 7d1c13c5d8 (reference) | not run | | | |
| QuickJS-NG v0.17.0 (reference) | not run | | | |
| Node v24.11.1 (reference) | 44 | 2 of 2 | 2 of 2 | 0.10 (0.08–0.13) |
| Node v24.11.1 --jitless (reference) | 71 | 2 of 2 | 2 of 2 | 0.35 (0.34–0.35) |

| Page | liustack-quickjs | liustack-tifa | modernc-quickjs | go-quickjs-interp | go-quickjs-jit | goant-interp | goant-jit | moejs | quickjs-2026-06-04 | quickjs-master | quickjs-ng | node-jit | node-jitless |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spiral | 31.5 (1.00) | 30.9 (0.98) |  |  |  |  |  |  | 20.3 (0.65) |  |  | 4.11 (0.13) | 11 (0.35) |
| table | 28.6 (1.00) | 28.5 (1.00) |  |  |  |  |  |  | 19.8 (0.69) |  |  | 2.4 (0.08) | 9.72 (0.34) |

### Shop on the FormatJS polyfill, every engine

| Engine | Load ms | Pages that render | Same page as liustack-quickjs | Time per render vs liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs | 660 | 20 of 20 | 20 of 20 | 1.00 (1.00–1.00) |
| liustack-tifa | not run: this row has no polyfill | | | |
| modernc-quickjs | not run | | | |
| go-quickjs-interp | not run | | | |
| go-quickjs-jit | not run | | | |
| goant-interp | not run | | | |
| goant-jit | not run | | | |
| moejs | not run | | | |
| C QuickJS 2026-06-04 (reference) | 380 | 20 of 20 | 20 of 20 | 0.69 (0.63–0.78) |
| C QuickJS master 7d1c13c5d8 (reference) | not run | | | |
| QuickJS-NG v0.17.0 (reference) | not run | | | |
| Node v24.11.1 (reference) | 266 | 20 of 20 | 20 of 20 | 0.16 (0.13–0.20) |
| Node v24.11.1 --jitless (reference) | 271 | 20 of 20 | 20 of 20 | 0.40 (0.35–0.42) |

| Page | liustack-quickjs | liustack-tifa | modernc-quickjs | go-quickjs-interp | go-quickjs-jit | goant-interp | goant-jit | moejs | quickjs-2026-06-04 | quickjs-master | quickjs-ng | node-jit | node-jitless |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| store-en-US-cart | 37.1 (1.00) |  |  |  |  |  |  |  | 25 (0.67) |  |  | 7.47 (0.20) | 14.6 (0.39) |
| store-en-US-checkout | 34.9 (1.00) |  |  |  |  |  |  |  | 25.1 (0.72) |  |  | 6.99 (0.20) | 14.5 (0.42) |
| store-en-US-home | 59.1 (1.00) |  |  |  |  |  |  |  | 40.2 (0.68) |  |  | 8.9 (0.15) | 23.3 (0.39) |
| store-en-US-journal | 11.9 (1.00) |  |  |  |  |  |  |  | 7.48 (0.63) |  |  | 1.68 (0.14) | 4.35 (0.37) |
| store-en-US-post | 111 (1.00) |  |  |  |  |  |  |  | 74.3 (0.67) |  |  | 18.9 (0.17) | 39.1 (0.35) |
| store-en-US-product | 58.2 (1.00) |  |  |  |  |  |  |  | 39.4 (0.68) |  |  | 8.71 (0.15) | 23 (0.40) |
| store-en-US-products | 100 (1.00) |  |  |  |  |  |  |  | 68.1 (0.68) |  |  | 14.2 (0.14) | 41.1 (0.41) |
| store-zh-CN-cart | 37.2 (1.00) |  |  |  |  |  |  |  | 25.3 (0.68) |  |  | 5.66 (0.15) | 15 (0.40) |
| store-zh-CN-checkout | 35.1 (1.00) |  |  |  |  |  |  |  | 25.1 (0.71) |  |  | 5.65 (0.16) | 14.6 (0.42) |
| store-zh-CN-home | 59 (1.00) |  |  |  |  |  |  |  | 39.4 (0.67) |  |  | 7.49 (0.13) | 23.9 (0.40) |
| store-zh-CN-journal | 11.3 (1.00) |  |  |  |  |  |  |  | 7.73 (0.68) |  |  | 1.56 (0.14) | 4.61 (0.41) |
| store-zh-CN-post | 86.6 (1.00) |  |  |  |  |  |  |  | 57.9 (0.67) |  |  | 16.7 (0.19) | 32.1 (0.37) |
| store-zh-CN-product | 58.8 (1.00) |  |  |  |  |  |  |  | 40.7 (0.69) |  |  | 7.79 (0.13) | 24 (0.41) |
| store-zh-CN-products | 98.8 (1.00) |  |  |  |  |  |  |  | 67.7 (0.68) |  |  | 13.9 (0.14) | 41.4 (0.42) |
| wb-order-studio | 12.2 (1.00) |  |  |  |  |  |  |  | 9.37 (0.77) |  |  | 2.3 (0.19) | 5.04 (0.41) |
| wb-order-weekend | 12.4 (1.00) |  |  |  |  |  |  |  | 8.55 (0.69) |  |  | 1.85 (0.15) | 4.89 (0.40) |
| wb-stats-studio | 11.1 (1.00) |  |  |  |  |  |  |  | 8.08 (0.73) |  |  | 1.69 (0.15) | 4.29 (0.39) |
| wb-stats-weekend | 10.2 (1.00) |  |  |  |  |  |  |  | 7.9 (0.78) |  |  | 1.39 (0.14) | 4.2 (0.41) |
| wb-studio | 44.3 (1.00) |  |  |  |  |  |  |  | 30.9 (0.70) |  |  | 7.32 (0.17) | 17.7 (0.40) |
| wb-weekend | 43.6 (1.00) |  |  |  |  |  |  |  | 30.2 (0.69) |  |  | 6.94 (0.16) | 17.5 (0.40) |

### Shop as deployed: own Intl where the engine has one

| Engine | Load ms | Pages that render | Same page as liustack-quickjs | Time per render vs liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs (own Intl) | 302 | 20 of 20 | 20 of 20 | 1.00 (1.00–1.00) |
| liustack-tifa (own Intl) | 303 | 20 of 20 | 20 of 20 | 0.99 (0.95–1.02) |
| modernc-quickjs (polyfill) | not run | | | |
| go-quickjs-interp (own Intl) | not run | | | |
| go-quickjs-jit (own Intl) | not run | | | |
| goant-interp (polyfill) | not run | | | |
| goant-jit (polyfill) | not run | | | |
| moejs (polyfill) | not run | | | |
| C QuickJS 2026-06-04 (reference) (polyfill) | 380 | 20 of 20 | 20 of 20 | 0.92 (0.70–1.06) |
| C QuickJS master 7d1c13c5d8 (reference) (polyfill) | not run | | | |
| QuickJS-NG v0.17.0 (reference) (polyfill) | not run | | | |
| Node v24.11.1 (reference) (own Intl) | 126 | 20 of 20 | 20 of 20 | 0.17 (0.13–0.24) |
| Node v24.11.1 --jitless (reference) (own Intl) | 145 | 20 of 20 | 20 of 20 | 0.41 (0.37–0.46) |

| Page | liustack-quickjs | liustack-tifa | modernc-quickjs | go-quickjs-interp | go-quickjs-jit | goant-interp | goant-jit | moejs | quickjs-2026-06-04 | quickjs-master | quickjs-ng | node-jit | node-jitless |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| store-en-US-cart | 25.9 (1.00) | 26 (1.01) |  |  |  |  |  |  | 25 (0.97) |  |  | 6.21 (0.24) | 10.9 (0.42) |
| store-en-US-checkout | 28.3 (1.00) | 28.8 (1.02) |  |  |  |  |  |  | 25.1 (0.89) |  |  | 6.31 (0.22) | 12 (0.43) |
| store-en-US-home | 37.9 (1.00) | 37.5 (0.99) |  |  |  |  |  |  | 40.2 (1.06) |  |  | 6.29 (0.17) | 15.9 (0.42) |
| store-en-US-journal | 7.74 (1.00) | 7.79 (1.01) |  |  |  |  |  |  | 7.48 (0.97) |  |  | 1.35 (0.17) | 3.59 (0.46) |
| store-en-US-post | 106 (1.00) | 103 (0.98) |  |  |  |  |  |  | 74.3 (0.70) |  |  | 18.5 (0.17) | 39.7 (0.37) |
| store-en-US-product | 40.7 (1.00) | 41 (1.01) |  |  |  |  |  |  | 39.4 (0.97) |  |  | 6.65 (0.16) | 16.8 (0.41) |
| store-en-US-products | 69.3 (1.00) | 69 (1.00) |  |  |  |  |  |  | 68.1 (0.98) |  |  | 10.6 (0.15) | 29.5 (0.43) |
| store-zh-CN-cart | 25.9 (1.00) | 25.8 (1.00) |  |  |  |  |  |  | 25.3 (0.98) |  |  | 4.16 (0.16) | 10.8 (0.42) |
| store-zh-CN-checkout | 28.5 (1.00) | 28.6 (1.00) |  |  |  |  |  |  | 25.1 (0.88) |  |  | 4.95 (0.17) | 11.9 (0.42) |
| store-zh-CN-home | 38.9 (1.00) | 37.7 (0.97) |  |  |  |  |  |  | 39.4 (1.01) |  |  | 5.2 (0.13) | 16.1 (0.41) |
| store-zh-CN-journal | 7.96 (1.00) | 7.9 (0.99) |  |  |  |  |  |  | 7.73 (0.97) |  |  | 1.12 (0.14) | 3.3 (0.41) |
| store-zh-CN-post | 83.2 (1.00) | 79.1 (0.95) |  |  |  |  |  |  | 57.9 (0.70) |  |  | 14.4 (0.17) | 30.8 (0.37) |
| store-zh-CN-product | 41.2 (1.00) | 41.5 (1.01) |  |  |  |  |  |  | 40.7 (0.99) |  |  | 5.78 (0.14) | 17 (0.41) |
| store-zh-CN-products | 68.9 (1.00) | 69.5 (1.01) |  |  |  |  |  |  | 67.7 (0.98) |  |  | 10.3 (0.15) | 30 (0.43) |
| wb-order-studio | 11.1 (1.00) | 10.8 (0.97) |  |  |  |  |  |  | 9.37 (0.84) |  |  | 1.99 (0.18) | 4.35 (0.39) |
| wb-order-weekend | 10.6 (1.00) | 10.5 (0.99) |  |  |  |  |  |  | 8.55 (0.81) |  |  | 1.73 (0.16) | 4.23 (0.40) |
| wb-stats-studio | 8.98 (1.00) | 8.63 (0.96) |  |  |  |  |  |  | 8.08 (0.90) |  |  | 1.35 (0.15) | 3.55 (0.39) |
| wb-stats-weekend | 8.53 (1.00) | 8.55 (1.00) |  |  |  |  |  |  | 7.9 (0.93) |  |  | 1.16 (0.14) | 3.62 (0.42) |
| wb-studio | 30.2 (1.00) | 29.8 (0.99) |  |  |  |  |  |  | 30.9 (1.03) |  |  | 5.71 (0.19) | 12.7 (0.42) |
| wb-weekend | 28.8 (1.00) | 28.8 (1.00) |  |  |  |  |  |  | 30.2 (1.05) |  |  | 4.72 (0.16) | 12.3 (0.43) |

### MUI pages on the FormatJS polyfill, every engine

| Engine | Load ms | Pages that render | Same page as liustack-quickjs | Time per render vs liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs | 523 | 3 of 3 | 3 of 3 | 1.00 (1.00–1.00) |
| liustack-tifa | not run: this row has no polyfill | | | |
| modernc-quickjs | not run | | | |
| go-quickjs-interp | not run | | | |
| go-quickjs-jit | not run | | | |
| goant-interp | not run | | | |
| goant-jit | not run | | | |
| moejs | not run | | | |
| C QuickJS 2026-06-04 (reference) | 292 | 3 of 3 | 3 of 3 | 0.85 (0.77–0.93) |
| C QuickJS master 7d1c13c5d8 (reference) | not run | | | |
| QuickJS-NG v0.17.0 (reference) | not run | | | |
| Node v24.11.1 (reference) | 215 | 3 of 3 | 3 of 3 | 0.14 (0.12–0.16) |
| Node v24.11.1 --jitless (reference) | 225 | 3 of 3 | 3 of 3 | 0.41 (0.39–0.44) |

| Page | liustack-quickjs | liustack-tifa | modernc-quickjs | go-quickjs-interp | go-quickjs-jit | goant-interp | goant-jit | moejs | quickjs-2026-06-04 | quickjs-master | quickjs-ng | node-jit | node-jitless |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| home | 143 (1.00) |  |  |  |  |  |  |  | 133 (0.93) |  |  | 21.6 (0.15) | 57.3 (0.40) |
| order | 76.2 (1.00) |  |  |  |  |  |  |  | 58.4 (0.77) |  |  | 12.2 (0.16) | 33.2 (0.44) |
| orders | 302 (1.00) |  |  |  |  |  |  |  | 260 (0.86) |  |  | 36.3 (0.12) | 119 (0.39) |

### MUI pages as deployed: own Intl where the engine has one

| Engine | Load ms | Pages that render | Same page as liustack-quickjs | Time per render vs liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs (own Intl) | 163 | 3 of 3 | 3 of 3 | 1.00 (1.00–1.00) |
| liustack-tifa (own Intl) | 157 | 3 of 3 | 3 of 3 | 0.72 (0.71–0.72) |
| modernc-quickjs (polyfill) | not run | | | |
| go-quickjs-interp (own Intl) | not run | | | |
| go-quickjs-jit (own Intl) | not run | | | |
| goant-interp (polyfill) | not run | | | |
| goant-jit (polyfill) | not run | | | |
| moejs (polyfill) | not run | | | |
| C QuickJS 2026-06-04 (reference) (polyfill) | 292 | 3 of 3 | 0 of 3 | 1.37 (0.85–1.84) |
| C QuickJS master 7d1c13c5d8 (reference) (polyfill) | not run | | | |
| QuickJS-NG v0.17.0 (reference) (polyfill) | not run | | | |
| Node v24.11.1 (reference) (own Intl) | 86 | 3 of 3 | 3 of 3 | 0.16 (0.14–0.17) |
| Node v24.11.1 --jitless (reference) (own Intl) | 110 | 3 of 3 | 3 of 3 | 0.46 (0.46–0.46) |

| Page | liustack-quickjs | liustack-tifa | modernc-quickjs | go-quickjs-interp | go-quickjs-jit | goant-interp | goant-jit | moejs | quickjs-2026-06-04 | quickjs-master | quickjs-ng | node-jit | node-jitless |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| home | 72.4 (1.00) | 52.2 (0.72) |  |  |  |  |  |  | 133 (1.84) |  |  | 12.1 (0.17) | 33.6 (0.46) |
| order | 68.5 (1.00) | 49.2 (0.72) |  |  |  |  |  |  | 58.4 (0.85) |  |  | 11.4 (0.17) | 31.4 (0.46) |
| orders | 157 (1.00) | 111 (0.71) |  |  |  |  |  |  | 260 (1.66) |  |  | 21.5 (0.14) | 72.7 (0.46) |

### liustack-tifa against the same JavaScript on the polyfill

| Bundle | Pages | Same HTML | Same but for U+202F | Other difference | Not compared |
|---|---|---|---|---|---|
| bench | 2 | 2 | 0 | 0 | 0 |
| mui | 3 | 0 | 3 | 0 | 0 |
| shop | 20 | 20 | 0 | 0 | 0 |

### Per bundle, against C QuickJS 2026-06-04

Geometric mean over the pages of a bundle, range in brackets. Below 1 is faster than C. Bench has no Intl, MUI and the shop need it: every row but liustack-tifa and the "own Intl only" row runs them on the polyfill.

| Row | bench | MUI | shop |
|---|---|---|---|
| liustack-quickjs, same JavaScript (polyfill) | 1.49 (1.44–1.55) | 1.18 (1.08–1.30) | 1.44 (1.29–1.59) |
| liustack-quickjs, own Intl only | 1.49 (1.44–1.55) | 0.73 (0.54–1.17) | 1.08 (0.94–1.44) |
| liustack-tifa, as tifa runs it | 1.48 (1.44–1.52) | 0.52 (0.39–0.84) | 1.07 (0.93–1.39) |
| C QuickJS 2026-06-04 | 1.00 (1.00–1.00) | 1.00 (1.00–1.00) | 1.00 (1.00–1.00) |
| Node v24.11.1 | 0.16 (0.12–0.20) | 0.17 (0.14–0.21) | 0.23 (0.18–0.30) |
| Node v24.11.1 --jitless | 0.52 (0.49–0.54) | 0.48 (0.43–0.57) | 0.57 (0.53–0.61) |

### What the polyfill costs liustack-quickjs

- Loading it: 373 ms per VM, before the bundle (287 ms after it, 302 ms without it).
- Memory per VM, polyfill: liustack-quickjs 34.7 MiB RSS, go-quickjs-interp 118.7 MiB RSS (84.0 MiB Go heap)
- Memory per VM, own Intl: liustack-quickjs 12.8 MiB RSS, go-quickjs-interp 48.3 MiB RSS (40.0 MiB Go heap)
- Time inside the polyfill: 219.3 ms of 945.0 ms per render summed over the pages (23%), 1354 calls.
- Time inside its own Intl: 5.4 ms of 707.0 ms per render summed over the pages (1%), 1354 calls.
