# Arquitetura — fluxo de venda e fronteiras de confiança

Diagrama validado (sintaxe conferida no Mermaid). Renderiza direto no GitHub.

O que ele torna visível e o texto não:

- **A Alice é o único serviço público sem autenticação** (em vermelho). Foi por
  isso que o achado A1 importava: ela carregava o segredo que emitia token de
  administrador. Hoje ela só tem leitura pública do catálogo.
- **O token de serviço parte apenas do order-service.** Nenhuma outra seta usa
  `SERVICE_JWT_SECRET`.
- **O webhook do PSP é seta de ida E volta**: o corpo é só gatilho; status e
  valor vêm da reconsulta autenticada.
- **Um banco por serviço, sem seta entre bancos.** Não existe caminho de
  `SELECT` cruzado — quem precisa de dado alheio chama a API.

```mermaid
flowchart TB
    subgraph clientes["Quem usa"]
        C["Cliente<br/>navegador"]
        V["Vendedor<br/>tablet no balcão"]
        A["Dono<br/>admin"]
    end

    SPA["SPA React :5175<br/>loja · /balcao · /admin"]

    C --> SPA
    V --> SPA
    A --> SPA

    subgraph servicos["Serviços Go"]
        AUTH["Auth :8093<br/>papéis · lojas · operadores"]
        CAT["Catalog :8091<br/>produtos · estoque · custo"]
        ORD["Order :8092<br/>pedidos · frete · balcão"]
        PAY["Payment :8090<br/>PSP · livro contábil"]
        ALI["Alice :8094<br/>PÚBLICA, sem auth"]
    end

    SPA -->|JWT| AUTH
    SPA -->|JWT| CAT
    SPA -->|JWT + Idempotency-Key| ORD
    SPA -->|JWT + Idempotency-Key| PAY
    SPA --> ALI

    ORD -->|"token de serviço<br/>SERVICE_JWT_SECRET"| CAT
    ORD -->|token de serviço| AUTH
    ORD -->|token de serviço| PAY
    ALI -->|somente leitura pública| CAT

    PAY -->|payment.confirmed| RP[["Redpanda"]]
    RP -->|consumer| ORD

    PSP{{"Appmax / Stripe<br/>gateway externo"}}
    PAY <-->|"cobra + RECONSULTA<br/>o webhook é só gatilho"| PSP

    subgraph dados["Um banco por serviço · sem acesso cruzado"]
        DA[("auth")]
        DC[("catalog")]
        DO[("order")]
        DP[("payment")]
    end

    AUTH --- DA
    CAT --- DC
    ORD --- DO
    PAY --- DP

    classDef publico fill:#FBE8E6,stroke:#A32218,stroke-width:2px,color:#000
    classDef servico fill:#EDEFF4,stroke:#1B3E8A,color:#000
    classDef banco fill:#E6F4EC,stroke:#15794A,color:#000
    classDef externo fill:#FDF3E0,stroke:#93610B,color:#000

    class ALI publico
    class AUTH,CAT,ORD,PAY servico
    class DA,DC,DO,DP banco
    class PSP,RP externo
```

Ver também [`../CLAUDE.md`](../CLAUDE.md) e
[`security/auditoria-arquitetural-2026-07-18.md`](security/auditoria-arquitetural-2026-07-18.md).
