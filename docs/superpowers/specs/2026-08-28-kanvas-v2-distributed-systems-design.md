# Kanvas v2 — Distributed Systems Showcase

**Data:** 2026-08-28
**Status:** Em design

## 1. Visao geral

O upgrade transforma o kanvas de um CRUD com WebSocket num showcase de patterns de engenharia avancada. Sao 12 novas funcionalidades organizadas em 3 camadas: produto (features visiveis ao usuario), infraestrutura (patterns de engenharia), e operacoes (production-readiness).

O objetivo e portfolio tecnico: cada feature demonstra um pattern diferente usado em sistemas distribuidos reais — event sourcing (audit log), caching com invalidacao event-driven, rate limiting, webhook delivery com retry, background job processing, observabilidade, e graceful shutdown.

Todas as features rodam localmente via `docker-compose`. Deploy em producao nao e requisito.

## 2. Stack tecnica — adicoes

| Componente | Tecnologia |
|---|---|
| Cache + Rate limit + Job queue | Redis 7 (`go-redis/redis/v9`) |
| Structured logging | `log/slog` (stdlib Go 1.23) |
| Metrics | `prometheus/client_golang` |
| Full-text search | PostgreSQL `tsvector` + GIN index (sem servico externo) |

Redis e o unico servico novo no docker-compose. O resto usa PostgreSQL e o processo Go existentes.

## 3. Modelo de dados — novas tabelas

### 3.1. `comments`

```sql
CREATE TABLE comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    card_id UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES users(id),
    body TEXT NOT NULL,
    search_vector tsvector GENERATED ALWAYS AS (to_tsvector('english', body)) STORED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_comments_card_id ON comments(card_id);
CREATE INDEX idx_comments_search ON comments USING GIN(search_vector);
```

### 3.2. `labels`

```sql
CREATE TABLE labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    color VARCHAR(7) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_labels_board_name ON labels(board_id, name);
```

### 3.3. `card_labels`

```sql
CREATE TABLE card_labels (
    card_id UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (card_id, label_id)
);
```

### 3.4. `activity_log`

```sql
CREATE TABLE activity_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    actor_id UUID NOT NULL REFERENCES users(id),
    action VARCHAR(50) NOT NULL,
    entity_type VARCHAR(30) NOT NULL,
    entity_id UUID NOT NULL,
    snapshot_before JSONB,
    snapshot_after JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_activity_log_board_created ON activity_log(board_id, created_at DESC);
```

Tabela append-only. Nenhum UPDATE ou DELETE e permitido pela aplicacao.

### 3.5. `webhooks`

```sql
CREATE TABLE webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES users(id),
    url TEXT NOT NULL,
    secret VARCHAR(64) NOT NULL,
    events TEXT[] NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_webhooks_board ON webhooks(board_id);
```

### 3.6. `webhook_deliveries`

```sql
CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    response_code INT,
    last_attempt_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_webhook_deliveries_webhook ON webhook_deliveries(webhook_id, created_at DESC);
```

`status` e um de: `pending`, `success`, `failed`, `exhausted`.

### 3.7. Alteracoes em tabelas existentes

```sql
ALTER TABLE cards ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', title || ' ' || coalesce(description, ''))) STORED;
CREATE INDEX idx_cards_search ON cards USING GIN(search_vector);
```

## 4. Features de produto

### 4.1. Card Comments

**Dominio:** `internal/comment/`

**Arquitetura:** segue o mesmo pattern dos dominios existentes — `domain.go`, `repository.go` (interface), `repository_postgres.go` (sqlc), `service.go`, `handler.go`.

**Entidade:**
```go
type Comment struct {
    ID        uuid.UUID
    CardID    uuid.UUID
    AuthorID  uuid.UUID
    Body      string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**Repository interface:**
```go
type Repository interface {
    Create(ctx context.Context, c Comment) (Comment, error)
    GetByID(ctx context.Context, id uuid.UUID) (Comment, error)
    Update(ctx context.Context, id uuid.UUID, body string) (Comment, error)
    Delete(ctx context.Context, id uuid.UUID) error
    ListByCard(ctx context.Context, cardID uuid.UUID, cursor time.Time, limit int) ([]Comment, error)
}
```

**Dependencias:** `BoardAuthorizer` (para verificar membership via card -> column -> board) e `EventPublisher`.

**API:**

| Metodo | Rota | Descricao |
|---|---|---|
| POST | `/cards/{cardID}/comments` | Cria comentario |
| GET | `/cards/{cardID}/comments?cursor=&limit=20` | Lista com cursor pagination |
| PATCH | `/comments/{commentID}` | Edita (so o autor) |
| DELETE | `/comments/{commentID}` | Deleta (autor ou owner do board) |

**Cursor pagination:** o cursor e o `created_at` do ultimo item encodado em base64. A query usa `WHERE created_at > $cursor ORDER BY created_at ASC LIMIT $limit+1`. O +1 detecta se ha proxima pagina. Response inclui `next_cursor` (ou null se nao ha mais).

**Eventos realtime:** `comment.created`, `comment.updated`, `comment.deleted`.

**Frontend:** secao de comentarios no `CardDetailModal` — lista de comments com nome do autor, timestamp relativo, input de texto, e updates em tempo real via WebSocket hook existente.

**Permissoes:**
- Criar/listar: qualquer membro do board
- Editar: apenas o autor do comentario
- Deletar: autor do comentario OU owner do board

### 4.2. Labels/Tags Coloridas

**Dominio:** `internal/label/`

**Entidade:**
```go
type Label struct {
    ID        uuid.UUID
    BoardID   uuid.UUID
    Name      string
    Color     string // hex, ex: "#FF5733"
    CreatedAt time.Time
}
```

**Repository interface:**
```go
type Repository interface {
    Create(ctx context.Context, l Label) (Label, error)
    GetByID(ctx context.Context, id uuid.UUID) (Label, error)
    Update(ctx context.Context, id uuid.UUID, name, color string) (Label, error)
    Delete(ctx context.Context, id uuid.UUID) error
    ListByBoard(ctx context.Context, boardID uuid.UUID) ([]Label, error)
    AttachToCard(ctx context.Context, cardID, labelID uuid.UUID) error
    DetachFromCard(ctx context.Context, cardID, labelID uuid.UUID) error
    ListCardLabels(ctx context.Context, cardID uuid.UUID) ([]Label, error)
}
```

Nota: `ListByBoard` e `ListByCard` tem nomes diferentes — nao ha colisao.

**API:**

| Metodo | Rota | Descricao |
|---|---|---|
| POST | `/boards/{boardID}/labels` | Cria label |
| GET | `/boards/{boardID}/labels` | Lista labels do board |
| PATCH | `/boards/{boardID}/labels/{labelID}` | Edita label |
| DELETE | `/boards/{boardID}/labels/{labelID}` | Deleta label |
| POST | `/cards/{cardID}/labels` | Attach label ao card (`{"label_id":"..."}`) |
| DELETE | `/cards/{cardID}/labels/{labelID}` | Detach label do card |

**Eventos realtime:** `label.created`, `label.updated`, `label.deleted`, `card.label_added`, `card.label_removed`.

**Frontend:**
- Chips coloridos nos cards do kanban board
- Painel de gestao de labels acessivel pelo header do board
- Picker de labels no `CardDetailModal`
- Dropdown de filtro por label no header do board

**Filtragem:** `GET /boards/{boardID}/columns?label_id=...` filtra cards. O service faz a filtragem no banco (JOIN com `card_labels`). No frontend, o filtro atualiza a query key do TanStack Query.

**Permissoes:**
- CRUD de labels: qualquer membro do board
- Attach/detach: qualquer membro do board

**Validacao:**
- `name`: 1-50 caracteres, unico por board
- `color`: regex `^#[0-9a-fA-F]{6}$`

### 4.3. Full-text Search

**Dominio:** `internal/search/` — handler + queries diretas, sem service layer (read-only, sem logica de dominio).

**Busca usa PostgreSQL `tsvector` + `plainto_tsquery`:**
```sql
SELECT id, title, column_id, ts_rank(search_vector, query) AS rank
FROM cards, plainto_tsquery('english', $1) query
WHERE board_id_from_column = $2 AND search_vector @@ query
ORDER BY rank DESC
LIMIT 20;
```

Para cards, a busca precisa de um JOIN com `columns` para filtrar por board. Para comments, JOIN com `cards` e `columns`.

**API:** `GET /boards/{boardID}/search?q=texto&type=cards|comments|all`

- `q`: texto de busca (minimo 2 caracteres)
- `type`: `cards` (default), `comments`, ou `all`
- Retorna no maximo 20 resultados por tipo

**Response:**
```json
{
  "cards": [
    {"id": "...", "title": "...", "column_id": "...", "rank": 0.85}
  ],
  "comments": [
    {"id": "...", "card_id": "...", "body_excerpt": "...", "rank": 0.72}
  ]
}
```

`body_excerpt` usa `ts_headline` do PostgreSQL para highlight com `<mark>` tags.

**Frontend:** search bar no header do board. Ao digitar (debounce 300ms), dropdown com resultados agrupados por tipo. Clicar num resultado navega ate o card.

### 4.4. Presence ("quem esta online")

**Implementacao no Hub** — nao e um dominio separado, e uma extensao do `internal/realtime/`:

O `Hub` ganha um campo `presence`:
```go
type PresenceInfo struct {
    UserID        uuid.UUID
    Name          string
    LastHeartbeat time.Time
}

// No Hub:
presence map[uuid.UUID]map[uuid.UUID]PresenceInfo // boardID -> userID -> info
```

**Mecanica:**
1. Ao conectar ao WebSocket: o handler registra a presenca e broadcast `presence.joined` com `{user_id, name}`
2. Client envia mensagem `{"type":"ping"}` a cada 30 segundos
3. Server atualiza `LastHeartbeat` no mapa
4. Goroutine no Hub checa a cada 15s: se `LastHeartbeat` > 60s atras, remove e broadcast `presence.left`
5. Ao desconectar (WebSocket close): remove presenca e broadcast `presence.left`

**API REST:** `GET /boards/{boardID}/presence` — retorna snapshot da presenca atual (para quem acabou de conectar e precisa do estado inicial antes dos eventos WebSocket).

**Frontend:** barra de avatares no header do board com indicador verde. Tooltip com nome do usuario ao hover.

## 5. Features de infraestrutura

### 5.1. Redis Caching Layer

**Novo servico no docker-compose:** Redis 7 Alpine.

**Pacote:** `internal/platform/cache/`

**Interface:**
```go
type Cache interface {
    Get(ctx context.Context, key string, dest interface{}) error
    Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
    Delete(ctx context.Context, keys ...string) error
}
```

Implementacao: `RedisCache` usando `go-redis/redis/v9` com JSON serialization.

**O que e cacheado:**

| Key pattern | Conteudo | TTL |
|---|---|---|
| `board:{boardID}:data` | Board completo (colunas + cards + labels) | 5 min |
| `user:{userID}:boards` | Lista de boards do user | 2 min |

**Invalidacao event-driven:**

`CacheInvalidator` implementa `EventPublisher` e age como decorator:

```go
type CacheInvalidator struct {
    cache Cache
    next  EventPublisher // o Hub real
}

func (ci *CacheInvalidator) Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
    // Invalida cache baseado no evento
    ci.cache.Delete(ctx, fmt.Sprintf("board:%s:data", boardID))
    // Repassa pro Hub real
    ci.next.Publish(ctx, boardID, eventType, payload)
}
```

No `main.go`, a cadeia e: services -> CacheInvalidator -> Hub. Os services nao sabem que cache existe.

O `ListBoardColumns` do card service (e endpoints similares) checa cache antes de ir ao banco:
- Cache hit: retorna direto
- Cache miss: consulta banco, popula cache, retorna

Para isso, o handler (nao o service) checa o cache — mantendo o service puro. Ou, alternativamente, um middleware/decorator no service. A opcao mais limpa e um `CachedCardService` que wrapa o service real e adiciona cache nos metodos de leitura.

### 5.2. Rate Limiting com Redis

**Algoritmo:** sliding window counter.

**Implementacao:**
```
window_key = "ratelimit:{identifier}:{window_start}"
count_prev = GET window_key_prev
count_curr = INCR window_key_curr
EXPIRE window_key_curr {window_size + 1}
weight = 1 - (elapsed_in_current_window / window_size)
effective_count = count_prev * weight + count_curr
```

**Pacote:** `internal/platform/middleware/ratelimit.go`

**Limites:**

| Contexto | Limite | Chave |
|---|---|---|
| Rotas autenticadas (mutacao) | 100 req/min | `rl:user:{userID}:write` |
| Rotas autenticadas (leitura) | 300 req/min | `rl:user:{userID}:read` |
| Rotas publicas (login/register) | 20 req/min | `rl:ip:{ip}` |

**Headers de resposta:**
- `X-RateLimit-Limit`: limite da janela
- `X-RateLimit-Remaining`: requests restantes
- `X-RateLimit-Reset`: timestamp Unix do reset da janela

**Resposta quando excede:** `429 Too Many Requests` com header `Retry-After` (segundos ate o reset).

**Registro no router:** dois middlewares separados — um para rotas autenticadas, outro para rotas publicas. Registrados no `main.go` antes dos handlers.

### 5.3. Activity/Audit Log

**Dominio:** `internal/activity/`

**Entidade:**
```go
type Entry struct {
    ID             uuid.UUID
    BoardID        uuid.UUID
    ActorID        uuid.UUID
    Action         string    // ex: "card.created", "column.deleted"
    EntityType     string    // ex: "card", "column", "label", "comment"
    EntityID       uuid.UUID
    SnapshotBefore interface{} // JSONB — estado anterior (nil para creates)
    SnapshotAfter  interface{} // JSONB — estado novo (nil para deletes)
    CreatedAt      time.Time
}
```

**Recorder:** outro decorator do `EventPublisher`, na cadeia: services -> ActivityRecorder -> CacheInvalidator -> Hub.

O `ActivityRecorder` precisa saber _quem_ fez a acao (actor). O actor vem do context (ja propagado pelo auth middleware). Para acessar o context, o recorder usa `middleware.UserIDFromContext(ctx)`.

Para capturar `snapshot_before`, o recorder precisa ler o estado atual do banco _antes_ da mutacao. Mas o evento so e publicado _depois_ da mutacao. Solucao: o recorder nao captura `snapshot_before` — em vez disso, o service passa o estado anterior no payload do evento (como campo adicional). Isso e mais limpo que o recorder fazer queries ao banco.

Alternativa mais simples: os services passam `snapshot_before` e `snapshot_after` como campos no payload do evento. O recorder extrai esses campos e persiste. Isso requer uma convencao leve nos payloads de evento (adicionar campo `_before` quando relevante).

**API:** `GET /boards/{boardID}/activity?cursor=&limit=50` — paginacao cursor-based por `created_at DESC`.

**Eventos realtime:** `activity.created` — o frontend atualiza o painel de atividade em tempo real.

**Frontend:** painel lateral "Activity" no board — timeline de acoes com icone por tipo, nome do actor, timestamp relativo ("ha 2 min"), e link para o card/coluna afetado.

### 5.4. Webhook System

**Dominio:** `internal/webhook/`

**Entidades:** `Webhook` e `Delivery` conforme modelo de dados na secao 3.

**Service:**
- `Create(ctx, boardID, ownerID, url, secret, events)` — so o owner do board
- `Update(ctx, webhookID, ownerID, url, events, active)` — so o owner
- `Delete(ctx, webhookID, ownerID)` — so o owner
- `List(ctx, boardID, requesterID)` — qualquer membro
- `ListDeliveries(ctx, webhookID, requesterID, cursor, limit)` — qualquer membro

**API:**

| Metodo | Rota | Descricao |
|---|---|---|
| POST | `/boards/{boardID}/webhooks` | Cria webhook |
| GET | `/boards/{boardID}/webhooks` | Lista webhooks do board |
| PATCH | `/boards/{boardID}/webhooks/{webhookID}` | Edita webhook |
| DELETE | `/boards/{boardID}/webhooks/{webhookID}` | Deleta webhook |
| GET | `/boards/{boardID}/webhooks/{webhookID}/deliveries` | Log de entregas |

**Dispatcher:** observer do Hub. Quando um evento passa:
1. Busca webhooks ativos do board que escutam aquele event type
2. Para cada webhook, enfileira um job `webhook.deliver` no Redis

**Payload do POST:**
```json
{
  "event": "card.created",
  "board_id": "...",
  "timestamp": "2026-08-28T14:30:00Z",
  "data": { ... }
}
```

**Seguranca:** header `X-Kanvas-Signature: sha256=<hex>` — HMAC-SHA256 do body usando `webhook.secret`.

**Retry:** exponential backoff — intervalos 1s, 5s, 25s, 2min, 10min (5 tentativas maximo). Apos a 5a falha, status muda para `exhausted`.

### 5.5. Background Job Queue

**Pacote:** `internal/platform/worker/`

**Arquitetura:** Redis LIST como fila + goroutine worker no mesmo processo Go.

**Interface:**
```go
type Queue interface {
    Enqueue(ctx context.Context, job Job) error
    Dequeue(ctx context.Context, timeout time.Duration) (*Job, error)
}

type Job struct {
    ID      string          `json:"id"`
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"`
    Retries int             `json:"retries"`
}

type Handler func(ctx context.Context, payload json.RawMessage) error
```

**Implementacao Redis:**
- `Enqueue`: `LPUSH kanvas:jobs <json>`
- `Dequeue`: `BRPOP kanvas:jobs <timeout>` — bloqueia ate ter um job ou timeout

**Worker:**
```go
type Worker struct {
    queue    Queue
    handlers map[string]Handler
}

func (w *Worker) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            job, err := w.queue.Dequeue(ctx, 5*time.Second)
            if err != nil || job == nil {
                continue
            }
            handler, ok := w.handlers[job.Type]
            if !ok {
                log.Error("unknown job type", "type", job.Type)
                continue
            }
            if err := handler(ctx, job.Payload); err != nil {
                // re-enqueue com retry incrementado se < max
                w.handleFailure(ctx, *job, err)
            }
        }
    }
}
```

**Job types:**

| Tipo | Descricao | Trigger |
|---|---|---|
| `webhook.deliver` | HTTP POST para webhook URL | Webhook dispatcher |
| `token.cleanup` | Deleta refresh tokens expirados | Ticker goroutine (1h) |

**Graceful shutdown:** o worker respeita context cancelation — ao receber sinal de shutdown, termina o job atual e para.

## 6. Features de operacoes

### 6.1. Graceful Shutdown

**Mudanca no `main.go`:**

Troca `http.ListenAndServe` por:
```go
srv := &http.Server{Addr: ":" + cfg.Port, Handler: router}

go func() {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    srv.Shutdown(ctx)       // 1. drain HTTP
    hub.Close()             // 2. close WebSockets
    workerCancel()          // 3. stop worker
    redisClient.Close()     // 4. close Redis
    pool.Close()            // 5. close DB
}()

srv.ListenAndServe()
```

O Hub ganha `Close()` que envia close frame (`1001 Going Away`) para todos os subscribers e fecha os channels.

### 6.2. Structured Logging + Prometheus Metrics

**Logging:**
- Migra de `log.Printf` para `slog.Logger` com JSON handler
- Cada request recebe um `correlation_id` (UUID) via middleware, propagado no context
- Middleware de logging substitui `chimiddleware.Logger`
- Formato: `{"time":"...","level":"INFO","msg":"request completed","correlation_id":"...","method":"GET","path":"/boards","status":200,"duration_ms":12}`

**Metricas Prometheus:**

Pacote: `internal/platform/metrics/`

| Metrica | Tipo | Labels |
|---|---|---|
| `http_requests_total` | Counter | method, path, status |
| `http_request_duration_seconds` | Histogram | method, path |
| `websocket_connections_active` | Gauge | — |
| `cache_operations_total` | Counter | operation (get/set/delete), result (hit/miss/error) |
| `worker_jobs_processed_total` | Counter | job_type, result (success/failure) |
| `webhook_deliveries_total` | Counter | status (success/failed/exhausted) |

Endpoint `/metrics` registrado no router, sem autenticacao (padrao Prometheus).

### 6.3. Health Checks Avancados

Substituem o `/healthz` existente:

**`/livez`** — o processo esta rodando:
- Retorna `200 {"status":"ok"}` sempre, a menos que graceful shutdown tenha comecado
- Depois de receber SIGTERM: retorna `503 {"status":"shutting_down"}`

**`/readyz`** — o processo esta pronto para receber trafego:
- Checa: DB ping, Redis ping
- Todos OK: `200 {"status":"ready","checks":{"db":"ok","redis":"ok"}}`
- Algum falhou: `503 {"status":"not_ready","checks":{"db":"ok","redis":"error: connection refused"}}`

Docker Compose backend healthcheck muda para `curl -f http://localhost:8080/readyz`.

## 7. Docker Compose — adicoes

```yaml
redis:
  image: redis:7-alpine
  ports:
    - "6379:6379"
  healthcheck:
    test: ["CMD", "redis-cli", "ping"]
    interval: 5s
    timeout: 5s
    retries: 5

# prometheus (opcional, para dashboards locais)
prometheus:
  image: prom/prometheus:latest
  volumes:
    - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
  ports:
    - "9090:9090"
```

O backend ganha `REDIS_URL: redis://redis:6379` no environment.

O `depends_on` do backend adiciona `redis: condition: service_healthy`.

Prometheus e opcional — presente no compose para quem quiser visualizar metricas localmente, mas nao e dependencia do backend.

## 8. Testes

Cada novo dominio segue o mesmo padrao de testes do projeto existente:

**Unit tests (com fakes):**
- `comment/service_test.go` — CRUD, permissoes (so autor edita, autor/owner deleta), cursor pagination
- `label/service_test.go` — CRUD, attach/detach, validacao de cor, unicidade de nome por board
- `webhook/service_test.go` — CRUD, permissoes (so owner), filtragem por events
- `activity/recorder_test.go` — registro correto de entries por tipo de evento
- `worker/worker_test.go` — processamento de jobs, retry, graceful shutdown
- `cache/invalidator_test.go` — invalidacao correta por tipo de evento
- `middleware/ratelimit_test.go` — sliding window, headers corretos, 429 quando excede

**Integration tests (com Postgres real via testcontainers):**
- `comment/e2e_test.go` — CRUD via HTTP, cursor pagination, permissoes
- `label/e2e_test.go` — CRUD via HTTP, attach/detach, filtragem
- `search/e2e_test.go` — full-text search com dados reais, ranking
- `webhook/e2e_test.go` — CRUD via HTTP, delivery log
- `activity/e2e_test.go` — listar activity entries via HTTP
- `realtime/presence_test.go` — join/leave/heartbeat timeout

**Integration tests (com Redis real via testcontainers):**
- `cache/redis_test.go` — get/set/delete, TTL
- `middleware/ratelimit_integration_test.go` — sliding window com Redis real
- `worker/queue_test.go` — enqueue/dequeue com Redis real

## 9. Arquitetura — cadeia de event publishers

A cadeia de decorators no `EventPublisher` (registrada no `main.go`):

```
Services
    |
    v
ActivityRecorder  (persiste no activity_log)
    |
    v
WebhookDispatcher (enfileira webhook.deliver jobs)
    |
    v
CacheInvalidator  (invalida keys no Redis)
    |
    v
Hub               (broadcast WebSocket para clients conectados)
```

Cada camada implementa `EventPublisher` e delega para a proxima. Os services publicam uma unica vez — os 4 observers processam em cadeia, de forma transparente.

## 10. Frontend — resumo das mudancas

| Feature | Componente | Descricao |
|---|---|---|
| Comments | `CardDetailModal` | Secao de comentarios com lista, input, realtime |
| Labels | `CardItem`, `Column`, `CardDetailModal`, `BoardPage` | Chips coloridos, picker, filtro, painel de gestao |
| Search | `BoardPage` | Search bar no header com dropdown de resultados |
| Presence | `BoardPage` | Barra de avatares com indicador online |
| Activity | `BoardPage` | Painel lateral "Activity" com timeline |

Novos event types no hook `useBoardRealtime.ts`: `comment.*`, `label.*`, `card.label_*`, `activity.created`, `presence.joined`, `presence.left`.

Webhooks nao tem UI neste escopo — sao gerenciados via API (curl/Postman). Se necessario, um painel de webhooks pode ser adicionado como follow-up.

## 11. Ordem de implementacao sugerida

A ordem respeita dependencias entre features:

1. **Redis + cache infra** — adiciona Redis ao compose, pacote cache, config
2. **Graceful shutdown + health checks** — fundacao operacional
3. **Structured logging + Prometheus metrics** — observabilidade desde o inicio
4. **Rate limiting** — middleware, depende de Redis
5. **Labels** — feature independente, prepara terreno para search
6. **Comments** — feature independente, prepara terreno para search
7. **Full-text search** — depende de cards e comments existirem com search_vector
8. **Presence** — extensao do Hub, independente
9. **Activity/Audit log** — decorator do EventPublisher
10. **Background job queue** — worker infra, depende de Redis
11. **Webhook system** — depende de job queue para delivery
12. **README update** — documenta todas as features

## 12. Limitacoes conhecidas (mantidas da v1)

- **Last-write-wins** em moves concorrentes — nao resolvido neste upgrade (seria CRDT, fora do escopo)
- **Hub in-memory** — a presenca e o fan-out continuam in-process; para multi-instancia, precisaria de Redis pub/sub (candidato a v3)
- **Webhooks sem UI** — gerenciamento apenas via API
- **Search so em ingles** — `to_tsvector('english', ...)` nao suporta portugues sem configuracao adicional de dicionario
