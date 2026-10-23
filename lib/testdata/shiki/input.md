```bash
curl https://api.shanhai.example/v1/products?limit=2 \
  -H "Authorization: Bearer shk_live_8f2c1e" \
  -H "Accept: application/json"
```

```json
{
  "data": [
    {
      "id": "pour-over-kettle",
      "title": "Morning Pour-Over Kettle",
      "variants": [
        { "sku": "PK-MIST", "color": "mist", "stock": 23 },
        { "sku": "PK-CHAR", "color": "charcoal", "stock": 0 }
      ],
      "updatedAt": "2026-10-08T09:12:44Z"
    }
  ],
  "next": "eyJvZmZzZXQiOjJ9"
}
```

```go
type Variant struct {
	SKU   string `json:"sku"`
	Color string `json:"color"`
	Stock int    `json:"stock"`
}

type page struct {
	Data []struct {
		ID       string    `json:"id"`
		Variants []Variant `json:"variants"`
	} `json:"data"`
	Next string `json:"next"`
}

func pullAll(ctx context.Context, token string) (map[string]Variant, error) {
	out := map[string]Variant{}
	cursor := ""
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", base+"/v1/products?limit=100&cursor="+cursor, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		var p page
		err = json.NewDecoder(res.Body).Decode(&p)
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, prod := range p.Data {
			for _, v := range prod.Variants {
				out[v.SKU] = v
			}
		}
		if p.Next == "" {
			return out, nil
		}
		cursor = p.Next
	}
}
```

```bash
curl -X POST https://api.shanhai.example/v1/inventory/adjust \
  -H "Authorization: Bearer shk_live_8f2c1e" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: pos-2026-10-08-0042" \
  -d '{"sku": "PK-MIST", "delta": -1, "reason": "store_sale"}'
```

```json
{
  "event": "inventory.updated",
  "sku": "PK-MIST",
  "stock": 21,
  "delta": -2,
  "source": "web_order",
  "occurredAt": "2026-10-08T10:03:12Z"
}
```

```ts
import { createHmac, timingSafeEqual } from 'node:crypto'

export function verify(body: string, header: string, secret: string): boolean {
  const expected = createHmac('sha256', secret).update(body).digest('hex')
  const given = header.replace(/^sha256=/, '')
  if (given.length !== expected.length) return false
  return timingSafeEqual(Buffer.from(given), Buffer.from(expected))
}
```

```yaml
# Configuration of the till
sync:
  base: https://api.shanhai.example
  token_env: SHANHAI_TOKEN
  full_pull: "0 3 * * *"   # every night at 3 am
  webhook:
    listen: ":8443"
    secret_env: SHANHAI_WEBHOOK_SECRET
```

```css
:root {
  --background: #fbf8f4;
  --foreground: #3a2a2c;
  --primary: #8c3b4a;
  --primary-foreground: #fbf8f4;
  --muted-foreground: #8f7f80;
  --border: #eadfd9;
  --radius: 1rem;
}
```

```css
:root {
  --primary: oklch(0.45 0.12 10);
  --ring: oklch(0.7 0.08 10);
}
```

```css
body[data-page="product"] {
  --radius: 0;
}

body[data-page="product"] .price {
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.01em;
}
```

```css
@font-face {
  font-family: "Shop Serif";
  src: url("/files/shop-serif.woff2") format("woff2");
  font-display: swap;
}

:root {
  --font-sans: "Shop Serif", "Noto Serif", serif;
}
```

```html
<!-- Custom CSS goes in after the theme's stylesheet -->
<link rel="stylesheet" href="/assets/theme.css">
<style id="custom-css">/* what you wrote */</style>
```

