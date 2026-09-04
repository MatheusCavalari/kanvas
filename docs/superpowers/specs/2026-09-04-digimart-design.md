# DigiMart — Design Spec

**Data:** 2026-09-04
**Autor:** Matheus Cavalari
**Status:** Aprovado

## Visão Geral

DigiMart é um marketplace de produtos digitais (cursos, templates, e-books, assets de design) com arquitetura de microservices event-driven. O projeto serve como peça de portfólio profissional, complementando o Kanvas (realtime/WebSocket) com um case de microservices, Kafka e comunicação assíncrona.

**Público-alvo do portfólio:** Recrutadores e tech leads avaliando competência full-stack com foco em sistemas distribuídos.

## Arquitetura Geral

### Serviços

| Serviço | Stack | Banco | Responsabilidade |
|---------|-------|-------|-----------------|
| **Identity** | .NET 9 Web API | PostgreSQL | Registro, login, JWT, perfis (comprador/vendedor) |
| **Catalog** | .NET 9 Web API | PostgreSQL + Redis (cache) | CRUD de produtos, categorias, busca, upload de arquivos |
| **Order** | .NET 9 Web API | PostgreSQL | Carrinho, checkout, histórico de pedidos, simulação de pagamento |
| **Notification** | .NET 9 Worker Service | — (stateless) | Consome eventos Kafka, envia emails (simulado via log/template) |

### Comunicação

- **Síncrona (HTTP):** Frontend → cada serviço diretamente (sem API Gateway)
- **Assíncrona (Kafka):** Eventos entre serviços no fluxo de compra
- **Sem chamadas HTTP entre serviços** — toda comunicação inter-serviço é via Kafka

### Infra local

Tudo sobe com `docker-compose`: 4 serviços .NET + PostgreSQL (1 instância, databases separados) + Kafka + Zookeeper + Redis.

## Modelo de Dados

### Identity Service

```
Users
├── Id (UUID)
├── Email (unique)
├── PasswordHash
├── FullName
├── Role (Buyer | Seller | Admin)
├── CreatedAt
└── UpdatedAt
```

### Catalog Service

```
Categories
├── Id (UUID)
├── Name
├── Slug (unique)
└── Description

Products
├── Id (UUID)
├── SellerId (UUID — referência ao Identity, sem FK cross-service)
├── CategoryId (FK → Categories)
├── Title
├── Slug (unique)
├── Description
├── Price (decimal)
├── FileUrl (link para o arquivo digital)
├── ThumbnailUrl
├── Status (Draft | Active | Inactive)
├── CreatedAt
└── UpdatedAt

ProductAccess
├── Id (UUID)
├── ProductId (FK → Products)
├── BuyerId (UUID)
├── OrderId (UUID)
├── GrantedAt
└── ExpiresAt (nullable — null = acesso permanente)
```

### Order Service

```
Orders
├── Id (UUID)
├── BuyerId (UUID)
├── Status (Pending | Confirmed | Cancelled)
├── TotalAmount (decimal)
├── CreatedAt
└── UpdatedAt

OrderItems
├── Id (UUID)
├── OrderId (FK → Orders)
├── ProductId (UUID)
├── ProductTitle (snapshot — não depende do Catalog)
├── UnitPrice (decimal, snapshot)
└── Quantity (sempre 1 para digital, mas mantém flexibilidade)
```

### Decisões de modelagem

- **Sem foreign keys cross-service** — cada serviço é dono dos seus dados.
- **Order grava snapshots** do nome e preço do produto no momento da compra.
- **ProductAccess** fica no Catalog porque é ele quem controla o download. O evento `PaymentConfirmed` é o trigger para criar esse registro.
- **SellerId/BuyerId** são UUIDs guardados sem FK — a verdade sobre o usuário vive no Identity, os outros serviços só referenciam pelo ID.

## Fluxo de Eventos

### Fluxo principal — Compra de produto digital

```
Buyer (Frontend)
  │
  ├─ 1. POST /api/orders  (Order Service)
  │     → Cria Order com status Pending
  │     → Publica evento "OrderPlaced" no Kafka
  │
  ├─ 2. POST /api/orders/{id}/pay  (Order Service)
  │     → Simula pagamento (aceita sempre, sem gateway real)
  │     → Atualiza status para Confirmed
  │     → Publica evento "PaymentConfirmed" no Kafka
  │
  ├─ 3. Catalog Service consome "PaymentConfirmed"
  │     → Cria registro em ProductAccess
  │     → Publica evento "AccessGranted" no Kafka
  │
  └─ 4. Notification Service consome "OrderPlaced", "PaymentConfirmed", "AccessGranted"
        → Loga emails simulados (confirmação de pedido, link de download)
```

### Tópicos Kafka

| Tópico | Producer | Consumer | Payload principal |
|--------|----------|----------|-------------------|
| `order.placed` | Order | Notification | orderId, buyerId, items, total |
| `payment.confirmed` | Order | Catalog, Notification | orderId, buyerId, items |
| `access.granted` | Catalog | Notification | buyerId, productId, downloadUrl |

## APIs

### Identity Service — `/api/auth`

| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/register` | Cria conta (buyer ou seller) |
| POST | `/login` | Retorna JWT access token + refresh token |
| POST | `/refresh` | Renova access token |
| GET | `/me` | Perfil do usuário logado |

### Catalog Service — `/api/catalog`

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/products` | Lista com filtros (categoria, busca, paginação) |
| GET | `/products/{slug}` | Detalhe do produto |
| POST | `/products` | Cria produto (seller only) |
| PUT | `/products/{id}` | Atualiza produto (seller owner only) |
| DELETE | `/products/{id}` | Remove produto (seller owner only) |
| GET | `/products/my` | Produtos do seller logado |
| GET | `/categories` | Lista categorias |
| GET | `/purchases` | Produtos comprados pelo buyer logado |
| GET | `/purchases/{productId}/download` | Download do arquivo (verifica ProductAccess) |

### Order Service — `/api/orders`

| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/` | Cria pedido (recebe lista de productIds) |
| POST | `/{id}/pay` | Confirma pagamento (simulado) |
| GET | `/` | Histórico de pedidos do buyer |
| GET | `/{id}` | Detalhe do pedido |

### Autenticação cross-service

O Identity emite JWTs. Os outros serviços validam o token localmente (shared signing key via config). Sem chamadas HTTP entre serviços.

## Frontend

### Stack

React 18 + TypeScript + Vite + Tailwind CSS + React Router v6

### Páginas

| Página | Rota | Descrição |
|--------|------|-----------|
| **Home** | `/` | Landing com produtos em destaque e categorias |
| **Catálogo** | `/products` | Lista com filtros por categoria, busca e paginação |
| **Detalhe do produto** | `/products/:slug` | Imagem, descrição, preço, botão comprar |
| **Login** | `/login` | Email + senha |
| **Registro** | `/register` | Formulário com escolha de role (buyer/seller) |
| **Checkout** | `/checkout` | Resumo do carrinho, botão "Pagar" (simulado) |
| **Meus pedidos** | `/orders` | Histórico de compras do buyer |
| **Minha biblioteca** | `/library` | Produtos comprados com botão de download |
| **Painel do seller** | `/seller/products` | CRUD dos próprios produtos |
| **Criar/Editar produto** | `/seller/products/new` e `/:id/edit` | Formulário com upload de thumbnail e arquivo |

### Organização de pastas

```
src/
├── api/          # Clients HTTP por serviço (identity, catalog, order)
├── components/   # Componentes reutilizáveis (Button, Input, Card, Modal, Layout)
├── contexts/     # AuthContext (JWT, refresh, user info)
├── hooks/        # useAuth, useProducts, useOrders
├── pages/        # Uma pasta por página
├── routes/       # Definição de rotas, guards (PrivateRoute, SellerRoute)
├── types/        # Tipos TypeScript compartilhados
└── utils/        # Formatadores, helpers
```

### Decisões de UX

- **Carrinho em memória** (state local) — simplifica e faz sentido para produtos digitais.
- **JWT no localStorage** com refresh token via httpOnly cookie.
- **SPA pura** (sem SSR) — o foco é o backend/microservices.
- **Responsivo** com Tailwind, mas mobile não é prioridade.

## Testes

| Camada | Ferramenta | O que testa |
|--------|-----------|-------------|
| **Unit (backend)** | xUnit + Moq | Domain logic, handlers, validações — cada serviço isolado |
| **Integration (backend)** | xUnit + Testcontainers | Repositories contra PostgreSQL real, producers/consumers Kafka |
| **Unit (frontend)** | Vitest + Testing Library | Componentes, hooks, lógica de estado |
| **E2E** | Playwright | Fluxo completo: registro → login → browse → compra → download |

**Testcontainers** sobe containers efêmeros de Postgres e Kafka durante os testes de integração — sem mocks de infra.

## CI/CD (GitHub Actions)

| Workflow | Trigger | Steps |
|----------|---------|-------|
| `backend-ci.yml` | Push/PR em `backend/**` | Restore → Build → Unit tests → Integration tests → Lint |
| `frontend-ci.yml` | Push/PR em `frontend/**` | Install → Lint → Type check → Unit tests → Build |
| `e2e-ci.yml` | Push/PR em `main` | Docker-compose up → Playwright → Teardown |

## Deploy

- **Backend:** Cada serviço como container Docker no Render (free tier)
- **Frontend:** Build estático no Render (Static Site)
- **PostgreSQL e Redis:** Managed no Render
- **Kafka:** Upstash Kafka (free tier, serverless)
- **Docker Compose** para dev local

## Estrutura do Repositório

```
digimart/
├── backend/
│   ├── src/
│   │   ├── DigiMart.Identity/
│   │   ├── DigiMart.Catalog/
│   │   ├── DigiMart.Order/
│   │   ├── DigiMart.Notification/
│   │   └── DigiMart.Shared/
│   ├── tests/
│   │   ├── DigiMart.Identity.Tests/
│   │   ├── DigiMart.Catalog.Tests/
│   │   ├── DigiMart.Order.Tests/
│   │   └── DigiMart.Integration.Tests/
│   └── DigiMart.sln
├── frontend/
│   ├── src/
│   ├── package.json
│   └── vite.config.ts
├── e2e/
│   └── tests/
├── docker-compose.yml
├── .github/workflows/
├── README.md
└── LICENSE
```
