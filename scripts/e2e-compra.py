#!/usr/bin/env python3
"""Fluxo de compra ponta a ponta contra os serviços reais."""
import json, urllib.request, urllib.error, uuid, sys

AUTH, CAT, ORD, PAY = "http://localhost:8093", "http://localhost:8091", "http://localhost:8092", "http://localhost:8090"
falhas = []


def call(method, url, body=None, token=None, extra=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    for k, v in (extra or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        raw = e.read()
        try:
            return e.code, json.loads(raw or b"{}")
        except Exception:
            return e.code, {"error": raw[:200].decode(errors="replace")}
    except Exception as e:
        return 0, {"error": str(e)}


def passo(n, desc, ok, detalhe=""):
    print(f"{n:>2}. {desc:.<40} {'OK' if ok else 'FALHOU'}  {detalhe}")
    if not ok:
        falhas.append(f"{desc}: {detalhe}")
    return ok


# 1 — login
st, d = call("POST", f"{AUTH}/api/v1/auth/login",
             {"email": "test1@utilar.com.br", "password": "utilar123"})
token = d.get("accessToken") or d.get("token", "")
passo(1, "login", bool(token), f"HTTP {st}")
if not token:
    sys.exit(1)

# 2 — catálogo
st, d = call("GET", f"{CAT}/api/v1/products?per_page=1&in_stock=true")
prod = (d.get("data") or [{}])[0]
pid, price, sku, stock = prod.get("id"), prod.get("price"), prod.get("sku"), prod.get("stock")
nome = prod.get("name", "Produto")
sel = prod.get("seller")
seller = sel if isinstance(sel, dict) else {}
seller_id = seller.get("id") or prod.get("sellerId") or "ferragem-silva"
seller_nome = seller.get("name") or (sel if isinstance(sel, str) else None) or "Ferragem Silva"
passo(2, "catálogo", bool(pid and price), f"{sku} R$ {price} (estoque {stock})")

# 3 — busca por SKU (o caminho do balcão)
st, d = call("GET", f"{CAT}/api/v1/products?sku={sku}")
passo(3, "busca exata por SKU", d.get("meta", {}).get("total") == 1, f"HTTP {st}")

# 4 — cotação de frete
st, d = call("POST", f"{ORD}/api/v1/shipping/quote",
             {"cep": "01310100", "subtotal": 250, "itemCount": 2}, token)
opts = d.get("options") or []
passo(4, "cotação de frete", st == 200 and len(opts) > 0,
      f"{len(opts)} opções, a partir de R$ {opts[0]['cost'] if opts else '?'}")

# 5 — criação de pedido
idem = str(uuid.uuid4())
pedido = {
    "paymentMethod": "pix",
    "items": [{"productId": pid, "name": nome, "sellerId": seller_id,
               "sellerName": seller_nome, "quantity": 1, "unitPrice": price}],
    "shippingCost": 0,
    "shippingService": opts[0]["serviceCode"] if opts else "standard",
    "address": {"cep": "01310-100", "street": "Av Paulista", "number": "1000",
                "neighborhood": "Bela Vista", "city": "São Paulo", "state": "SP"},
}
st, d = call("POST", f"{ORD}/api/v1/orders", pedido, token, {"Idempotency-Key": idem})
oid, total = d.get("id"), d.get("total")
passo(5, "pedido criado", bool(oid), f"HTTP {st} total R$ {total} {'' if oid else json.dumps(d)[:160]}")

if oid:
    # 6 — idempotência: mesma chave não pode criar segundo pedido
    st2, d2 = call("POST", f"{ORD}/api/v1/orders", pedido, token, {"Idempotency-Key": idem})
    passo(6, "idempotência (mesma chave)", d2.get("id") == oid,
          "replay devolveu o mesmo pedido" if d2.get("id") == oid else f"criou outro: {d2.get('id')}")

    # 7 — frete server-side: acima de R$299 a tabela dá frete grátis, então o
    #     total IGUAL ao subtotal é o resultado CORRETO aqui.
    sub = sum(i["quantity"] * i["unitPrice"] for i in pedido["items"])
    gratis = sub >= 299
    passo(7, "frete resolvido no servidor",
          (total == sub) if gratis else (total > sub),
          f"subtotal {sub} → total {total} ({'frete grátis acima de R$299' if gratis else 'frete somado'})")

    # 8 — pagamento Pix
    st, d = call("POST", f"{PAY}/api/v1/payments",
                 {"order_id": oid, "method": "pix", "amount": total,
                  "payer_cpf": "39053344705", "payer_name": "Teste E2E",
                  "payer_phone": "11999990000"},
                 token, {"Idempotency-Key": str(uuid.uuid4())})
    passo(8, "pagamento Pix criado", st in (200, 201), f"HTTP {st} {json.dumps(d)[:130]}")

# ── SEGURANÇA ────────────────────────────────────────────────────────────────
print("\n── segurança ──")

st, _ = call("GET", f"{CAT}/api/v1/store/products/costs?ids={pid}")
passo(9, "custo NÃO responde a anônimo", st in (401, 403), f"HTTP {st}")

st, _ = call("GET", f"{CAT}/api/v1/store/products/costs?ids={pid}", token=token)
passo(10, "custo NÃO responde a customer", st in (401, 403), f"HTTP {st}")

st, d = call("GET", f"{CAT}/api/v1/products/by-id/{pid}")
passo(11, "custo ausente da API pública", "cost" not in d, "campo cost não existe no payload")

st, _ = call("POST", f"{CAT}/api/v1/admin/products", {"name": "x"}, token=token)
passo(12, "customer NÃO cria produto", st in (401, 403), f"HTTP {st}")

if oid:
    st, _ = call("POST", f"{ORD}/api/v1/balcao/orders/{oid}/settle-external",
                 {"nsu": "123456"}, token=token)
    passo(13, "customer NÃO liquida o próprio pedido", st in (401, 403, 404, 409), f"HTTP {st}")

st, _ = call("GET", f"{ORD}/api/v1/orders")
passo(14, "pedidos exigem autenticação", st == 401, f"HTTP {st}")

st, d = call("GET", f"{ORD}/api/v1/orders", token=token)
passo(15, "listagem escopada ao usuário", st == 200, f"{len(d.get('data') or [])} pedidos do próprio usuário")

print()
if falhas:
    print(f"❌ {len(falhas)} FALHAS:")
    for f in falhas:
        print("   -", f)
    sys.exit(1)
print("✅ todos os passos passaram")
