# Kanvas — Design

**Data:** 2026-08-10
**Status:** Aprovado para planejamento

## 1. Visão geral

Kanvas é uma aplicação de quadro Kanban colaborativo em tempo real, construída como projeto vitrine (portfólio) em Go + React/TypeScript. O objetivo principal não é a funcionalidade em si, mas demonstrar boas práticas de engenharia: arquitetura limpa no backend, testes em profundidade, colaboração ao vivo via WebSockets, e um pipeline de CI/CD com deploy real.

O código vive em um único repositório (monorepo), com `backend/` e `frontend/` na raiz, publicado no GitHub com commits incrementais ao longo do desenvolvimento.

## 2. Escopo (MVP)

**Dentro do escopo:**
- Autenticação de usuários (registro, login, refresh, logout)
- Boards com membros (dono e membros convidados)
- Colunas dentro de um board, reordenáveis
- Cards dentro de uma coluna: título, descrição, responsável (assignee), data de vencimento, reordenáveis e movíveis entre colunas
- Sincronização em tempo real: mudanças feitas por um usuário aparecem instantaneamente para os outros membros conectados ao mesmo board

**Fora do escopo inicial** (candidatos a trabalho futuro, não bloqueiam o MVP):
- Comentários em cards
- Labels/tags coloridas
- Anexos de arquivo
- Presença de cursor ao vivo / indicador de "quem está online"

## 3. Modelo de dados

| Entidade | Campos principais |
|---|---|
| `User` | id, name, email, password_hash, created_at, updated_at |
| `Board` | id, name, owner_id, created_at, updated_at |
| `BoardMember` | board_id, user_id, role (`owner` \| `member`), created_at |
| `Column` | id, board_id, title, position, created_at, updated_at |
| `Card` | id, column_id, title, description, position, assignee_id (nullable), due_date (nullable), created_at, updated_at |

`role` em `BoardMember` controla permissões: apenas `owner` pode convidar/remover membros ou apagar o board; ambos os papéis podem criar/editar/mover colunas e cards.

## 4. Arquitetura do backend

Clean/Hexagonal Architecture, organizada por domínio dentro de cada camada.

```
backend/
  cmd/api/            # main.go — monta dependências, sobe o servidor HTTP
  internal/
    auth/              # domain, service, postgres repo, http handlers
    board/             # domain, service, postgres repo, http handlers
    card/              # domain, service, postgres repo, http handlers
    realtime/          # WebSocket hub: registro de conexões por board, broadcast de eventos
    platform/          # infra compartilhada: db (pgx pool), jwt, middleware, config, logger
  db/
    migrations/        # arquivos .sql versionados (golang-migrate)
    queries/           # .sql fonte para o sqlc gerar código Go type-safe
```

Cada domínio (`auth`, `board`, `card`) segue o mesmo padrão:
- `domain.go` — entidades e regras de negócio puras, sem dependência de infraestrutura
- `service.go` — casos de uso, depende de uma **interface** de repositório (inversão de dependência)
- `repository_postgres.go` — implementa a interface usando código gerado pelo sqlc
- `handler.go` — handlers HTTP (chi), traduzem request/response ↔ chamadas ao service

Essa separação permite testar `service.go` com um repositório fake, sem precisar de banco de dados nos testes unitários.

**Stack técnica do backend:**
- Router HTTP: [chi](https://github.com/go-chi/chi)
- Acesso a dados: [sqlc](https://sqlc.dev/) + [pgx](https://github.com/jackc/pgx) (SQL puro, sem ORM)
- Migrações: [golang-migrate](https://github.com/golang-migrate/migrate)
- WebSocket: [nhooyr.io/websocket](https://github.com/coder/websocket)
- Banco de dados: PostgreSQL

## 5. API

**REST (exemplos representativos, não exaustivo):**
- `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`
- `GET/POST /boards`, `GET/PATCH/DELETE /boards/{id}`, `POST /boards/{id}/members`, `DELETE /boards/{id}/members/{userId}`
- `POST/PATCH/DELETE /boards/{id}/columns`, `PATCH /boards/{id}/columns/reorder`
- `POST/PATCH/DELETE /cards/{id}`, `PATCH /cards/{id}/move` (coluna + posição destino)

**WebSocket:** `GET /boards/{id}/ws`. O cliente se autentica no handshake (JWT), entra no "hub" daquele board, e recebe eventos sempre que o backend processa uma mutação via REST: `card.created`, `card.updated`, `card.moved`, `card.deleted`, `column.created`, `column.updated`, `column.deleted`, `column.reordered`.

## 6. Autenticação

JWT com access + refresh token:
1. Login retorna um `access_token` de vida curta (~15 min) e um `refresh_token` de vida longa (~7 dias) em cookie `httpOnly`.
2. O `access_token` vai no header `Authorization: Bearer` de cada request e fica só em memória no frontend (nunca em `localStorage`), reduzindo exposição a XSS.
3. Quando o `access_token` expira, o client chama `/auth/refresh` (lendo o cookie httpOnly) para obter um novo, de forma transparente via interceptor HTTP.
4. Logout revoga o refresh token no backend.

## 7. Frontend

React + TypeScript, dentro de `frontend/` no monorepo.

```
frontend/src/
  api/                # client HTTP + hooks do TanStack Query por domínio
  features/
    auth/              # login/registro, store Zustand de sessão
    boards/            # lista de boards, criação, convite de membros
    board/             # tela do board: colunas, cards, drag-and-drop, hook de WebSocket
  components/          # componentes de UI reutilizáveis
  lib/                 # utilitários
```

- **TanStack Query** gerencia cache/sincronização com a API REST; invalidação de cache disparada pelos eventos recebidos via WebSocket.
- **Zustand** guarda estado de sessão (usuário, token em memória) e estado leve de UI.
- **Drag-and-drop** de cards entre colunas via `@dnd-kit`.

## 8. Testes

- **Unitários (Go):** regras de negócio em cada `service.go`, com repositórios fake — cobre validações, permissões por role, lógica de reordenação.
- **Integração (Go):** endpoints HTTP contra Postgres real via `testcontainers-go` — cobre auth, criação/edição de boards, cards e checagem de permissões.
- **E2E (Playwright):** fluxo completo pelo navegador — login, criar board, criar/mover cards, e verificação de que uma segunda aba conectada via WebSocket recebe a atualização em tempo real.
- **Frontend (Vitest + Testing Library):** componentes e hooks com lógica sensível (ex: hook de WebSocket, formulários).

## 9. DevOps

**Docker:**
- `Dockerfile` multi-stage para o backend (build Go estático, imagem final mínima).
- `Dockerfile` para o frontend (build Vite, servido via Nginx ou pelo próprio backend).
- `docker-compose.yml` na raiz: sobe backend + frontend + Postgres com um único comando, para desenvolvimento local.

**CI (GitHub Actions):**
- Workflow em push/PR: lint (`golangci-lint`, `eslint`), testes Go (unit + integração com Postgres de serviço), testes frontend (Vitest), build de ambos.
- Job (pode ser condicional) para e2e com Playwright contra os containers via `docker compose`.

**Deploy (Fly.io):**
- Backend + Postgres gerenciado no Fly.io.
- Frontend servido como build estático (via Nginx ou pelo backend, a definir na implementação).
- Deploy automático (`flyctl deploy`) ao mergear na branch principal, após os testes de CI passarem.

**Versionamento:** o repositório será criado no GitHub e receberá commits incrementais ao longo de todo o desenvolvimento (não apenas no final).

## 10. Erros e casos de borda a considerar na implementação

- Conflito de posição ao mover cards/colunas simultaneamente (dois usuários movendo o mesmo card ao mesmo tempo) — resolver com a última escrita vencendo (last-write-wins) na v1; documentar como limitação conhecida.
- Reconexão do WebSocket após queda de rede — client deve tentar reconectar e ressincronizar o estado do board via REST ao reconectar.
- Usuário removido de um board enquanto conectado ao WebSocket daquele board — servidor deve encerrar a conexão.
- Token de acesso expirado durante uma operação — interceptor no frontend deve renovar e repetir a requisição original de forma transparente.
