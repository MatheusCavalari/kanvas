# FitDuel — Design Spec

**App de desafios fitness entre amigos, offline-first.**

Flutter + Dart · SQLite · QR Code sync · Gamificação

---

## Conceito

FitDuel é um app mobile onde usuários criam desafios fitness (ex: "200 flexões em 5 dias"), compartilham com amigos via deep link ou QR code, e competem registrando progresso no próprio dispositivo. Resultados são comparados via sync por QR code quando os participantes se encontram.

Sem backend centralizado — dados 100% locais com SQLite.

## Fluxo Principal

1. Usuário cria um desafio: exercício, meta, duração, tipo (1v1 ou grupo até 6)
2. Compartilha via deep link ou QR code (dados do desafio codificados no payload)
3. Participantes abrem o link/escaneiam QR → desafio importado no app
4. Cada pessoa registra progresso diário no seu dispositivo
5. Para comparar: cada um gera QR com seu progresso, o outro escaneia → ranking montado localmente

## Funcionalidades Core

- **Criar desafio** — exercício (lista predefinida + custom), meta numérica, unidade (reps/km/min), prazo (1-30 dias)
- **Entrar em desafio** — via deep link ou scan QR
- **Registrar progresso** — input rápido com valor + data, máximo 2 toques
- **Dashboard pessoal** — gráficos de evolução, histórico, streak de dias ativos
- **Comparação/Ranking** — sincronizar via QR para ver ranking entre participantes
- **Badges/Conquistas** — completar desafios, streaks, metas pessoais

### Fora de escopo (v1)

- Chat entre participantes
- Push notifications
- Backend centralizado / conta de usuário
- Integração com wearables
- Deploy em app stores

## Stack Técnico

| Camada | Tecnologia |
|---|---|
| Framework | Flutter 3.x + Dart |
| Persistência | SQLite via `sqflite` |
| Estado | Riverpod |
| Navegação | GoRouter (com deep link support) |
| Gráficos | fl_chart |
| QR Code | qr_flutter (gerar) + mobile_scanner (ler) |
| Compartilhamento | share_plus |

## Arquitetura

Clean Architecture simplificada com separação por feature:

```
lib/
├── core/               # tema, constantes, utils, database helper
├── features/
│   ├── challenges/
│   │   ├── domain/     # entities (Challenge), repository interface
│   │   ├── data/       # SQLite repository impl, models
│   │   └── presentation/ # screens, widgets, controllers (Riverpod)
│   ├── progress/
│   │   ├── domain/     # entities (ProgressEntry), repository interface
│   │   ├── data/       # SQLite repository impl
│   │   └── presentation/ # registro de progresso, gráficos
│   ├── sync/
│   │   ├── domain/     # payload models, codec interface
│   │   ├── data/       # JSON serialization, compression, base64
│   │   └── presentation/ # telas de scan/share QR
│   └── achievements/
│       ├── domain/     # entities (Achievement), regras de desbloqueio
│       ├── data/       # SQLite impl
│       └── presentation/ # badges grid, animações de unlock
└── main.dart
```

## Modelo de Dados (SQLite)

### challenges
| Coluna | Tipo | Descrição |
|---|---|---|
| id | TEXT (UUID) | Identificador único |
| title | TEXT | Nome do desafio |
| exercise | TEXT | Exercício (ex: "Flexões") |
| target | INTEGER | Meta numérica |
| unit | TEXT | Unidade: reps, km, min |
| duration_days | INTEGER | Duração em dias (1-30) |
| start_date | TEXT (ISO8601) | Data de início |
| status | TEXT | active, completed, expired |
| created_at | TEXT (ISO8601) | Timestamp de criação |

### participants
| Coluna | Tipo | Descrição |
|---|---|---|
| id | TEXT (UUID) | Identificador único |
| challenge_id | TEXT | FK → challenges.id |
| name | TEXT | Nome do participante |
| is_self | INTEGER | 1 se é o dono do dispositivo |

### progress_entries
| Coluna | Tipo | Descrição |
|---|---|---|
| id | TEXT (UUID) | Identificador único |
| challenge_id | TEXT | FK → challenges.id |
| participant_id | TEXT | FK → participants.id |
| value | REAL | Valor registrado |
| date | TEXT (ISO8601) | Data do registro |
| created_at | TEXT (ISO8601) | Timestamp de criação |

### achievements
| Coluna | Tipo | Descrição |
|---|---|---|
| id | TEXT (UUID) | Identificador único |
| type | TEXT | Tipo: first_challenge, streak_7, etc. |
| unlocked_at | TEXT (ISO8601) | Quando foi desbloqueado |

## Sync via QR / Deep Link

### Payload de desafio (para compartilhar)

```json
{
  "v": 1,
  "c": {
    "id": "uuid",
    "title": "200 Flexões",
    "exercise": "Flexões",
    "target": 200,
    "unit": "reps",
    "duration_days": 5,
    "start_date": "2026-09-24"
  },
  "p": "NomeDoParticipante"
}
```

Fluxo: JSON → gzip compress → base64url encode → QR code ou deep link query param.

### Payload de progresso (para comparar)

```json
{
  "v": 1,
  "challenge_id": "uuid",
  "participant": "João",
  "entries": [
    {"date": "2026-09-24", "value": 50},
    {"date": "2026-09-25", "value": 80}
  ]
}
```

Mesmo fluxo de encode. Ao escanear, o app importa as entradas e monta o ranking.

## Telas e Navegação

Bottom navigation com 3 tabs:

### Tab 1 — Home
Lista de desafios ativos em cards. Cada card mostra: título, exercício, barra de progresso %, dias restantes, avatares dos participantes. FAB "+" abre o wizard de criação.

### Tab 2 — Perfil/Stats
Estatísticas pessoais: total de desafios completados, streak atual, heatmap de atividade (estilo GitHub contributions), grid de badges desbloqueadas.

### Tab 3 — Sync
Duas opções: "Escanear QR" (abre câmera) e "Meu QR" (gera QR com progresso do desafio selecionado). Lista de deep links pendentes.

### Telas modais
- **Criar Desafio** — Wizard 3 passos: exercício → meta/duração → preview + compartilhar
- **Detalhe do Desafio** — Header com info, ranking dos participantes, botão "Registrar", timeline de entradas
- **Registrar Progresso** — Bottom sheet com input numérico grande + data (default: hoje)

## Design Visual

- **Dark-first** com tema esportivo e clean
- Cores primárias: verde (#00E676) e azul (#448AFF) sobre fundo escuro (#121212)
- Material Design 3 como base
- Animações: barra de progresso preenchendo, badge desbloqueando com bounce, confetti ao completar desafio
- Tipografia bold para números e métricas

## Testes

### Unit tests
- Cálculo de progresso percentual
- Ranking de participantes (ordenação por valor)
- Validação de desafio (meta > 0, duração 1-30, etc.)
- Serialização/deserialização do QR payload (encode → decode roundtrip)
- Regras de desbloqueio de badges

### Widget tests
- Card de desafio renderiza corretamente
- Barra de progresso reflete valor
- Input de registro aceita valores válidos
- Wizard de criação navega entre passos

### Integration tests
- Fluxo completo: criar desafio → registrar progresso → ver atualização no dashboard
- Importar desafio via deep link simulado

## CI/CD (GitHub Actions)

```yaml
# .github/workflows/ci.yml
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - flutter analyze
      - flutter test
      - flutter build apk --debug
```

Badge de status do CI no README.

## README

O README do repositório terá:
- Descrição do projeto e motivação
- Screenshots das telas principais
- GIF animado mostrando o fluxo criar → compartilhar → registrar → ranking
- Seção de tech stack com badges
- Instruções para rodar localmente (`flutter run`)
- Licença MIT
