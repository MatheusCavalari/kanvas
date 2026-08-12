# Kanvas — Fase 5: Frontend Setup + Autenticação — Design

**Data:** 2026-08-11
**Status:** Aprovado para planejamento

## 1. Contexto e escopo

Fases 1–4 (backend) estão completas e mergeadas: autenticação JWT (access + refresh via cookie httpOnly), boards com membros, colunas/cards, e broadcast de eventos em tempo real via WebSocket (`docs/superpowers/specs/2026-08-10-kanvas-design.md`).

A Fase 5 inicia o frontend: setup do projeto React + TypeScript e o fluxo completo de autenticação (registro, login, refresh automático, logout) contra o backend real. Não inclui a listagem de boards nem a tela de board em si — isso fica para as Fases 6 e 7, respectivamente. A Fase 5 termina num placeholder pós-login autenticado.

**Fora do escopo desta fase:**
- Lista/criação de boards (Fase 6)
- Colunas, cards, drag-and-drop (Fase 6)
- Integração com WebSocket / eventos em tempo real (Fase 7)
- TanStack Query (só entra na Fase 6, quando há dados de board para cachear)
- Testes E2E com Playwright (a partir da Fase 6/7, quando há fluxo de board para exercitar)
- Deploy real / Dockerfile de produção completo (Fase 8), embora um `Dockerfile` básico de build seja criado nesta fase para não deixar a lacuna crescer

## 2. Estrutura do projeto

```
frontend/
  src/
    api/
      client.ts          # wrapper fetch com interceptor de refresh automático
      auth.ts             # funções da API de auth (register, login, refresh, logout)
    features/
      auth/
        LoginPage.tsx
        RegisterPage.tsx
        useAuthStore.ts    # Zustand: user, access token (em memória), status
    components/
      layout/
        AppLayout.tsx      # shell autenticado (header + área de conteúdo)
      ui/                  # componentes de UI base reutilizáveis (botão, input)
    routes/
      router.tsx           # React Router: rotas públicas vs protegidas
    lib/
      env.ts               # leitura de variáveis de ambiente (VITE_API_URL)
    App.tsx
    main.tsx
  index.html
  vite.config.ts
  tailwind.config.js
  .env.example
  Dockerfile               # build Vite + Nginx (usado a partir da Fase 8)
```

**Stack técnica:**
- [Vite](https://vitejs.dev/) + React + TypeScript
- [Tailwind CSS](https://tailwindcss.com/) para estilo
- [React Router](https://reactrouter.com/) para roteamento client-side
- [Zustand](https://github.com/pmndrs/zustand) para estado de sessão
- [Vitest](https://vitest.dev/) + [Testing Library](https://testing-library.com/) para testes

## 3. Autenticação

- `useAuthStore` (Zustand) guarda `user` (id, name, email), `accessToken` e `status` (`idle` | `authenticated` | `unauthenticated`). O `accessToken` vive só em memória — nunca em `localStorage` — para reduzir exposição a XSS, conforme o design original (seção 6).
- Ao montar o app, `App.tsx` dispara silenciosamente `POST /auth/refresh` (o cookie `httpOnly` já contém o refresh token se a sessão anterior for válida) para restaurar a sessão sem exigir novo login. Enquanto essa checagem inicial está em andamento, `status` fica `idle` e a UI mostra um estado de carregamento mínimo antes de decidir a rota.
- `client.ts` injeta `Authorization: Bearer <accessToken>` em toda request autenticada. Se uma resposta vier `401`, o client tenta `POST /auth/refresh` **uma única vez** e repete a request original com o novo token; se esse refresh também falhar, o store muda para `unauthenticated`, o `accessToken` é limpo, e o usuário é redirecionado ao login. Essa é a única tentativa de retry — não há fila de requests concorrentes aguardando o mesmo refresh nesta fase (ver seção 6, casos de borda).
- Logout chama `POST /auth/logout` (revoga o refresh token no backend) e limpa `useAuthStore`.
- Erros de validação do backend, que seguem o envelope `{"error":{"code","message"}}` (documentado em `backend/README.md`), são mapeados para mensagens exibidas nos formulários de login/registro — mensagem geral para códigos genéricos (`unauthorized`, `invalid_request`), mensagem por campo quando o `code`/contexto permitir (ex: e-mail já cadastrado no registro).

## 4. Rotas

- `/login`, `/register` — públicas. Se o usuário já está `authenticated`, redirecionam para `/`.
- `/` — protegida por um wrapper `RequireAuth`: se `status` é `unauthenticated`, redireciona para `/login` preservando a URL de destino original (via `state` de navegação) para retomar após o login. Nesta fase, `/` é apenas um placeholder ("Bem-vindo, {nome}" + botão de logout) dentro do `AppLayout` — a lista de boards substitui esse placeholder na Fase 6.
- `AppLayout` (header com nome do usuário e botão de logout) envolve todas as rotas protegidas.

## 5. Testes

Vitest + Testing Library, sem dependência de rede real (fetch mockado):
- `useAuthStore`: transições de estado — login com sucesso, login com falha, refresh bem-sucedido, refresh falho, logout.
- `client.ts`: o interceptor de refresh — simula uma resposta `401` seguida de refresh bem-sucedido e retry automático da request original; e o caso onde o refresh também falha (usuário é deslogado, sem loop).
- `LoginPage` / `RegisterPage`: validação de formulário (campos obrigatórios, formato de e-mail) e submissão (chamada correta à API, exibição de erro do backend).
- `RequireAuth`: redireciona quando não autenticado, renderiza os filhos quando autenticado.

## 6. CI

Novo `.github/workflows/frontend-ci.yml`, espelhando a estrutura do `backend-ci.yml` existente: em push/PR, roda `npm ci`, `eslint`, `vitest run`, `npm run build` dentro de `frontend/`.

## 7. Erros e casos de borda

- **Refresh falhando na carga inicial** (cookie ausente/expirado): `status` vai direto para `unauthenticated`, sem erro visível — é o caso normal de "nunca logou" ou "sessão expirou".
- **Duas requisições simultâneas recebendo 401 ao mesmo tempo**: nesta fase, cada uma dispara seu próprio `/auth/refresh` independentemente (sem deduplicação/fila). Isso pode gerar uma chamada de refresh redundante ocasional, mas não quebra a sessão — o backend aceita refresh concorrente. Deduplicar é um candidato de melhoria futura, não bloqueia a Fase 5.
- **`VITE_API_URL` ausente ou incorreta**: `lib/env.ts` lança erro explícito na inicialização (fail-fast) em vez de deixar todas as chamadas falharem silenciosamente com mensagens confusas.
- **Envio de formulário com o backend fora do ar**: erro de rede é tratado separadamente dos erros de validação (mensagem genérica "não foi possível conectar ao servidor", sem tentar interpretar como envelope JSON de erro).
