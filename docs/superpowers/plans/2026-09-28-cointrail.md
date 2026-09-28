# CoinTrail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a personal finance REST API in Clojure with JWT auth, transaction management, category CRUD, and financial reports — fully containerized with Docker Compose.

**Architecture:** Layered Clojure application using Ring + Compojure for HTTP, next.jdbc + HoneySQL for PostgreSQL access, buddy for JWT auth, and clojure.spec for validation. Each domain (auth, categories, transactions, reports) has its own handlers/service/repository namespaces. Multi-stage Docker build produces a lean JRE image.

**Tech Stack:** Clojure 1.12, Ring 1.12, Compojure, next.jdbc, HoneySQL 2, buddy-auth, buddy-hashers, clojure.spec.alpha, Migratus, environ, timbre, PostgreSQL 16, Leiningen, Docker Compose, GitHub Actions, clj-kondo

## Global Constraints

- Clojure 1.12 minimum, JVM 21 (Eclipse Temurin)
- PostgreSQL 16
- All config from environment variables via environ — zero hardcoded values
- Handlers are thin — extract, validate, delegate, respond
- Services are pure functions when possible
- Repositories encapsulate all SQL — no other namespace uses HoneySQL directly
- All listing endpoints return paginated response `{data: [...], meta: {page, per_page, total, total_pages}}`
- Error response format: `{"error": "code", "message": "...", "details": {...}}`
- Status codes: 400 (validation), 401 (unauthenticated), 403 (wrong owner), 404 (not found), 409 (duplicate)
- JWT tokens expire in 24 hours, payload: `{user_id, email, exp}`
- Password minimum 8 characters, hashed with bcrypt
- Seed categories created on user registration
- Migrations auto-run on boot via Migratus
- Docker Compose: app (:8080) + postgres (:5432)
- CI: clj-kondo lint + lein test (with PG service container) + docker compose build

---

### Task 1: Project Scaffolding, Config, and Database

**Files:**
- Create: `project.clj`
- Create: `src/cointrail/core.clj`
- Create: `src/cointrail/config.clj`
- Create: `src/cointrail/db.clj`
- Create: `src/cointrail/middleware.clj`
- Create: `src/cointrail/routes.clj`
- Create: `resources/migrations/001-create-users.up.sql`
- Create: `resources/migrations/001-create-users.down.sql`
- Create: `resources/migrations/002-create-categories.up.sql`
- Create: `resources/migrations/002-create-categories.down.sql`
- Create: `resources/migrations/003-create-transactions.up.sql`
- Create: `resources/migrations/003-create-transactions.down.sql`
- Create: `docker-compose.yml`
- Create: `.env.example`
- Create: `.gitignore`
- Create: `test/cointrail/db_test.clj`

**Interfaces:**
- Consumes: nothing (first task)
- Produces:
  - `cointrail.config/config` — function `(config)` returning `{:database-url string, :jwt-secret string, :port int}`
  - `cointrail.db/datasource` — atom holding a `javax.sql.DataSource` (HikariCP pool)
  - `cointrail.db/init!` — `(init! database-url)` creates the pool and runs migrations
  - `cointrail.middleware/wrap-json` — Ring middleware for JSON request/response
  - `cointrail.middleware/wrap-cors` — Ring CORS middleware
  - `cointrail.middleware/wrap-logging` — Ring request logging middleware
  - `cointrail.middleware/wrap-auth` — Ring JWT authentication middleware
  - `cointrail.routes/app-routes` — Compojure routes (initially just health)
  - `cointrail.core/-main` — entry point that starts Jetty

- [ ] **Step 1: Create `project.clj`**

```clojure
(defproject cointrail "0.1.0"
  :description "Personal Finance REST API"
  :license {:name "MIT"}
  :min-lein-version "2.0.0"
  :dependencies [[org.clojure/clojure "1.12.0"]
                 [ring/ring-core "1.12.2"]
                 [ring/ring-jetty-adapter "1.12.2"]
                 [ring/ring-json "0.5.1"]
                 [compojure "1.7.1"]
                 [com.github.seancorfield/next.jdbc "1.3.939"]
                 [com.github.seancorfield/honeysql "2.6.1196"]
                 [org.postgresql/postgresql "42.7.4"]
                 [com.zaxxer/HikariCP "6.2.1"]
                 [migratus "1.5.8"]
                 [buddy/buddy-auth "3.0.323"]
                 [buddy/buddy-hashers "2.0.167"]
                 [buddy/buddy-sign "3.6.1-359"]
                 [environ "1.2.0"]
                 [com.taoensso/timbre "6.6.1"]
                 [ring-cors "0.1.13"]
                 [metosin/ring-swagger "0.26.2"]
                 [metosin/ring-swagger-ui "5.9.0"]
                 [cheshire "5.13.0"]]
  :plugins [[lein-environ "1.2.0"]]
  :main cointrail.core
  :aot [cointrail.core]
  :uberjar-name "cointrail-standalone.jar"
  :profiles {:dev {:dependencies [[clj-kondo "2024.08.01"]]
                   :env {:database-url "jdbc:postgresql://localhost:5432/cointrail?user=cointrail&password=cointrail"
                         :jwt-secret "dev-secret-change-in-production"
                         :port "8080"}}
             :test {:env {:database-url "jdbc:postgresql://localhost:5432/cointrail_test?user=cointrail&password=cointrail"
                          :jwt-secret "test-secret"
                          :port "8080"}}})
```

- [ ] **Step 2: Create `.gitignore`**

```
/target
/classes
/checkouts
pom.xml
pom.xml.asc
*.jar
*.class
/.lein-*
/.nrepl-port
/.env
.hsp
.calva
.clj-kondo/.cache
```

- [ ] **Step 3: Create `.env.example`**

```
DATABASE_URL=jdbc:postgresql://localhost:5432/cointrail?user=cointrail&password=cointrail
JWT_SECRET=change-me-in-production
PORT=8080
```

- [ ] **Step 4: Create `src/cointrail/config.clj`**

```clojure
(ns cointrail.config
  (:require [environ.core :refer [env]]))

(defn config []
  {:database-url (or (:database-url env)
                     (throw (ex-info "DATABASE_URL not set" {})))
   :jwt-secret   (or (:jwt-secret env)
                      (throw (ex-info "JWT_SECRET not set" {})))
   :port         (Integer/parseInt (or (:port env) "8080"))})
```

- [ ] **Step 5: Create `src/cointrail/db.clj`**

```clojure
(ns cointrail.db
  (:require [next.jdbc :as jdbc]
            [next.jdbc.result-set :as rs]
            [migratus.core :as migratus]
            [taoensso.timbre :as log])
  (:import [com.zaxxer.hikari HikariDataSource HikariConfig]))

(defonce datasource (atom nil))

(defn- make-pool [database-url]
  (let [config (doto (HikariConfig.)
                 (.setJdbcUrl database-url)
                 (.setMaximumPoolSize 10)
                 (.setMinimumIdle 2)
                 (.setConnectionTimeout 30000))]
    (HikariDataSource. config)))

(defn- run-migrations! [ds]
  (migratus/migrate {:store :database
                     :migration-dir "migrations"
                     :db {:datasource ds}}))

(defn init! [database-url]
  (log/info "Initializing database connection pool")
  (let [pool (make-pool database-url)]
    (reset! datasource pool)
    (run-migrations! pool)
    (log/info "Database ready, migrations applied")))

(defn execute! [sql-params]
  (jdbc/execute! @datasource sql-params
                 {:builder-fn rs/as-unqualified-kebab-maps}))

(defn execute-one! [sql-params]
  (jdbc/execute-one! @datasource sql-params
                     {:builder-fn rs/as-unqualified-kebab-maps}))
```

- [ ] **Step 6: Create migration files**

`resources/migrations/001-create-users.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);
```

`resources/migrations/001-create-users.down.sql`:
```sql
DROP TABLE IF EXISTS users;
```

`resources/migrations/002-create-categories.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    type VARCHAR(7) NOT NULL CHECK (type IN ('income', 'expense')),
    icon VARCHAR(30),
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(user_id, name, type)
);

CREATE INDEX idx_categories_user_id ON categories(user_id);
```

`resources/migrations/002-create-categories.down.sql`:
```sql
DROP TABLE IF EXISTS categories;
```

`resources/migrations/003-create-transactions.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    type VARCHAR(7) NOT NULL CHECK (type IN ('income', 'expense')),
    amount DECIMAL(12,2) NOT NULL CHECK (amount > 0),
    description VARCHAR(255),
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_transactions_user_date ON transactions(user_id, date);
CREATE INDEX idx_transactions_user_category ON transactions(user_id, category_id);
```

`resources/migrations/003-create-transactions.down.sql`:
```sql
DROP TABLE IF EXISTS transactions;
```

- [ ] **Step 7: Create `src/cointrail/middleware.clj`**

```clojure
(ns cointrail.middleware
  (:require [ring.middleware.json :refer [wrap-json-response wrap-json-body]]
            [ring-cors.core :refer [wrap-cors]]
            [buddy.auth :refer [authenticated?]]
            [buddy.auth.backends :as backends]
            [buddy.auth.middleware :refer [wrap-authentication]]
            [buddy.sign.jwt :as jwt]
            [taoensso.timbre :as log]))

(defn wrap-json [handler]
  (-> handler
      (wrap-json-body {:keywords? true :bigdecimals? true})
      (wrap-json-response {:pretty false})))

(defn wrap-request-logging [handler]
  (fn [request]
    (let [start (System/currentTimeMillis)
          response (handler request)
          elapsed (- (System/currentTimeMillis) start)]
      (log/info (:request-method request) (:uri request)
                (:status response) (str elapsed "ms"))
      response)))

(defn wrap-cors-headers [handler]
  (wrap-cors handler
             :access-control-allow-origin [#".*"]
             :access-control-allow-methods [:get :post :put :delete :options]
             :access-control-allow-headers [:content-type :authorization]))

(defn jwt-backend [secret]
  (backends/jws {:secret secret
                 :token-name "Bearer"}))

(defn wrap-jwt-auth [handler secret]
  (wrap-authentication handler (jwt-backend secret)))

(defn require-auth [handler]
  (fn [request]
    (if (authenticated? request)
      (handler request)
      {:status 401
       :body {:error "unauthorized" :message "Authentication required"}})))
```

- [ ] **Step 8: Create `src/cointrail/routes.clj`**

```clojure
(ns cointrail.routes
  (:require [compojure.core :refer [defroutes GET context]]
            [compojure.route :as route]
            [cointrail.db :as db]))

(defn health-handler [_]
  (let [db-ok? (try
                 (db/execute-one! ["SELECT 1 AS ok"])
                 true
                 (catch Exception _ false))]
    {:status (if db-ok? 200 503)
     :body {:status (if db-ok? "ok" "degraded")
            :database (if db-ok? "connected" "disconnected")
            :version "0.1.0"}}))

(defroutes app-routes
  (context "/api" []
    (GET "/health" [] health-handler))
  (route/not-found {:status 404
                    :body {:error "not_found" :message "Resource not found"}}))
```

- [ ] **Step 9: Create `src/cointrail/core.clj`**

```clojure
(ns cointrail.core
  (:require [ring.adapter.jetty :refer [run-jetty]]
            [cointrail.config :refer [config]]
            [cointrail.db :as db]
            [cointrail.middleware :as mw]
            [cointrail.routes :refer [app-routes]]
            [taoensso.timbre :as log])
  (:gen-class))

(defn create-app [cfg]
  (-> app-routes
      (mw/wrap-jwt-auth (:jwt-secret cfg))
      mw/wrap-cors-headers
      mw/wrap-json
      mw/wrap-request-logging))

(defn -main [& _args]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (let [app (create-app cfg)]
      (log/info "Starting CoinTrail on port" (:port cfg))
      (run-jetty app {:port (:port cfg) :join? true}))))
```

- [ ] **Step 10: Create `docker-compose.yml`**

```yaml
services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: cointrail
      POSTGRES_USER: cointrail
      POSTGRES_PASSWORD: cointrail
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-ONLY", "pg_isready", "-U", "cointrail"]
      interval: 5s
      timeout: 5s
      retries: 5

  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: jdbc:postgresql://db:5432/cointrail?user=cointrail&password=cointrail
      JWT_SECRET: dev-secret-change-in-production
      PORT: "8080"
    depends_on:
      db:
        condition: service_healthy

volumes:
  pgdata:
```

- [ ] **Step 11: Write test for db connection and health endpoint**

`test/cointrail/db_test.clj`:
```clojure
(ns cointrail.db-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (f)))

(use-fixtures :once db-fixture)

(deftest health-endpoint-test
  (let [cfg (config)
        app (create-app cfg)
        response (app (mock/request :get "/api/health"))
        body (json/parse-string (slurp (:body response)) true)]
    (testing "returns 200 with status ok"
      (is (= 200 (:status response)))
      (is (= "ok" (:status body)))
      (is (= "connected" (:database body)))
      (is (= "0.1.0" (:version body))))))
```

- [ ] **Step 12: Add ring-mock to test dependencies in `project.clj`**

Add to `:profiles :test :dependencies`:
```clojure
:test {:dependencies [[ring/ring-mock "0.4.0"]]
       :env {:database-url "jdbc:postgresql://localhost:5432/cointrail_test?user=cointrail&password=cointrail"
             :jwt-secret "test-secret"
             :port "8080"}}
```

- [ ] **Step 13: Verify tests pass**

Run: `lein test cointrail.db-test`
Expected: PASS — health endpoint returns 200 with correct body.

- [ ] **Step 14: Commit**

```bash
git add -A
git commit -m "feat: project scaffolding with config, db, migrations, middleware, and health endpoint"
```

---

### Task 2: Auth — Registration and Login

**Files:**
- Create: `src/cointrail/auth/spec.clj`
- Create: `src/cointrail/auth/service.clj`
- Create: `src/cointrail/auth/handlers.clj`
- Modify: `src/cointrail/routes.clj`
- Create: `test/cointrail/auth/handlers_test.clj`

**Interfaces:**
- Consumes: `cointrail.db/execute!`, `cointrail.db/execute-one!`, `cointrail.middleware/require-auth`
- Produces:
  - `cointrail.auth.service/hash-password` — `(hash-password password)` returns bcrypt hash string
  - `cointrail.auth.service/check-password` — `(check-password password hash)` returns boolean
  - `cointrail.auth.service/generate-token` — `(generate-token user secret)` returns JWT string
  - `cointrail.auth.service/register!` — `(register! {:keys [email password name]})` returns `{:user map :token string}` or throws
  - `cointrail.auth.service/login!` — `(login! {:keys [email password]})` returns `{:user map :token string}` or throws
  - `cointrail.auth.service/seed-default-categories!` — `(seed-default-categories! user-id)` inserts default categories for new user

- [ ] **Step 1: Create `src/cointrail/auth/spec.clj`**

```clojure
(ns cointrail.auth.spec
  (:require [clojure.spec.alpha :as s]))

(s/def ::email (s/and string? #(re-matches #"^[^\s@]+@[^\s@]+\.[^\s@]+$" %)))
(s/def ::password (s/and string? #(>= (count %) 8)))
(s/def ::name (s/and string? #(>= (count %) 1) #(<= (count %) 100)))

(s/def ::register-request (s/keys :req-un [::email ::password ::name]))
(s/def ::login-request (s/keys :req-un [::email ::password]))

(defn validate [spec data]
  (when-not (s/valid? spec data)
    (let [problems (s/explain-data spec data)
          details (->> (:clojure.spec.alpha/problems problems)
                       (map (fn [p]
                              [(last (:path p))
                               (str "failed: " (:pred p))]))
                       (into {}))]
      {:error "validation_failed"
       :message "Invalid request data"
       :details details})))
```

- [ ] **Step 2: Create `src/cointrail/auth/service.clj`**

```clojure
(ns cointrail.auth.service
  (:require [buddy.hashers :as hashers]
            [buddy.sign.jwt :as jwt]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [honey.sql :as sql]
            [honey.sql.helpers :as h]))

(defn hash-password [password]
  (hashers/derive password {:alg :bcrypt+sha512}))

(defn check-password [password hash]
  (:valid (hashers/verify password hash)))

(defn generate-token [user secret]
  (jwt/sign {:user-id (str (:id user))
             :email (:email user)
             :exp (-> (java.time.Instant/now)
                      (.plusSeconds 86400)
                      .getEpochSecond)}
            secret))

(def default-categories
  [{:name "Salário"        :type "income"  :icon "💰"}
   {:name "Freelance"      :type "income"  :icon "💻"}
   {:name "Investimentos"  :type "income"  :icon "📈"}
   {:name "Outros"         :type "income"  :icon "📋"}
   {:name "Alimentação"    :type "expense" :icon "🍔"}
   {:name "Transporte"     :type "expense" :icon "🚗"}
   {:name "Moradia"        :type "expense" :icon "🏠"}
   {:name "Saúde"          :type "expense" :icon "🏥"}
   {:name "Lazer"          :type "expense" :icon "🎮"}
   {:name "Educação"       :type "expense" :icon "📚"}
   {:name "Outros"         :type "expense" :icon "📋"}])

(defn seed-default-categories! [user-id]
  (doseq [cat default-categories]
    (db/execute-one!
     (-> (h/insert-into :categories)
         (h/values [{:user-id user-id
                     :name (:name cat)
                     :type (:type cat)
                     :icon (:icon cat)
                     :is-default true}])
         sql/format))))

(defn find-user-by-email [email]
  (db/execute-one!
   (-> (h/select :*)
       (h/from :users)
       (h/where [:= :email email])
       sql/format)))

(defn register! [{:keys [email password name]}]
  (when (find-user-by-email email)
    (throw (ex-info "Email already registered"
                    {:status 409
                     :body {:error "conflict"
                            :message "Email already registered"}})))
  (let [hashed (hash-password password)
        user (db/execute-one!
              (-> (h/insert-into :users)
                  (h/values [{:email email
                              :password hashed
                              :name name}])
                  (h/returning :id :email :name :created-at)
                  sql/format))
        secret (:jwt-secret (config))
        token (generate-token user secret)]
    (seed-default-categories! (:id user))
    {:user (dissoc user :password)
     :token token}))

(defn login! [{:keys [email password]}]
  (let [user (find-user-by-email email)]
    (when-not (and user (check-password password (:password user)))
      (throw (ex-info "Invalid credentials"
                      {:status 401
                       :body {:error "unauthorized"
                              :message "Invalid email or password"}})))
    (let [secret (:jwt-secret (config))
          token (generate-token user secret)]
      {:user (dissoc user :password)
       :token token})))
```

- [ ] **Step 3: Create `src/cointrail/auth/handlers.clj`**

```clojure
(ns cointrail.auth.handlers
  (:require [cointrail.auth.service :as service]
            [cointrail.auth.spec :as auth-spec]))

(defn register-handler [request]
  (let [body (:body request)]
    (if-let [errors (auth-spec/validate ::auth-spec/register-request body)]
      {:status 400 :body errors}
      (try
        (let [result (service/register! body)]
          {:status 201 :body result})
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn login-handler [request]
  (let [body (:body request)]
    (if-let [errors (auth-spec/validate ::auth-spec/login-request body)]
      {:status 400 :body errors}
      (try
        (let [result (service/login! body)]
          {:status 200 :body result})
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))
```

- [ ] **Step 4: Update `src/cointrail/routes.clj`**

```clojure
(ns cointrail.routes
  (:require [compojure.core :refer [defroutes GET POST context]]
            [compojure.route :as route]
            [cointrail.db :as db]
            [cointrail.auth.handlers :as auth]))

(defn health-handler [_]
  (let [db-ok? (try
                 (db/execute-one! ["SELECT 1 AS ok"])
                 true
                 (catch Exception _ false))]
    {:status (if db-ok? 200 503)
     :body {:status (if db-ok? "ok" "degraded")
            :database (if db-ok? "connected" "disconnected")
            :version "0.1.0"}}))

(defroutes app-routes
  (context "/api" []
    (GET "/health" [] health-handler)
    (context "/auth" []
      (POST "/register" [] auth/register-handler)
      (POST "/login" [] auth/login-handler)))
  (route/not-found {:status 404
                    :body {:error "not_found" :message "Resource not found"}}))
```

- [ ] **Step 5: Write auth tests**

`test/cointrail/auth/handlers_test.clj`:
```clojure
(ns cointrail.auth.handlers-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (db/execute! ["DELETE FROM transactions"])
    (db/execute! ["DELETE FROM categories"])
    (db/execute! ["DELETE FROM users"])
    (f)))

(use-fixtures :each db-fixture)

(defn json-request [method url body]
  (-> (mock/request method url)
      (mock/content-type "application/json")
      (mock/body (json/generate-string body))))

(defn parse-body [response]
  (json/parse-string (slurp (:body response)) true))

(deftest register-test
  (let [app (create-app (config))]
    (testing "successful registration"
      (let [response (app (json-request :post "/api/auth/register"
                                        {:email "test@example.com"
                                         :password "password123"
                                         :name "Test User"}))
            body (parse-body response)]
        (is (= 201 (:status response)))
        (is (contains? body :token))
        (is (= "test@example.com" (get-in body [:user :email])))
        (is (not (contains? (:user body) :password)))))

    (testing "duplicate email returns 409"
      (let [response (app (json-request :post "/api/auth/register"
                                        {:email "test@example.com"
                                         :password "password123"
                                         :name "Another User"}))]
        (is (= 409 (:status response)))))

    (testing "invalid email returns 400"
      (let [response (app (json-request :post "/api/auth/register"
                                        {:email "not-an-email"
                                         :password "password123"
                                         :name "Test"}))]
        (is (= 400 (:status response)))))

    (testing "short password returns 400"
      (let [response (app (json-request :post "/api/auth/register"
                                        {:email "short@example.com"
                                         :password "short"
                                         :name "Test"}))]
        (is (= 400 (:status response)))))))

(deftest login-test
  (let [app (create-app (config))]
    (testing "setup: register user"
      (app (json-request :post "/api/auth/register"
                         {:email "login@example.com"
                          :password "password123"
                          :name "Login User"})))

    (testing "successful login"
      (let [response (app (json-request :post "/api/auth/login"
                                        {:email "login@example.com"
                                         :password "password123"}))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (contains? body :token))
        (is (= "login@example.com" (get-in body [:user :email])))))

    (testing "wrong password returns 401"
      (let [response (app (json-request :post "/api/auth/login"
                                        {:email "login@example.com"
                                         :password "wrongpassword"}))]
        (is (= 401 (:status response)))))

    (testing "non-existent email returns 401"
      (let [response (app (json-request :post "/api/auth/login"
                                        {:email "nobody@example.com"
                                         :password "password123"}))]
        (is (= 401 (:status response)))))))

(deftest seed-categories-test
  (let [app (create-app (config))]
    (testing "registration creates default categories"
      (app (json-request :post "/api/auth/register"
                         {:email "seed@example.com"
                          :password "password123"
                          :name "Seed User"}))
      (let [categories (db/execute! ["SELECT * FROM categories WHERE is_default = true"])]
        (is (= 11 (count categories)))))))
```

- [ ] **Step 6: Run tests**

Run: `lein test cointrail.auth.handlers-test`
Expected: PASS — all register, login, and seed category tests pass.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: auth endpoints with JWT registration, login, and seed categories"
```

---

### Task 3: Categories CRUD

**Files:**
- Create: `src/cointrail/categories/spec.clj`
- Create: `src/cointrail/categories/repository.clj`
- Create: `src/cointrail/categories/service.clj`
- Create: `src/cointrail/categories/handlers.clj`
- Modify: `src/cointrail/routes.clj`
- Create: `test/cointrail/categories/handlers_test.clj`

**Interfaces:**
- Consumes: `cointrail.db/execute!`, `cointrail.db/execute-one!`, `cointrail.middleware/require-auth`, auth registration for test setup
- Produces:
  - `cointrail.categories.repository/find-by-user` — `(find-by-user user-id)` returns list of category maps
  - `cointrail.categories.repository/find-by-id` — `(find-by-id id)` returns category map or nil
  - `cointrail.categories.repository/insert!` — `(insert! category-map)` returns inserted category
  - `cointrail.categories.repository/update!` — `(update! id updates)` returns updated category
  - `cointrail.categories.repository/delete!` — `(delete! id)` deletes category

- [ ] **Step 1: Create `src/cointrail/categories/spec.clj`**

```clojure
(ns cointrail.categories.spec
  (:require [clojure.spec.alpha :as s]
            [cointrail.auth.spec :refer [validate]]))

(s/def ::name (s/and string? #(>= (count %) 1) #(<= (count %) 50)))
(s/def ::type #{"income" "expense"})
(s/def ::icon (s/nilable (s/and string? #(<= (count %) 30))))

(s/def ::create-category (s/keys :req-un [::name ::type] :opt-un [::icon]))
(s/def ::update-category (s/keys :opt-un [::name ::type ::icon]))
```

- [ ] **Step 2: Create `src/cointrail/categories/repository.clj`**

```clojure
(ns cointrail.categories.repository
  (:require [cointrail.db :as db]
            [honey.sql :as sql]
            [honey.sql.helpers :as h]))

(defn find-by-user [user-id]
  (db/execute!
   (-> (h/select :*)
       (h/from :categories)
       (h/where [:= :user-id user-id])
       (h/order-by [:type :asc] [:name :asc])
       sql/format)))

(defn find-by-id [id]
  (db/execute-one!
   (-> (h/select :*)
       (h/from :categories)
       (h/where [:= :id id])
       sql/format)))

(defn insert! [category]
  (db/execute-one!
   (-> (h/insert-into :categories)
       (h/values [category])
       (h/returning :*)
       sql/format)))

(defn update! [id updates]
  (db/execute-one!
   (-> (h/update :categories)
       (h/set updates)
       (h/where [:= :id id])
       (h/returning :*)
       sql/format)))

(defn delete! [id]
  (db/execute-one!
   (-> (h/delete-from :categories)
       (h/where [:= :id id])
       sql/format)))
```

- [ ] **Step 3: Create `src/cointrail/categories/service.clj`**

```clojure
(ns cointrail.categories.service
  (:require [cointrail.categories.repository :as repo]))

(defn list-categories [user-id]
  (repo/find-by-user user-id))

(defn create-category! [user-id data]
  (repo/insert! (assoc data :user-id user-id :is-default false)))

(defn update-category! [user-id id data]
  (let [category (repo/find-by-id id)]
    (cond
      (nil? category)
      (throw (ex-info "Not found" {:status 404
                                   :body {:error "not_found"
                                          :message "Category not found"}}))
      (not= (:user-id category) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Category belongs to another user"}}))
      (:is-default category)
      (throw (ex-info "Cannot modify default" {:status 400
                                               :body {:error "validation_failed"
                                                      :message "Cannot modify default categories"}}))
      :else
      (repo/update! id data))))

(defn delete-category! [user-id id]
  (let [category (repo/find-by-id id)]
    (cond
      (nil? category)
      (throw (ex-info "Not found" {:status 404
                                   :body {:error "not_found"
                                          :message "Category not found"}}))
      (not= (:user-id category) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Category belongs to another user"}}))
      (:is-default category)
      (throw (ex-info "Cannot delete default" {:status 400
                                               :body {:error "validation_failed"
                                                      :message "Cannot delete default categories"}}))
      :else
      (do (repo/delete! id) nil))))
```

- [ ] **Step 4: Create `src/cointrail/categories/handlers.clj`**

```clojure
(ns cointrail.categories.handlers
  (:require [cointrail.categories.service :as service]
            [cointrail.categories.spec :as cat-spec]
            [cointrail.auth.spec :refer [validate]]))

(defn- user-id [request]
  (java.util.UUID/fromString (get-in request [:identity :user-id])))

(defn list-handler [request]
  {:status 200
   :body {:data (service/list-categories (user-id request))}})

(defn create-handler [request]
  (let [body (:body request)]
    (if-let [errors (validate ::cat-spec/create-category body)]
      {:status 400 :body errors}
      (try
        {:status 201
         :body (service/create-category! (user-id request) body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn update-handler [request]
  (let [id (java.util.UUID/fromString (get-in request [:params :id]))
        body (:body request)]
    (if-let [errors (validate ::cat-spec/update-category body)]
      {:status 400 :body errors}
      (try
        {:status 200
         :body (service/update-category! (user-id request) id body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn delete-handler [request]
  (let [id (java.util.UUID/fromString (get-in request [:params :id]))]
    (try
      (service/delete-category! (user-id request) id)
      {:status 204 :body nil}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))))
```

- [ ] **Step 5: Update `src/cointrail/routes.clj` — add category routes**

Add to requires: `[cointrail.categories.handlers :as categories]`, `[cointrail.middleware :as mw]`

Add inside the `/api` context, after auth routes:
```clojure
(context "/categories" []
  (mw/require-auth
   (compojure.core/routes
    (GET "/" [] categories/list-handler)
    (POST "/" [] categories/create-handler)
    (PUT "/:id" [] categories/update-handler)
    (DELETE "/:id" [] categories/delete-handler))))
```

- [ ] **Step 6: Write category tests**

`test/cointrail/categories/handlers_test.clj`:
```clojure
(ns cointrail.categories.handlers-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (db/execute! ["DELETE FROM transactions"])
    (db/execute! ["DELETE FROM categories"])
    (db/execute! ["DELETE FROM users"])
    (f)))

(use-fixtures :each db-fixture)

(defn json-request [method url body]
  (-> (mock/request method url)
      (mock/content-type "application/json")
      (mock/body (json/generate-string body))))

(defn parse-body [response]
  (json/parse-string (slurp (:body response)) true))

(defn auth-header [request token]
  (mock/header request "Authorization" (str "Bearer " token)))

(defn register-and-get-token [app]
  (let [response (app (json-request :post "/api/auth/register"
                                    {:email (str (java.util.UUID/randomUUID) "@test.com")
                                     :password "password123"
                                     :name "Test User"}))
        body (parse-body response)]
    (:token body)))

(deftest list-categories-test
  (let [app (create-app (config))
        token (register-and-get-token app)]
    (testing "lists default categories after registration"
      (let [response (app (auth-header (mock/request :get "/api/categories") token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 11 (count (:data body))))))

    (testing "unauthenticated returns 401"
      (let [response (app (mock/request :get "/api/categories"))]
        (is (= 401 (:status response)))))))

(deftest create-category-test
  (let [app (create-app (config))
        token (register-and-get-token app)]
    (testing "creates custom category"
      (let [response (app (auth-header
                           (json-request :post "/api/categories"
                                        {:name "Pets" :type "expense" :icon "🐕"})
                           token))
            body (parse-body response)]
        (is (= 201 (:status response)))
        (is (= "Pets" (:name body)))
        (is (= false (:is-default body)))))

    (testing "invalid type returns 400"
      (let [response (app (auth-header
                           (json-request :post "/api/categories"
                                        {:name "Bad" :type "invalid"})
                           token))]
        (is (= 400 (:status response)))))))

(deftest update-delete-category-test
  (let [app (create-app (config))
        token (register-and-get-token app)
        create-resp (app (auth-header
                          (json-request :post "/api/categories"
                                       {:name "Custom" :type "expense"})
                          token))
        custom-id (:id (parse-body create-resp))]

    (testing "updates custom category"
      (let [response (app (auth-header
                           (json-request :put (str "/api/categories/" custom-id)
                                        {:name "Updated"})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= "Updated" (:name body)))))

    (testing "deletes custom category"
      (let [response (app (auth-header
                           (mock/request :delete (str "/api/categories/" custom-id))
                           token))]
        (is (= 204 (:status response)))))

    (testing "cannot modify default category"
      (let [cats-resp (app (auth-header (mock/request :get "/api/categories") token))
            default-id (:id (first (filter :is-default (:data (parse-body cats-resp)))))
            response (app (auth-header
                           (json-request :put (str "/api/categories/" default-id)
                                        {:name "Hacked"})
                           token))]
        (is (= 400 (:status response)))))))
```

- [ ] **Step 7: Run tests**

Run: `lein test cointrail.categories.handlers-test`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat: categories CRUD with default category protection"
```

---

### Task 4: Transactions CRUD with Filtering and Pagination

**Files:**
- Create: `src/cointrail/transactions/spec.clj`
- Create: `src/cointrail/transactions/repository.clj`
- Create: `src/cointrail/transactions/service.clj`
- Create: `src/cointrail/transactions/handlers.clj`
- Modify: `src/cointrail/routes.clj`
- Create: `test/cointrail/transactions/handlers_test.clj`

**Interfaces:**
- Consumes: `cointrail.db/execute!`, `cointrail.db/execute-one!`, `cointrail.middleware/require-auth`, `cointrail.categories.repository/find-by-id`
- Produces:
  - `cointrail.transactions.repository/find-by-user` — `(find-by-user user-id filters)` returns `{:data list :total int}`
  - `cointrail.transactions.repository/find-by-id` — `(find-by-id id)` returns transaction map or nil
  - `cointrail.transactions.repository/insert!` — `(insert! txn-map)` returns inserted transaction
  - `cointrail.transactions.repository/update!` — `(update! id updates)` returns updated transaction
  - `cointrail.transactions.repository/delete!` — `(delete! id)` deletes transaction

- [ ] **Step 1: Create `src/cointrail/transactions/spec.clj`**

```clojure
(ns cointrail.transactions.spec
  (:require [clojure.spec.alpha :as s]))

(s/def ::type #{"income" "expense"})
(s/def ::amount (s/and number? pos?))
(s/def ::category-id (s/nilable string?))
(s/def ::date (s/and string? #(re-matches #"\d{4}-\d{2}-\d{2}" %)))
(s/def ::description (s/nilable (s/and string? #(<= (count %) 255))))
(s/def ::notes (s/nilable string?))

(s/def ::create-transaction (s/keys :req-un [::type ::amount]
                                    :opt-un [::category-id ::date ::description ::notes]))
(s/def ::update-transaction (s/keys :opt-un [::type ::amount ::category-id ::date ::description ::notes]))
```

- [ ] **Step 2: Create `src/cointrail/transactions/repository.clj`**

```clojure
(ns cointrail.transactions.repository
  (:require [cointrail.db :as db]
            [honey.sql :as sql]
            [honey.sql.helpers :as h]))

(defn- apply-filters [query {:keys [type category-id from to]}]
  (cond-> query
    type        (h/where [:= :t.type type])
    category-id (h/where [:= :t.category-id (java.util.UUID/fromString category-id)])
    from        (h/where [:>= :t.date (java.time.LocalDate/parse from)])
    to          (h/where [:<= :t.date (java.time.LocalDate/parse to)])))

(defn- valid-sort-field [s]
  (get {"date" :t.date "amount" :t.amount "created_at" :t.created-at} s :t.date))

(defn count-by-user [user-id filters]
  (:count
   (db/execute-one!
    (-> (h/select [:%count.* :count])
        (h/from [:transactions :t])
        (h/where [:= :t.user-id user-id])
        (apply-filters filters)
        sql/format))))

(defn find-by-user [user-id {:keys [page per-page sort order] :as filters
                             :or {page 1 per-page 20 sort "date" order "desc"}}]
  (let [page (max 1 page)
        per-page (min 100 (max 1 per-page))
        offset (* (dec page) per-page)
        total (count-by-user user-id filters)
        sort-field (valid-sort-field sort)
        sort-dir (if (= order "asc") :asc :desc)
        data (db/execute!
              (-> (h/select :t.* [:c.name :category-name] [:c.icon :category-icon])
                  (h/from [:transactions :t])
                  (h/left-join [:categories :c] [:= :t.category-id :c.id])
                  (h/where [:= :t.user-id user-id])
                  (apply-filters filters)
                  (h/order-by [sort-field sort-dir])
                  (h/limit per-page)
                  (h/offset offset)
                  sql/format))]
    {:data data
     :meta {:page page
            :per-page per-page
            :total total
            :total-pages (int (Math/ceil (/ (double total) per-page)))}}))

(defn find-by-id [id]
  (db/execute-one!
   (-> (h/select :t.* [:c.name :category-name] [:c.icon :category-icon])
       (h/from [:transactions :t])
       (h/left-join [:categories :c] [:= :t.category-id :c.id])
       (h/where [:= :t.id id])
       sql/format)))

(defn insert! [txn]
  (let [result (db/execute-one!
                (-> (h/insert-into :transactions)
                    (h/values [txn])
                    (h/returning :*)
                    sql/format))]
    (find-by-id (:id result))))

(defn update! [id updates]
  (let [updates (assoc updates :updated-at :%now)]
    (db/execute-one!
     (-> (h/update :transactions)
         (h/set updates)
         (h/where [:= :id id])
         (h/returning :*)
         sql/format))
    (find-by-id id)))

(defn delete! [id]
  (db/execute-one!
   (-> (h/delete-from :transactions)
       (h/where [:= :id id])
       sql/format)))
```

- [ ] **Step 3: Create `src/cointrail/transactions/service.clj`**

```clojure
(ns cointrail.transactions.service
  (:require [cointrail.transactions.repository :as repo]))

(defn list-transactions [user-id filters]
  (repo/find-by-user user-id filters))

(defn get-transaction [user-id id]
  (let [txn (repo/find-by-id id)]
    (cond
      (nil? txn)
      (throw (ex-info "Not found" {:status 404
                                   :body {:error "not_found"
                                          :message "Transaction not found"}}))
      (not= (:user-id txn) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Transaction belongs to another user"}}))
      :else txn)))

(defn create-transaction! [user-id data]
  (let [txn-data (-> data
                     (assoc :user-id user-id)
                     (cond-> (:category-id data)
                       (update :category-id #(java.util.UUID/fromString %))
                       (:date data)
                       (update :date #(java.time.LocalDate/parse %))))]
    (repo/insert! txn-data)))

(defn update-transaction! [user-id id data]
  (get-transaction user-id id)
  (let [updates (cond-> data
                  (:category-id data) (update :category-id #(java.util.UUID/fromString %))
                  (:date data) (update :date #(java.time.LocalDate/parse %)))]
    (repo/update! id updates)))

(defn delete-transaction! [user-id id]
  (get-transaction user-id id)
  (repo/delete! id)
  nil)
```

- [ ] **Step 4: Create `src/cointrail/transactions/handlers.clj`**

```clojure
(ns cointrail.transactions.handlers
  (:require [cointrail.transactions.service :as service]
            [cointrail.transactions.spec :as txn-spec]
            [cointrail.auth.spec :refer [validate]]))

(defn- user-id [request]
  (java.util.UUID/fromString (get-in request [:identity :user-id])))

(defn- parse-int [s default]
  (if s (try (Integer/parseInt s) (catch Exception _ default)) default))

(defn- extract-filters [params]
  {:type (:type params)
   :category-id (:category_id params)
   :from (:from params)
   :to (:to params)
   :sort (or (:sort params) "date")
   :order (or (:order params) "desc")
   :page (parse-int (:page params) 1)
   :per-page (parse-int (:per_page params) 20)})

(defn list-handler [request]
  (let [filters (extract-filters (:params request))
        result (service/list-transactions (user-id request) filters)]
    {:status 200 :body result}))

(defn create-handler [request]
  (let [body (:body request)]
    (if-let [errors (validate ::txn-spec/create-transaction body)]
      {:status 400 :body errors}
      (try
        {:status 201 :body (service/create-transaction! (user-id request) body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn get-handler [request]
  (let [id (java.util.UUID/fromString (get-in request [:params :id]))]
    (try
      {:status 200 :body (service/get-transaction (user-id request) id)}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))))

(defn update-handler [request]
  (let [id (java.util.UUID/fromString (get-in request [:params :id]))
        body (:body request)]
    (if-let [errors (validate ::txn-spec/update-transaction body)]
      {:status 400 :body errors}
      (try
        {:status 200 :body (service/update-transaction! (user-id request) id body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn delete-handler [request]
  (let [id (java.util.UUID/fromString (get-in request [:params :id]))]
    (try
      (service/delete-transaction! (user-id request) id)
      {:status 204 :body nil}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))))
```

- [ ] **Step 5: Update `src/cointrail/routes.clj` — add transaction routes**

Add to requires: `[cointrail.transactions.handlers :as transactions]`

Add inside `/api` context:
```clojure
(context "/transactions" []
  (mw/require-auth
   (compojure.core/routes
    (GET "/" [] transactions/list-handler)
    (POST "/" [] transactions/create-handler)
    (GET "/:id" [] transactions/get-handler)
    (PUT "/:id" [] transactions/update-handler)
    (DELETE "/:id" [] transactions/delete-handler))))
```

- [ ] **Step 6: Write transaction tests**

`test/cointrail/transactions/handlers_test.clj`:
```clojure
(ns cointrail.transactions.handlers-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (db/execute! ["DELETE FROM transactions"])
    (db/execute! ["DELETE FROM categories"])
    (db/execute! ["DELETE FROM users"])
    (f)))

(use-fixtures :each db-fixture)

(defn json-request [method url body]
  (-> (mock/request method url)
      (mock/content-type "application/json")
      (mock/body (json/generate-string body))))

(defn parse-body [response]
  (json/parse-string (slurp (:body response)) true))

(defn auth-header [request token]
  (mock/header request "Authorization" (str "Bearer " token)))

(defn register-and-get-token [app]
  (let [response (app (json-request :post "/api/auth/register"
                                    {:email (str (java.util.UUID/randomUUID) "@test.com")
                                     :password "password123"
                                     :name "Test User"}))
        body (parse-body response)]
    (:token body)))

(deftest create-transaction-test
  (let [app (create-app (config))
        token (register-and-get-token app)]
    (testing "creates a transaction"
      (let [response (app (auth-header
                           (json-request :post "/api/transactions"
                                        {:type "expense"
                                         :amount 42.50
                                         :description "Lunch"
                                         :date "2026-09-15"})
                           token))
            body (parse-body response)]
        (is (= 201 (:status response)))
        (is (= "expense" (:type body)))
        (is (= 42.50 (:amount body)))))

    (testing "invalid amount returns 400"
      (let [response (app (auth-header
                           (json-request :post "/api/transactions"
                                        {:type "expense" :amount -10})
                           token))]
        (is (= 400 (:status response)))))))

(deftest list-transactions-test
  (let [app (create-app (config))
        token (register-and-get-token app)]
    (dotimes [i 25]
      (app (auth-header
            (json-request :post "/api/transactions"
                         {:type "expense" :amount (inc i)
                          :date "2026-09-15"})
            token)))

    (testing "returns paginated results"
      (let [response (app (auth-header
                           (mock/request :get "/api/transactions?page=1&per_page=10")
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 10 (count (:data body))))
        (is (= 25 (get-in body [:meta :total])))
        (is (= 3 (get-in body [:meta :total-pages])))))

    (testing "filters by type"
      (app (auth-header
            (json-request :post "/api/transactions"
                         {:type "income" :amount 1000 :date "2026-09-15"})
            token))
      (let [response (app (auth-header
                           (mock/request :get "/api/transactions?type=income")
                           token))
            body (parse-body response)]
        (is (every? #(= "income" (:type %)) (:data body)))))

    (testing "filters by date range"
      (let [response (app (auth-header
                           (mock/request :get "/api/transactions?from=2026-09-15&to=2026-09-15")
                           token))
            body (parse-body response)]
        (is (pos? (get-in body [:meta :total])))))))

(deftest get-update-delete-transaction-test
  (let [app (create-app (config))
        token (register-and-get-token app)
        create-resp (app (auth-header
                          (json-request :post "/api/transactions"
                                       {:type "expense" :amount 50
                                        :description "Test" :date "2026-09-15"})
                          token))
        txn-id (:id (parse-body create-resp))]

    (testing "gets a transaction by id"
      (let [response (app (auth-header
                           (mock/request :get (str "/api/transactions/" txn-id))
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 50.0 (:amount body)))))

    (testing "updates a transaction"
      (let [response (app (auth-header
                           (json-request :put (str "/api/transactions/" txn-id)
                                        {:amount 75 :description "Updated"})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 75.0 (:amount body)))
        (is (= "Updated" (:description body)))))

    (testing "deletes a transaction"
      (let [response (app (auth-header
                           (mock/request :delete (str "/api/transactions/" txn-id))
                           token))]
        (is (= 204 (:status response)))))

    (testing "deleted transaction returns 404"
      (let [response (app (auth-header
                           (mock/request :get (str "/api/transactions/" txn-id))
                           token))]
        (is (= 404 (:status response)))))))
```

- [ ] **Step 7: Run tests**

Run: `lein test cointrail.transactions.handlers-test`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat: transactions CRUD with filtering, sorting, and pagination"
```

---

### Task 5: Reports — Summary and By-Category

**Files:**
- Create: `src/cointrail/reports/repository.clj`
- Create: `src/cointrail/reports/service.clj`
- Create: `src/cointrail/reports/handlers.clj`
- Modify: `src/cointrail/routes.clj`
- Create: `test/cointrail/reports/handlers_test.clj`

**Interfaces:**
- Consumes: `cointrail.db/execute!`, `cointrail.db/execute-one!`, `cointrail.middleware/require-auth`
- Produces:
  - `cointrail.reports.repository/summary` — `(summary user-id from to)` returns `{:total-income, :total-expense, :transaction-count}`
  - `cointrail.reports.repository/by-category` — `(by-category user-id from to type)` returns list of `{:category, :total, :count}`

- [ ] **Step 1: Create `src/cointrail/reports/repository.clj`**

```clojure
(ns cointrail.reports.repository
  (:require [cointrail.db :as db]
            [honey.sql :as sql]
            [honey.sql.helpers :as h])
  (:import [java.time LocalDate YearMonth]))

(defn- current-month-range []
  (let [ym (YearMonth/now)
        from (.atDay ym 1)
        to (.atEndOfMonth ym)]
    [from to]))

(defn- parse-date-range [from-str to-str]
  (let [[default-from default-to] (current-month-range)
        from (if from-str (LocalDate/parse from-str) default-from)
        to (if to-str (LocalDate/parse to-str) default-to)]
    [from to]))

(defn summary [user-id from-str to-str]
  (let [[from to] (parse-date-range from-str to-str)
        income (or (:total
                    (db/execute-one!
                     (-> (h/select [:%sum.amount :total])
                         (h/from :transactions)
                         (h/where [:= :user-id user-id]
                                  [:= :type "income"]
                                  [:>= :date from]
                                  [:<= :date to])
                         sql/format)))
                   0M)
        expense (or (:total
                     (db/execute-one!
                      (-> (h/select [:%sum.amount :total])
                          (h/from :transactions)
                          (h/where [:= :user-id user-id]
                                   [:= :type "expense"]
                                   [:>= :date from]
                                   [:<= :date to])
                          sql/format)))
                    0M)
        tx-count (:count
                  (db/execute-one!
                   (-> (h/select [:%count.* :count])
                       (h/from :transactions)
                       (h/where [:= :user-id user-id]
                                [:>= :date from]
                                [:<= :date to])
                       sql/format)))]
    {:total-income income
     :total-expense expense
     :balance (- income expense)
     :transaction-count tx-count}))

(defn by-category [user-id from-str to-str type-filter]
  (let [[from to] (parse-date-range from-str to-str)
        type-filter (or type-filter "expense")
        rows (db/execute!
              (-> (h/select [:c.name :category]
                            [:c.icon :icon]
                            [:%sum.t.amount :total]
                            [:%count.* :count])
                  (h/from [:transactions :t])
                  (h/join [:categories :c] [:= :t.category-id :c.id])
                  (h/where [:= :t.user-id user-id]
                           [:= :t.type type-filter]
                           [:>= :t.date from]
                           [:<= :t.date to])
                  (h/group-by :c.name :c.icon)
                  (h/order-by [:%sum.t.amount :desc])
                  sql/format))
        grand-total (reduce + 0M (map :total rows))]
    (mapv (fn [row]
            (assoc row :percentage
                   (if (pos? grand-total)
                     (double (* 100 (/ (:total row) grand-total)))
                     0.0)))
          rows)))
```

- [ ] **Step 2: Create `src/cointrail/reports/service.clj`**

```clojure
(ns cointrail.reports.service
  (:require [cointrail.reports.repository :as repo]))

(defn summary [user-id from to]
  (repo/summary user-id from to))

(defn by-category [user-id from to type]
  (repo/by-category user-id from to type))
```

- [ ] **Step 3: Create `src/cointrail/reports/handlers.clj`**

```clojure
(ns cointrail.reports.handlers
  (:require [cointrail.reports.service :as service]))

(defn- user-id [request]
  (java.util.UUID/fromString (get-in request [:identity :user-id])))

(defn summary-handler [request]
  (let [params (:params request)
        result (service/summary (user-id request) (:from params) (:to params))]
    {:status 200 :body result}))

(defn by-category-handler [request]
  (let [params (:params request)
        result (service/by-category (user-id request)
                                   (:from params) (:to params)
                                   (:type params))]
    {:status 200 :body result}))
```

- [ ] **Step 4: Update `src/cointrail/routes.clj` — add report routes**

Add to requires: `[cointrail.reports.handlers :as reports]`

Add inside `/api` context:
```clojure
(context "/reports" []
  (mw/require-auth
   (compojure.core/routes
    (GET "/summary" [] reports/summary-handler)
    (GET "/by-category" [] reports/by-category-handler))))
```

- [ ] **Step 5: Write report tests**

`test/cointrail/reports/handlers_test.clj`:
```clojure
(ns cointrail.reports.handlers-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (db/execute! ["DELETE FROM transactions"])
    (db/execute! ["DELETE FROM categories"])
    (db/execute! ["DELETE FROM users"])
    (f)))

(use-fixtures :each db-fixture)

(defn json-request [method url body]
  (-> (mock/request method url)
      (mock/content-type "application/json")
      (mock/body (json/generate-string body))))

(defn parse-body [response]
  (json/parse-string (slurp (:body response)) true))

(defn auth-header [request token]
  (mock/header request "Authorization" (str "Bearer " token)))

(defn setup-user-with-transactions [app]
  (let [reg-resp (app (json-request :post "/api/auth/register"
                                    {:email (str (java.util.UUID/randomUUID) "@test.com")
                                     :password "password123"
                                     :name "Report User"}))
        token (:token (parse-body reg-resp))
        cats-resp (app (auth-header (mock/request :get "/api/categories") token))
        categories (:data (parse-body cats-resp))
        salary-id (:id (first (filter #(= "Salário" (:name %)) categories)))
        food-id (:id (first (filter #(= "Alimentação" (:name %)) categories)))
        transport-id (:id (first (filter #(= "Transporte" (:name %)) categories)))]
    (app (auth-header (json-request :post "/api/transactions"
                                    {:type "income" :amount 5000
                                     :category-id salary-id :date "2026-09-01"})
                      token))
    (app (auth-header (json-request :post "/api/transactions"
                                    {:type "expense" :amount 800
                                     :category-id food-id :date "2026-09-10"})
                      token))
    (app (auth-header (json-request :post "/api/transactions"
                                    {:type "expense" :amount 200
                                     :category-id transport-id :date "2026-09-15"})
                      token))
    token))

(deftest summary-test
  (let [app (create-app (config))
        token (setup-user-with-transactions app)]
    (testing "returns correct summary for date range"
      (let [response (app (auth-header
                           (mock/request :get "/api/reports/summary?from=2026-09-01&to=2026-09-30")
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 5000.0 (:total-income body)))
        (is (= 1000.0 (:total-expense body)))
        (is (= 4000.0 (:balance body)))
        (is (= 3 (:transaction-count body)))))

    (testing "empty range returns zeros"
      (let [response (app (auth-header
                           (mock/request :get "/api/reports/summary?from=2025-01-01&to=2025-01-31")
                           token))
            body (parse-body response)]
        (is (= 0 (:transaction-count body)))))))

(deftest by-category-test
  (let [app (create-app (config))
        token (setup-user-with-transactions app)]
    (testing "returns expense breakdown by category"
      (let [response (app (auth-header
                           (mock/request :get "/api/reports/by-category?from=2026-09-01&to=2026-09-30&type=expense")
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 2 (count body)))
        (let [food (first (filter #(= "Alimentação" (:category %)) body))]
          (is (= 800.0 (:total food)))
          (is (= 80.0 (:percentage food))))))))
```

- [ ] **Step 6: Run tests**

Run: `lein test cointrail.reports.handlers-test`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: reports endpoints with summary and by-category breakdown"
```

---

### Task 6: Swagger UI and API Documentation

**Files:**
- Create: `src/cointrail/swagger.clj`
- Modify: `src/cointrail/routes.clj`
- Create: `test/cointrail/swagger_test.clj`

**Interfaces:**
- Consumes: all route definitions from `cointrail.routes`
- Produces: Swagger UI served at `/swagger-ui`, OpenAPI spec at `/api/swagger.json`

- [ ] **Step 1: Create `src/cointrail/swagger.clj`**

```clojure
(ns cointrail.swagger
  (:require [ring.swagger.swagger-ui :refer [wrap-swagger-ui]]
            [cheshire.core :as json]))

(def api-spec
  {:swagger "2.0"
   :info {:title "CoinTrail API"
          :description "Personal Finance REST API built with Clojure"
          :version "0.1.0"}
   :basePath "/api"
   :schemes ["http"]
   :consumes ["application/json"]
   :produces ["application/json"]
   :securityDefinitions {:Bearer {:type "apiKey"
                                  :name "Authorization"
                                  :in "header"
                                  :description "JWT token. Format: Bearer <token>"}}
   :paths
   {"/auth/register"
    {:post {:tags ["Auth"]
            :summary "Register a new user"
            :parameters [{:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["email" "password" "name"]
                                   :properties {:email {:type "string" :format "email"}
                                                :password {:type "string" :minLength 8}
                                                :name {:type "string"}}}}]
            :responses {201 {:description "User created with JWT token"}
                        400 {:description "Validation error"}
                        409 {:description "Email already registered"}}}}
    "/auth/login"
    {:post {:tags ["Auth"]
            :summary "Login"
            :parameters [{:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["email" "password"]
                                   :properties {:email {:type "string"}
                                                :password {:type "string"}}}}]
            :responses {200 {:description "JWT token"}
                        401 {:description "Invalid credentials"}}}}
    "/categories"
    {:get {:tags ["Categories"] :summary "List user categories"
           :security [{:Bearer []}]
           :responses {200 {:description "List of categories"}}}
     :post {:tags ["Categories"] :summary "Create custom category"
            :security [{:Bearer []}]
            :parameters [{:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["name" "type"]
                                   :properties {:name {:type "string"}
                                                :type {:type "string" :enum ["income" "expense"]}
                                                :icon {:type "string"}}}}]
            :responses {201 {:description "Category created"}}}}
    "/categories/{id}"
    {:put {:tags ["Categories"] :summary "Update category (custom only)"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}
                        {:in "body" :name "body"
                         :schema {:type "object"
                                  :properties {:name {:type "string"}
                                               :type {:type "string" :enum ["income" "expense"]}
                                               :icon {:type "string"}}}}]
           :responses {200 {:description "Updated"}}}
     :delete {:tags ["Categories"] :summary "Delete category (custom only)"
              :security [{:Bearer []}]
              :parameters [{:in "path" :name "id" :type "string" :required true}]
              :responses {204 {:description "Deleted"}}}}
    "/transactions"
    {:get {:tags ["Transactions"] :summary "List transactions with filters"
           :security [{:Bearer []}]
           :parameters [{:in "query" :name "type" :type "string" :enum ["income" "expense"]}
                        {:in "query" :name "category_id" :type "string"}
                        {:in "query" :name "from" :type "string" :format "date"}
                        {:in "query" :name "to" :type "string" :format "date"}
                        {:in "query" :name "sort" :type "string" :enum ["date" "amount" "created_at"]}
                        {:in "query" :name "order" :type "string" :enum ["asc" "desc"]}
                        {:in "query" :name "page" :type "integer" :default 1}
                        {:in "query" :name "per_page" :type "integer" :default 20}]
           :responses {200 {:description "Paginated transactions"}}}
     :post {:tags ["Transactions"] :summary "Create transaction"
            :security [{:Bearer []}]
            :parameters [{:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["type" "amount"]
                                   :properties {:type {:type "string" :enum ["income" "expense"]}
                                                :amount {:type "number" :minimum 0.01}
                                                :category_id {:type "string"}
                                                :date {:type "string" :format "date"}
                                                :description {:type "string"}
                                                :notes {:type "string"}}}}]
            :responses {201 {:description "Transaction created"}}}}
    "/transactions/{id}"
    {:get {:tags ["Transactions"] :summary "Get transaction detail"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}]
           :responses {200 {:description "Transaction"}}}
     :put {:tags ["Transactions"] :summary "Update transaction"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}
                        {:in "body" :name "body"
                         :schema {:type "object"
                                  :properties {:type {:type "string" :enum ["income" "expense"]}
                                               :amount {:type "number"}
                                               :category_id {:type "string"}
                                               :date {:type "string" :format "date"}
                                               :description {:type "string"}
                                               :notes {:type "string"}}}}]
           :responses {200 {:description "Updated"}}}
     :delete {:tags ["Transactions"] :summary "Delete transaction"
              :security [{:Bearer []}]
              :parameters [{:in "path" :name "id" :type "string" :required true}]
              :responses {204 {:description "Deleted"}}}}
    "/reports/summary"
    {:get {:tags ["Reports"] :summary "Financial summary for date range"
           :security [{:Bearer []}]
           :parameters [{:in "query" :name "from" :type "string" :format "date"}
                        {:in "query" :name "to" :type "string" :format "date"}]
           :responses {200 {:description "Summary with balance, income, expense, count"}}}}
    "/reports/by-category"
    {:get {:tags ["Reports"] :summary "Expense breakdown by category"
           :security [{:Bearer []}]
           :parameters [{:in "query" :name "from" :type "string" :format "date"}
                        {:in "query" :name "to" :type "string" :format "date"}
                        {:in "query" :name "type" :type "string" :enum ["income" "expense"]}]
           :responses {200 {:description "Category breakdown with totals and percentages"}}}}
    "/health"
    {:get {:tags ["System"] :summary "Health check"
           :responses {200 {:description "Status, database, version"}}}}}})

(defn swagger-json-handler [_]
  {:status 200
   :headers {"Content-Type" "application/json"}
   :body (json/generate-string api-spec)})

(defn wrap-swagger [handler]
  (wrap-swagger-ui handler {:path "/swagger-ui"
                            :swagger-docs "/api/swagger.json"}))
```

- [ ] **Step 2: Update `src/cointrail/routes.clj` — add swagger route**

Add to requires: `[cointrail.swagger :as swagger]`

Add inside `/api` context:
```clojure
(GET "/swagger.json" [] swagger/swagger-json-handler)
```

- [ ] **Step 3: Update `src/cointrail/core.clj` — wrap swagger UI**

Add `cointrail.swagger` to requires and wrap app:
```clojure
(defn create-app [cfg]
  (-> app-routes
      (mw/wrap-jwt-auth (:jwt-secret cfg))
      mw/wrap-cors-headers
      mw/wrap-json
      mw/wrap-request-logging
      swagger/wrap-swagger))
```

- [ ] **Step 4: Write swagger test**

`test/cointrail/swagger_test.clj`:
```clojure
(ns cointrail.swagger-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (f)))

(use-fixtures :once db-fixture)

(deftest swagger-json-test
  (let [app (create-app (config))
        response (app (mock/request :get "/api/swagger.json"))
        body (json/parse-string (slurp (:body response)) true)]
    (testing "returns valid swagger spec"
      (is (= 200 (:status response)))
      (is (= "2.0" (:swagger body)))
      (is (= "CoinTrail API" (get-in body [:info :title]))))))

(deftest swagger-ui-test
  (let [app (create-app (config))
        response (app (mock/request :get "/swagger-ui"))]
    (testing "swagger UI redirects or serves content"
      (is (contains? #{200 301 302} (:status response))))))
```

- [ ] **Step 5: Run tests**

Run: `lein test cointrail.swagger-test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: Swagger UI and OpenAPI spec at /swagger-ui"
```

---

### Task 7: Dockerfile, Docker Compose, and GitHub Actions CI

**Files:**
- Create: `Dockerfile`
- Modify: `docker-compose.yml` (already created in Task 1, verify)
- Create: `.github/workflows/ci.yml`
- Create: `.clj-kondo/config.edn`

**Interfaces:**
- Consumes: all application code from Tasks 1-6
- Produces: Docker image, CI pipeline

- [ ] **Step 1: Create `Dockerfile`**

```dockerfile
FROM clojure:temurin-21-lein AS builder
WORKDIR /app
COPY project.clj .
RUN lein deps
COPY . .
RUN lein uberjar

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=builder /app/target/cointrail-standalone.jar app.jar
EXPOSE 8080
CMD ["java", "-jar", "app.jar"]
```

- [ ] **Step 2: Create `.clj-kondo/config.edn`**

```clojure
{:linters {:unresolved-symbol {:level :warning}
           :unused-namespace {:level :warning}
           :unused-binding {:level :warning}}}
```

- [ ] **Step 3: Create `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [master]
  pull_request:
    branches: [master]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install clj-kondo
        run: |
          curl -sLO https://raw.githubusercontent.com/clj-kondo/clj-kondo/master/script/install-clj-kondo
          chmod +x install-clj-kondo
          sudo ./install-clj-kondo
      - name: Run clj-kondo
        run: clj-kondo --lint src test

  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_DB: cointrail_test
          POSTGRES_USER: cointrail
          POSTGRES_PASSWORD: cointrail
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U cointrail"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - name: Setup Java
        uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: 21
      - name: Install Leiningen
        uses: DeLaGuardo/setup-clojure@12.5
        with:
          lein: 2.11.2
      - name: Cache dependencies
        uses: actions/cache@v4
        with:
          path: ~/.m2/repository
          key: ${{ runner.os }}-lein-${{ hashFiles('project.clj') }}
          restore-keys: ${{ runner.os }}-lein-
      - name: Run tests
        env:
          DATABASE_URL: jdbc:postgresql://localhost:5432/cointrail_test?user=cointrail&password=cointrail
          JWT_SECRET: test-secret
          PORT: "8080"
        run: lein test

  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build Docker image
        run: docker compose build
```

- [ ] **Step 4: Create `README.md`**

```markdown
<div align="center">

# CoinTrail

**Personal Finance REST API**

[![CI](https://github.com/MatheusCavalari/cointrail/actions/workflows/ci.yml/badge.svg)](https://github.com/MatheusCavalari/cointrail/actions/workflows/ci.yml)
[![Clojure 1.12](https://img.shields.io/badge/Clojure-1.12-5881d8?logo=clojure&logoColor=white)](https://clojure.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

RESTful personal finance API built with Clojure — JWT authentication, transaction management, category CRUD, and financial reports with aggregations by period and category.

[Getting Started](#getting-started) · [API Reference](#api-reference) · [Architecture](#architecture)

</div>

---

## Overview

CoinTrail is a backend API for managing personal finances. It demonstrates idiomatic Clojure on the JVM — immutability, function composition via threading macros, data-driven design with maps, and declarative validation via clojure.spec.

### Features

- **JWT Authentication** — register/login with bcrypt password hashing and 24h token expiry
- **Transaction Management** — CRUD for income/expense entries with filtering, sorting, and pagination
- **Category System** — 11 default categories seeded on registration + custom user categories
- **Financial Reports** — summary (balance/income/expense) and by-category breakdown with percentages
- **API Documentation** — Swagger UI at `/swagger-ui`

## Tech Stack

| Layer | Technology |
|-------|------------|
| Runtime | Clojure 1.12, JVM 21 (Eclipse Temurin) |
| HTTP | Ring + Compojure |
| Database | PostgreSQL 16, next.jdbc, HoneySQL 2 |
| Migrations | Migratus (auto-run on boot) |
| Auth | buddy-auth (JWT) + buddy-hashers (bcrypt) |
| Validation | clojure.spec.alpha |
| API Docs | Swagger UI |
| Infrastructure | Docker Compose |
| CI | GitHub Actions (clj-kondo + tests + docker build) |

## Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose v2+

### Quick Start

```bash
git clone https://github.com/MatheusCavalari/cointrail.git
cd cointrail
docker compose up --build
```

- **API:** http://localhost:8080
- **Swagger UI:** http://localhost:8080/swagger-ui

## API Reference

### Auth (public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/auth/register` | Register `{email, password, name}` → `{user, token}` |
| POST | `/api/auth/login` | Login `{email, password}` → `{user, token}` |

### Categories (authenticated)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/categories` | List categories (default + custom) |
| POST | `/api/categories` | Create custom `{name, type, icon}` |
| PUT | `/api/categories/:id` | Update (custom only) |
| DELETE | `/api/categories/:id` | Delete (custom only) |

### Transactions (authenticated)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/transactions` | List with filters (`type`, `from`, `to`, `category_id`, `sort`, `order`, `page`, `per_page`) |
| POST | `/api/transactions` | Create `{type, amount, category_id, date, description, notes}` |
| GET | `/api/transactions/:id` | Detail |
| PUT | `/api/transactions/:id` | Update |
| DELETE | `/api/transactions/:id` | Delete |

### Reports (authenticated)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/reports/summary` | Balance, income, expense, count for `?from=&to=` |
| GET | `/api/reports/by-category` | Category breakdown with percentages for `?from=&to=&type=` |

### Health (public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | `{status, database, version}` |

## Architecture

```
cointrail/
├── src/cointrail/
│   ├── core.clj           # Entry point
│   ├── config.clj         # Env var config
│   ├── db.clj             # Connection pool + migrations
│   ├── middleware.clj      # JSON, CORS, logging, JWT auth
│   ├── routes.clj         # Route aggregation
│   ├── swagger.clj        # OpenAPI spec + Swagger UI
│   ├── auth/              # Registration, login, JWT, seed categories
│   ├── categories/        # CRUD with default protection
│   ├── transactions/      # CRUD with filters + pagination
│   └── reports/           # Summary + by-category aggregations
├── test/cointrail/        # Mirrors src/
├── resources/migrations/  # SQL migration files
├── Dockerfile             # Multi-stage (Leiningen → JRE Alpine)
└── docker-compose.yml     # app + postgres
```

## Development

### Without Docker

```bash
# Start PostgreSQL locally, then:
cp .env.example .env
lein run
```

### Testing

```bash
lein test
```

### Linting

```bash
clj-kondo --lint src test
```

## License

This project is licensed under the [MIT License](LICENSE).
```

- [ ] **Step 5: Create `LICENSE`**

Standard MIT license file with copyright `2026 Matheus Cavalari`.

- [ ] **Step 6: Verify Docker build**

Run: `docker compose build`
Expected: Both services build successfully.

- [ ] **Step 7: Verify Docker Compose starts and health responds**

Run: `docker compose up -d && sleep 15 && curl http://localhost:8080/api/health && docker compose down`
Expected: `{"status":"ok","database":"connected","version":"0.1.0"}`

- [ ] **Step 8: Commit and push**

```bash
git add -A
git commit -m "feat: Dockerfile, Docker Compose, GitHub Actions CI, README, and LICENSE"
git push -u origin master
```

---

## Self-Review

**Spec coverage check:**
- Auth (register/login/JWT/seed categories) → Task 2 ✓
- Categories CRUD with default protection → Task 3 ✓
- Transactions CRUD with filters/pagination → Task 4 ✓
- Reports (summary + by-category) → Task 5 ✓
- Swagger UI → Task 6 ✓
- Docker Compose (app + postgres) → Tasks 1 + 7 ✓
- GitHub Actions CI (lint + test + docker) → Task 7 ✓
- Migrations auto-run on boot → Task 1 (Migratus in db/init!) ✓
- Error response format → Used consistently across handlers ✓
- Pagination format → Task 4 repository ✓
- Health endpoint → Task 1 ✓
- README → Task 7 ✓

**Placeholder scan:** No TBD, TODO, or "implement later" found.

**Type consistency:** `user-id` UUID used consistently via `(java.util.UUID/fromString (get-in request [:identity :user-id]))` across all handler files. Repository functions use consistent signatures. HoneySQL column names use kebab-case matching next.jdbc conventions.
