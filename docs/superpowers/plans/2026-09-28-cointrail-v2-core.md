# CoinTrail v2 Core Financial Features — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Tags, Budgets, Goals, and Recurring Transactions to CoinTrail's existing personal finance API.

**Architecture:** Each feature follows the existing layered pattern (handlers → service → repository) with its own `clojure.spec` validations, Migratus migrations, and integration tests. The recurring transactions feature adds a background scheduler using `java.util.concurrent.ScheduledExecutorService`. All four features are independent of each other and integrate with the existing `transactions` and `categories` tables.

**Tech Stack:** Clojure 1.12, Ring 1.12 + Compojure, next.jdbc + HoneySQL 2, buddy-auth (JWT), clojure.spec.alpha, PostgreSQL 16, Migratus, Docker Compose

## Global Constraints

- Clojure 1.12 on JVM 21 (Eclipse Temurin)
- PostgreSQL 16 — all new tables use UUID PKs with `gen_random_uuid()`
- Migratus migrations — ONE SQL statement per `.up.sql` file, use `IF NOT EXISTS` / `IF EXISTS` for idempotency
- All endpoints under `/api/` prefix, JWT Bearer auth on all except `/auth/*` and `/health`
- HoneySQL 2 for query building — `h/where` accepts multiple variadic clauses (wraps in AND)
- `BigDecimal` for all monetary calculations — use `.divide` with `RoundingMode/HALF_UP` and explicit scale
- Follow existing patterns: `parse-uuid` from `cointrail.util`, 400 on invalid UUIDs, `cointrail.auth.spec/validate` for request validation
- One migration file per DDL statement (Migratus + pgjdbc limitation)
- Tests use `ring.mock` with real database — follow `cointrail.categories.handlers-test` patterns (db-fixture, json-request, parse-body, auth-header, register-and-get-token helpers)
- Error responses: `{:error "error_code" :message "Human message"}` — use `ex-info` with `{:status N :body {...}}` in services
- Swagger spec at `/api/swagger.json` must include all new endpoints

## Existing Interfaces Referenced by Tasks

These are functions and patterns already in the codebase that tasks consume:

- `cointrail.db/execute!` — `([sql-params])` or `([conn sql-params])` — returns vec of kebab-case maps
- `cointrail.db/execute-one!` — `([sql-params])` or `([conn sql-params])` — returns one kebab-case map or nil
- `cointrail.db/datasource` — atom holding HikariCP connection pool
- `cointrail.util/parse-uuid` — `[s]` → `java.util.UUID` or `nil`
- `cointrail.auth.spec/validate` — `[spec data]` → error map or `nil`
- `cointrail.core/create-app` — `[cfg]` → Ring handler (used in tests)
- `cointrail.config/config` — `[]` → `{:database-url :jwt-secret :port}`
- Test helpers in each test ns: `db-fixture`, `json-request`, `parse-body`, `auth-header`, `register-and-get-token`

---

### Task 1: Tags — CRUD and Transaction Association

**Files:**
- Create: `src/cointrail/tags/spec.clj`
- Create: `src/cointrail/tags/repository.clj`
- Create: `src/cointrail/tags/service.clj`
- Create: `src/cointrail/tags/handlers.clj`
- Create: `test/cointrail/tags/handlers_test.clj`
- Create: `resources/migrations/007-create-tags.up.sql`
- Create: `resources/migrations/007-create-tags.down.sql`
- Create: `resources/migrations/008-create-transaction-tags.up.sql`
- Create: `resources/migrations/008-create-transaction-tags.down.sql`
- Modify: `src/cointrail/routes.clj`
- Modify: `src/cointrail/transactions/repository.clj`
- Modify: `src/cointrail/swagger.clj`

**Interfaces:**
- Consumes: `cointrail.db/execute!`, `cointrail.db/execute-one!`, `cointrail.util/parse-uuid`, `cointrail.auth.spec/validate`
- Produces:
  - `cointrail.tags.repository/find-by-user [user-id]` → `[{:id :name :color :created-at}]`
  - `cointrail.tags.repository/find-by-id [id]` → map or nil
  - `cointrail.tags.repository/insert! [tag]` → map with `:id`
  - `cointrail.tags.repository/update! [id updates]` → map
  - `cointrail.tags.repository/delete! [id]` → nil
  - `cointrail.tags.repository/find-by-transaction [transaction-id]` → `[{:id :name :color}]`
  - `cointrail.tags.repository/replace-transaction-tags! [conn transaction-id tag-ids]` → nil
  - `cointrail.tags.service/list-tags [user-id]` → vec of maps
  - `cointrail.tags.service/create-tag! [user-id data]` → map
  - `cointrail.tags.service/update-tag! [user-id id data]` → map
  - `cointrail.tags.service/delete-tag! [user-id id]` → nil
  - `cointrail.tags.service/set-transaction-tags! [user-id transaction-id tag-ids]` → vec of tag maps
  - `cointrail.tags.service/get-transaction-tags [user-id transaction-id]` → vec of tag maps

- [ ] **Step 1: Create tag migrations**

Create `resources/migrations/007-create-tags.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(30) NOT NULL,
    color VARCHAR(7),
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(user_id, name)
);
```

Create `resources/migrations/007-create-tags.down.sql`:
```sql
DROP TABLE IF EXISTS tags;
```

Create `resources/migrations/008-create-transaction-tags.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS transaction_tags (
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (transaction_id, tag_id)
);
```

Create `resources/migrations/008-create-transaction-tags.down.sql`:
```sql
DROP TABLE IF EXISTS transaction_tags;
```

- [ ] **Step 2: Create tag spec**

Create `src/cointrail/tags/spec.clj`:
```clojure
(ns cointrail.tags.spec
  (:require [clojure.spec.alpha :as s]))

(s/def ::name (s/and string? #(>= (count %) 1) #(<= (count %) 30)))
(s/def ::color (s/and string? #(re-matches #"^#[0-9a-fA-F]{6}$" %)))

(s/def ::create-tag (s/keys :req-un [::name] :opt-un [::color]))
(s/def ::update-tag (s/keys :opt-un [::name ::color]))

(s/def ::tag-id (s/and string? #(try (java.util.UUID/fromString %) true (catch Exception _ false))))
(s/def ::tag-ids (s/and vector? #(<= (count %) 10) (s/coll-of ::tag-id)))
(s/def ::set-tags (s/keys :req-un [::tag-ids]))
```

- [ ] **Step 3: Create tag repository**

Create `src/cointrail/tags/repository.clj`:
```clojure
(ns cointrail.tags.repository
  (:require [cointrail.db :as db]
            [honey.sql :as sql]
            [honey.sql.helpers :as h]))

(defn find-by-user [user-id]
  (db/execute!
   (-> (h/select :*)
       (h/from :tags)
       (h/where [:= :user-id user-id])
       (h/order-by [:name :asc])
       sql/format)))

(defn find-by-id [id]
  (db/execute-one!
   (-> (h/select :*)
       (h/from :tags)
       (h/where [:= :id id])
       sql/format)))

(defn insert! [tag]
  (db/execute-one!
   (-> (h/insert-into :tags)
       (h/values [tag])
       (h/returning :*)
       sql/format)))

(defn update! [id updates]
  (db/execute-one!
   (-> (h/update :tags)
       (h/set updates)
       (h/where [:= :id id])
       (h/returning :*)
       sql/format)))

(defn delete! [id]
  (db/execute-one!
   (-> (h/delete-from :tags)
       (h/where [:= :id id])
       sql/format)))

(defn find-by-transaction [transaction-id]
  (db/execute!
   (-> (h/select :t.id :t.name :t.color)
       (h/from [:tags :t])
       (h/join [:transaction-tags :tt] [:= :t.id :tt.tag-id])
       (h/where [:= :tt.transaction-id transaction-id])
       (h/order-by [:t.name :asc])
       sql/format)))

(defn replace-transaction-tags!
  ([transaction-id tag-ids]
   (replace-transaction-tags! @db/datasource transaction-id tag-ids))
  ([conn transaction-id tag-ids]
   (db/execute! conn
    (-> (h/delete-from :transaction-tags)
        (h/where [:= :transaction-id transaction-id])
        sql/format))
   (when (seq tag-ids)
     (db/execute! conn
      (-> (h/insert-into :transaction-tags)
          (h/values (mapv (fn [tid] {:transaction-id transaction-id :tag-id tid}) tag-ids))
          sql/format)))))

(defn find-by-user-and-ids [user-id tag-ids]
  (db/execute!
   (-> (h/select :id)
       (h/from :tags)
       (h/where [:= :user-id user-id]
                [:in :id tag-ids])
       sql/format)))
```

- [ ] **Step 4: Create tag service**

Create `src/cointrail/tags/service.clj`:
```clojure
(ns cointrail.tags.service
  (:require [cointrail.tags.repository :as repo]
            [cointrail.transactions.repository :as txn-repo]))

(defn list-tags [user-id]
  (repo/find-by-user user-id))

(defn create-tag! [user-id data]
  (try
    (repo/insert! (assoc data :user-id user-id))
    (catch org.postgresql.util.PSQLException e
      (if (.contains (.getMessage e) "duplicate key")
        (throw (ex-info "Tag already exists"
                        {:status 409
                         :body {:error "conflict"
                                :message "Tag with this name already exists"}}))
        (throw e)))))

(defn update-tag! [user-id id data]
  (let [tag (repo/find-by-id id)]
    (cond
      (nil? tag)
      (throw (ex-info "Not found" {:status 404
                                   :body {:error "not_found"
                                          :message "Tag not found"}}))
      (not= (:user-id tag) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Tag belongs to another user"}}))
      :else
      (repo/update! id data))))

(defn delete-tag! [user-id id]
  (let [tag (repo/find-by-id id)]
    (cond
      (nil? tag)
      (throw (ex-info "Not found" {:status 404
                                   :body {:error "not_found"
                                          :message "Tag not found"}}))
      (not= (:user-id tag) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Tag belongs to another user"}}))
      :else
      (do (repo/delete! id) nil))))

(defn set-transaction-tags! [user-id transaction-id tag-ids]
  (let [txn (txn-repo/find-by-id transaction-id)]
    (cond
      (nil? txn)
      (throw (ex-info "Not found" {:status 404
                                   :body {:error "not_found"
                                          :message "Transaction not found"}}))
      (not= (:user-id txn) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Transaction belongs to another user"}}))
      :else
      (let [uuid-ids (mapv #(java.util.UUID/fromString %) tag-ids)]
        (when (seq uuid-ids)
          (let [found (repo/find-by-user-and-ids user-id uuid-ids)]
            (when (not= (count found) (count uuid-ids))
              (throw (ex-info "Invalid tags" {:status 400
                                              :body {:error "bad_request"
                                                     :message "One or more tag IDs are invalid"}})))))
        (repo/replace-transaction-tags! transaction-id uuid-ids)
        (repo/find-by-transaction transaction-id)))))

(defn get-transaction-tags [user-id transaction-id]
  (let [txn (txn-repo/find-by-id transaction-id)]
    (cond
      (nil? txn)
      (throw (ex-info "Not found" {:status 404
                                   :body {:error "not_found"
                                          :message "Transaction not found"}}))
      (not= (:user-id txn) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Transaction belongs to another user"}}))
      :else
      (repo/find-by-transaction transaction-id))))
```

- [ ] **Step 5: Create tag handlers**

Create `src/cointrail/tags/handlers.clj`:
```clojure
(ns cointrail.tags.handlers
  (:require [cointrail.tags.service :as service]
            [cointrail.tags.spec :as tag-spec]
            [cointrail.auth.spec :refer [validate]]
            [cointrail.util :refer [parse-uuid]]))

(defn- user-id [request]
  (java.util.UUID/fromString (get-in request [:identity :user-id])))

(def ^:private invalid-id
  {:status 400 :body {:error "bad_request" :message "Invalid id"}})

(defn list-handler [request]
  {:status 200
   :body {:data (service/list-tags (user-id request))}})

(defn create-handler [request]
  (let [body (:body request)]
    (if-let [errors (validate ::tag-spec/create-tag body)]
      {:status 400 :body errors}
      (try
        {:status 201 :body (service/create-tag! (user-id request) body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn update-handler [request]
  (let [id (parse-uuid (get-in request [:params :id]))
        body (:body request)]
    (cond
      (nil? id) invalid-id
      (validate ::tag-spec/update-tag body)
      {:status 400 :body (validate ::tag-spec/update-tag body)}
      (empty? (select-keys body [:name :color]))
      {:status 400 :body {:error "validation_error"
                          :message "at least one field required"}}
      :else
      (try
        {:status 200 :body (service/update-tag! (user-id request) id body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn delete-handler [request]
  (if-let [id (parse-uuid (get-in request [:params :id]))]
    (try
      (service/delete-tag! (user-id request) id)
      {:status 204 :body nil}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))
    invalid-id))

(defn set-transaction-tags-handler [request]
  (let [txn-id (parse-uuid (get-in request [:params :id]))
        body (:body request)]
    (if (nil? txn-id)
      invalid-id
      (if-let [errors (validate ::tag-spec/set-tags body)]
        {:status 400 :body errors}
        (try
          {:status 200 :body {:data (service/set-transaction-tags! (user-id request) txn-id (:tag-ids body))}}
          (catch clojure.lang.ExceptionInfo e
            (let [data (ex-data e)]
              {:status (:status data) :body (:body data)})))))))

(defn get-transaction-tags-handler [request]
  (if-let [txn-id (parse-uuid (get-in request [:params :id]))]
    (try
      {:status 200 :body {:data (service/get-transaction-tags (user-id request) txn-id)}}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))
    invalid-id))
```

- [ ] **Step 6: Add tag routes**

Modify `src/cointrail/routes.clj` — add the require and routes:

Add to `:require`: `[cointrail.tags.handlers :as tags]`

Add routes inside the `(context "/api" [] ...)` block, after reports:
```clojure
    (context "/tags" []
      (mw/require-auth
       (routes
        (GET "/" [] tags/list-handler)
        (POST "/" [] tags/create-handler)
        (PUT "/:id" [] tags/update-handler)
        (DELETE "/:id" [] tags/delete-handler))))
```

Add inside the existing `/transactions` context, inside the `mw/require-auth` routes block:
```clojure
        (PUT "/:id/tags" [] tags/set-transaction-tags-handler)
        (GET "/:id/tags" [] tags/get-transaction-tags-handler)
```

- [ ] **Step 7: Add tags to transaction list response**

Modify `src/cointrail/transactions/repository.clj`:

Add a require for tags repository: `[cointrail.tags.repository :as tag-repo]`

Modify the `find-by-user` function to enrich each transaction with its tags. After the existing `data` binding, add:
```clojure
        data-with-tags (mapv (fn [txn]
                               (assoc txn :tags (tag-repo/find-by-transaction (:id txn))))
                             data)
```
And return `data-with-tags` instead of `data` in the result map.

Also modify `find-by-id` to include tags:
```clojure
(defn find-by-id [id]
  (when-let [txn (db/execute-one!
                  (-> (h/select :t.* [:c.name :category-name] [:c.icon :category-icon])
                      (h/from [:transactions :t])
                      (h/left-join [:categories :c] [:= :t.category-id :c.id])
                      (h/where [:= :t.id id])
                      sql/format))]
    (assoc txn :tags (tag-repo/find-by-transaction (:id txn)))))
```

Add tag filter support to `apply-filters`:
```clojure
    tag         (h/join [:transaction-tags :ttf] [:= :t.id :ttf.transaction-id])
    tag         (h/where [:= :ttf.tag-id (java.util.UUID/fromString tag)])
```

Add `:tag` to the filter extraction — this happens in `src/cointrail/transactions/handlers.clj`, add to `extract-filters`:
```clojure
   :tag (:tag params)
```

- [ ] **Step 8: Update Swagger spec with tag endpoints**

Modify `src/cointrail/swagger.clj` — add to the `:paths` map:

```clojure
    "/tags"
    {:get {:tags ["Tags"] :summary "List user tags"
           :security [{:Bearer []}]
           :responses {200 {:description "List of tags"}}}
     :post {:tags ["Tags"] :summary "Create tag"
            :security [{:Bearer []}]
            :parameters [{:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["name"]
                                   :properties {:name {:type "string" :maxLength 30}
                                                :color {:type "string" :pattern "^#[0-9a-fA-F]{6}$"}}}}]
            :responses {201 {:description "Tag created"}
                        409 {:description "Tag name already exists"}}}}
    "/tags/{id}"
    {:put {:tags ["Tags"] :summary "Update tag"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}
                        {:in "body" :name "body"
                         :schema {:type "object"
                                  :properties {:name {:type "string" :maxLength 30}
                                               :color {:type "string"}}}}]
           :responses {200 {:description "Updated"}}}
     :delete {:tags ["Tags"] :summary "Delete tag"
              :security [{:Bearer []}]
              :parameters [{:in "path" :name "id" :type "string" :required true}]
              :responses {204 {:description "Deleted"}}}}
    "/transactions/{id}/tags"
    {:get {:tags ["Tags"] :summary "List transaction's tags"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}]
           :responses {200 {:description "Tags for this transaction"}}}
     :put {:tags ["Tags"] :summary "Set transaction tags (replace all)"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}
                        {:in "body" :name "body" :required true
                         :schema {:type "object"
                                  :required ["tag_ids"]
                                  :properties {:tag_ids {:type "array"
                                                        :items {:type "string"}
                                                        :maxItems 10}}}}]
           :responses {200 {:description "Updated tags"}}}}
```

Also add `?tag` query param to the existing `/transactions` GET parameters:
```clojure
{:in "query" :name "tag" :type "string" :description "Filter by tag UUID"}
```

- [ ] **Step 9: Write tag integration tests**

Create `test/cointrail/tags/handlers_test.clj`:
```clojure
(ns cointrail.tags.handlers-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (db/execute! ["DELETE FROM transaction_tags"])
    (db/execute! ["DELETE FROM tags"])
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

(deftest tag-crud-test
  (let [app (create-app (config))
        token (register-and-get-token app)]

    (testing "creates a tag"
      (let [response (app (auth-header
                           (json-request :post "/api/tags"
                                        {:name "vacation" :color "#FF5733"})
                           token))
            body (parse-body response)]
        (is (= 201 (:status response)))
        (is (= "vacation" (:name body)))
        (is (= "#FF5733" (:color body)))))

    (testing "lists tags"
      (let [response (app (auth-header (mock/request :get "/api/tags") token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 1 (count (:data body))))))

    (testing "duplicate name returns 409"
      (let [response (app (auth-header
                           (json-request :post "/api/tags" {:name "vacation"})
                           token))]
        (is (= 409 (:status response)))))

    (testing "updates a tag"
      (let [tags-resp (app (auth-header (mock/request :get "/api/tags") token))
            tag-id (:id (first (:data (parse-body tags-resp))))
            response (app (auth-header
                           (json-request :put (str "/api/tags/" tag-id)
                                        {:color "#00FF00"})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= "#00FF00" (:color body)))))

    (testing "deletes a tag"
      (let [tags-resp (app (auth-header (mock/request :get "/api/tags") token))
            tag-id (:id (first (:data (parse-body tags-resp))))
            response (app (auth-header
                           (mock/request :delete (str "/api/tags/" tag-id))
                           token))]
        (is (= 204 (:status response)))))))

(deftest transaction-tags-test
  (let [app (create-app (config))
        token (register-and-get-token app)
        ;; Create a transaction
        txn-resp (app (auth-header
                       (json-request :post "/api/transactions"
                                    {:type "expense" :amount 50.00
                                     :description "Lunch"})
                       token))
        txn-id (:id (parse-body txn-resp))
        ;; Create two tags
        _ (app (auth-header (json-request :post "/api/tags" {:name "food"}) token))
        _ (app (auth-header (json-request :post "/api/tags" {:name "work"}) token))
        tags-resp (app (auth-header (mock/request :get "/api/tags") token))
        tag-ids (mapv :id (:data (parse-body tags-resp)))]

    (testing "sets tags on transaction"
      (let [response (app (auth-header
                           (json-request :put (str "/api/transactions/" txn-id "/tags")
                                        {:tag_ids tag-ids})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 2 (count (:data body))))))

    (testing "gets transaction tags"
      (let [response (app (auth-header
                           (mock/request :get (str "/api/transactions/" txn-id "/tags"))
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 2 (count (:data body))))))

    (testing "transaction detail includes tags"
      (let [response (app (auth-header
                           (mock/request :get (str "/api/transactions/" txn-id))
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 2 (count (:tags body))))))

    (testing "replaces tags with empty list"
      (let [response (app (auth-header
                           (json-request :put (str "/api/transactions/" txn-id "/tags")
                                        {:tag_ids []})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 0 (count (:data body))))))))
```

- [ ] **Step 10: Run tests and verify**

Run: `cd C:\Users\Mathe\source\cointrail && lein test cointrail.tags.handlers-test`
Expected: All tests pass.

- [ ] **Step 11: Commit**

```bash
git add src/cointrail/tags/ test/cointrail/tags/ resources/migrations/007-* resources/migrations/008-* src/cointrail/routes.clj src/cointrail/transactions/repository.clj src/cointrail/transactions/handlers.clj src/cointrail/swagger.clj
git commit -m "feat: tags CRUD with transaction association and filtering"
```

---

### Task 2: Budgets — Monthly Category Budgets with Spend Tracking

**Files:**
- Create: `src/cointrail/budgets/spec.clj`
- Create: `src/cointrail/budgets/repository.clj`
- Create: `src/cointrail/budgets/service.clj`
- Create: `src/cointrail/budgets/handlers.clj`
- Create: `test/cointrail/budgets/handlers_test.clj`
- Create: `resources/migrations/009-create-budgets.up.sql`
- Create: `resources/migrations/009-create-budgets.down.sql`
- Modify: `src/cointrail/routes.clj`
- Modify: `src/cointrail/swagger.clj`

**Interfaces:**
- Consumes: `cointrail.db/execute!`, `cointrail.db/execute-one!`, `cointrail.util/parse-uuid`, `cointrail.auth.spec/validate`, `cointrail.categories.repository/find-by-id`
- Produces:
  - `cointrail.budgets.repository/find-by-user [user-id]` → vec of maps with category info
  - `cointrail.budgets.repository/spent-for-category [user-id category-id year-month]` → BigDecimal
  - `cointrail.budgets.repository/find-by-id [id]` → map or nil
  - `cointrail.budgets.repository/insert! [budget]` → map
  - `cointrail.budgets.repository/update! [id updates]` → map
  - `cointrail.budgets.repository/delete! [id]` → nil
  - `cointrail.budgets.service/list-budgets [user-id month-str]` → vec of enriched maps with status
  - `cointrail.budgets.service/create-budget! [user-id data]` → map
  - `cointrail.budgets.service/update-budget! [user-id id data]` → map
  - `cointrail.budgets.service/delete-budget! [user-id id]` → nil

- [ ] **Step 1: Create budget migration**

Create `resources/migrations/009-create-budgets.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    amount DECIMAL(12,2) NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(user_id, category_id)
);
```

Create `resources/migrations/009-create-budgets.down.sql`:
```sql
DROP TABLE IF EXISTS budgets;
```

- [ ] **Step 2: Create budget spec**

Create `src/cointrail/budgets/spec.clj`:
```clojure
(ns cointrail.budgets.spec
  (:require [clojure.spec.alpha :as s]))

(s/def ::category-id (s/and string? #(try (java.util.UUID/fromString %) true (catch Exception _ false))))
(s/def ::amount (s/and number? pos?))
(s/def ::month (s/and string? #(re-matches #"\d{4}-\d{2}" %)))

(s/def ::create-budget (s/keys :req-un [::category-id ::amount]))
(s/def ::update-budget (s/keys :req-un [::amount]))
```

- [ ] **Step 3: Create budget repository**

Create `src/cointrail/budgets/repository.clj`:
```clojure
(ns cointrail.budgets.repository
  (:require [cointrail.db :as db]
            [honey.sql :as sql]
            [honey.sql.helpers :as h])
  (:import [java.time YearMonth]))

(defn find-by-user [user-id]
  (db/execute!
   (-> (h/select :b.* [:c.id :category-id] [:c.name :category-name] [:c.icon :category-icon])
       (h/from [:budgets :b])
       (h/join [:categories :c] [:= :b.category-id :c.id])
       (h/where [:= :b.user-id user-id])
       (h/order-by [:c.name :asc])
       sql/format)))

(defn spent-for-category [user-id category-id ^YearMonth year-month]
  (let [from (.atDay year-month 1)
        to (.atEndOfMonth year-month)]
    (or (:total
         (db/execute-one!
          (-> (h/select [:%sum.amount :total])
              (h/from :transactions)
              (h/where [:= :user-id user-id]
                       [:= :category-id category-id]
                       [:= :type "expense"]
                       [:>= :date from]
                       [:<= :date to])
              sql/format)))
        0M)))

(defn find-by-id [id]
  (db/execute-one!
   (-> (h/select :*)
       (h/from :budgets)
       (h/where [:= :id id])
       sql/format)))

(defn insert! [budget]
  (db/execute-one!
   (-> (h/insert-into :budgets)
       (h/values [budget])
       (h/returning :*)
       sql/format)))

(defn update! [id updates]
  (db/execute-one!
   (-> (h/update :budgets)
       (h/set (assoc updates :updated-at :%now))
       (h/where [:= :id id])
       (h/returning :*)
       sql/format)))

(defn delete! [id]
  (db/execute-one!
   (-> (h/delete-from :budgets)
       (h/where [:= :id id])
       sql/format)))

- [ ] **Step 4: Create budget service**

Create `src/cointrail/budgets/service.clj`:
```clojure
(ns cointrail.budgets.service
  (:require [cointrail.budgets.repository :as repo]
            [cointrail.categories.repository :as cat-repo])
  (:import [java.math RoundingMode]
           [java.time YearMonth]))

(defn- parse-month [month-str]
  (try
    (if month-str
      (YearMonth/parse month-str)
      (YearMonth/now))
    (catch Exception _ nil)))

(defn- budget-status [^BigDecimal spent ^BigDecimal amount]
  (let [pct (if (pos? (.compareTo amount BigDecimal/ZERO))
              (-> (BigDecimal. 100)
                  (.multiply spent)
                  (.divide amount 1 RoundingMode/HALF_UP)
                  double)
              0.0)]
    {:percentage pct
     :status (cond
               (>= pct 100.0) "over_budget"
               (>= pct 80.0)  "warning"
               :else           "on_track")}))

(defn list-budgets [user-id month-str]
  (let [ym (parse-month month-str)]
    (when-not ym
      (throw (ex-info "Invalid month" {:status 400
                                       :body {:error "bad_request"
                                              :message "Invalid month, expected YYYY-MM"}})))
    (let [budgets (repo/find-by-user user-id)]
      (mapv (fn [b]
              (let [spent (repo/spent-for-category user-id (:category-id b) ym)
                    amount (:amount b)
                    {:keys [percentage status]} (budget-status (bigdec spent) (bigdec amount))]
                {:id (:id b)
                 :category {:id (:category-id b)
                            :name (:category-name b)
                            :icon (:category-icon b)}
                 :amount amount
                 :spent spent
                 :remaining (- amount spent)
                 :percentage percentage
                 :status status}))
            budgets))))

(defn create-budget! [user-id data]
  (let [cat-id (java.util.UUID/fromString (:category-id data))
        category (cat-repo/find-by-id cat-id)]
    (cond
      (nil? category)
      (throw (ex-info "Category not found" {:status 404
                                            :body {:error "not_found"
                                                   :message "Category not found"}}))
      (not= (:user-id category) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Category belongs to another user"}}))
      (not= (:type category) "expense")
      (throw (ex-info "Invalid category type" {:status 400
                                               :body {:error "bad_request"
                                                      :message "Budgets can only be set for expense categories"}}))
      :else
      (try
        (repo/insert! {:user-id user-id
                       :category-id cat-id
                       :amount (:amount data)})
        (catch org.postgresql.util.PSQLException e
          (if (.contains (.getMessage e) "duplicate key")
            (throw (ex-info "Budget exists" {:status 409
                                             :body {:error "conflict"
                                                    :message "Budget already exists for this category"}}))
            (throw e)))))))

(defn update-budget! [user-id id data]
  (let [budget (repo/find-by-id id)]
    (cond
      (nil? budget)
      (throw (ex-info "Not found" {:status 404
                                   :body {:error "not_found"
                                          :message "Budget not found"}}))
      (not= (:user-id budget) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Budget belongs to another user"}}))
      :else
      (repo/update! id {:amount (:amount data)}))))

(defn delete-budget! [user-id id]
  (let [budget (repo/find-by-id id)]
    (cond
      (nil? budget)
      (throw (ex-info "Not found" {:status 404
                                   :body {:error "not_found"
                                          :message "Budget not found"}}))
      (not= (:user-id budget) user-id)
      (throw (ex-info "Forbidden" {:status 403
                                   :body {:error "forbidden"
                                          :message "Budget belongs to another user"}}))
      :else
      (do (repo/delete! id) nil))))
```

- [ ] **Step 5: Create budget handlers**

Create `src/cointrail/budgets/handlers.clj`:
```clojure
(ns cointrail.budgets.handlers
  (:require [cointrail.budgets.service :as service]
            [cointrail.budgets.spec :as budget-spec]
            [cointrail.auth.spec :refer [validate]]
            [cointrail.util :refer [parse-uuid]]))

(defn- user-id [request]
  (java.util.UUID/fromString (get-in request [:identity :user-id])))

(def ^:private invalid-id
  {:status 400 :body {:error "bad_request" :message "Invalid id"}})

(defn list-handler [request]
  (let [month (get-in request [:params :month])]
    (try
      {:status 200
       :body {:data (service/list-budgets (user-id request) month)}}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))))

(defn create-handler [request]
  (let [body (:body request)]
    (if-let [errors (validate ::budget-spec/create-budget body)]
      {:status 400 :body errors}
      (try
        {:status 201
         :body (service/create-budget! (user-id request) body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn update-handler [request]
  (let [id (parse-uuid (get-in request [:params :id]))
        body (:body request)]
    (if (nil? id)
      invalid-id
      (if-let [errors (validate ::budget-spec/update-budget body)]
        {:status 400 :body errors}
        (try
          {:status 200
           :body (service/update-budget! (user-id request) id body)}
          (catch clojure.lang.ExceptionInfo e
            (let [data (ex-data e)]
              {:status (:status data) :body (:body data)})))))))

(defn delete-handler [request]
  (if-let [id (parse-uuid (get-in request [:params :id]))]
    (try
      (service/delete-budget! (user-id request) id)
      {:status 204 :body nil}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))
    invalid-id))
```

- [ ] **Step 6: Add budget routes**

Modify `src/cointrail/routes.clj`:

Add to `:require`: `[cointrail.budgets.handlers :as budgets]`

Add routes inside `(context "/api" [] ...)`:
```clojure
    (context "/budgets" []
      (mw/require-auth
       (routes
        (GET "/" [] budgets/list-handler)
        (POST "/" [] budgets/create-handler)
        (PUT "/:id" [] budgets/update-handler)
        (DELETE "/:id" [] budgets/delete-handler))))
```

- [ ] **Step 7: Update Swagger spec with budget endpoints**

Modify `src/cointrail/swagger.clj` — add to the `:paths` map:

```clojure
    "/budgets"
    {:get {:tags ["Budgets"] :summary "List budgets with spend progress"
           :security [{:Bearer []}]
           :parameters [{:in "query" :name "month" :type "string"
                         :description "YYYY-MM format, defaults to current month"}]
           :responses {200 {:description "Budgets with spent/remaining/status"}}}
     :post {:tags ["Budgets"] :summary "Create budget for expense category"
            :security [{:Bearer []}]
            :parameters [{:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["category_id" "amount"]
                                   :properties {:category_id {:type "string"}
                                                :amount {:type "number" :minimum 0.01}}}}]
            :responses {201 {:description "Budget created"}
                        409 {:description "Budget already exists for category"}}}}
    "/budgets/{id}"
    {:put {:tags ["Budgets"] :summary "Update budget amount"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}
                        {:in "body" :name "body" :required true
                         :schema {:type "object"
                                  :required ["amount"]
                                  :properties {:amount {:type "number" :minimum 0.01}}}}]
           :responses {200 {:description "Updated"}}}
     :delete {:tags ["Budgets"] :summary "Delete budget"
              :security [{:Bearer []}]
              :parameters [{:in "path" :name "id" :type "string" :required true}]
              :responses {204 {:description "Deleted"}}}}
```

- [ ] **Step 8: Write budget integration tests**

Create `test/cointrail/budgets/handlers_test.clj`:
```clojure
(ns cointrail.budgets.handlers-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json])
  (:import [java.time LocalDate YearMonth]
           [java.time.format DateTimeFormatter]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (db/execute! ["DELETE FROM budgets"])
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

(defn get-expense-category-id [app token]
  (let [cats-resp (app (auth-header (mock/request :get "/api/categories") token))
        cats (:data (parse-body cats-resp))]
    (:id (first (filter #(and (= "expense" (:type %)) (not (:is-default %))) cats)))))

(defn get-default-expense-category-id [app token]
  (let [cats-resp (app (auth-header (mock/request :get "/api/categories") token))
        cats (:data (parse-body cats-resp))]
    (:id (first (filter #(and (= "expense" (:type %)) (:is-default %)) cats)))))

(deftest budget-crud-test
  (let [app (create-app (config))
        token (register-and-get-token app)
        cat-id (get-default-expense-category-id app token)]

    (testing "creates budget for expense category"
      (let [response (app (auth-header
                           (json-request :post "/api/budgets"
                                        {:category_id cat-id :amount 800.00})
                           token))
            body (parse-body response)]
        (is (= 201 (:status response)))
        (is (= 800.0 (:amount body)))))

    (testing "duplicate category returns 409"
      (let [response (app (auth-header
                           (json-request :post "/api/budgets"
                                        {:category_id cat-id :amount 500.00})
                           token))]
        (is (= 409 (:status response)))))

    (testing "lists budgets with spend progress"
      (let [response (app (auth-header (mock/request :get "/api/budgets") token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 1 (count (:data body))))
        (let [budget (first (:data body))]
          (is (= 800.0 (:amount budget)))
          (is (= 0 (:spent budget)))
          (is (= "on_track" (:status budget))))))

    (testing "updates budget amount"
      (let [budgets-resp (app (auth-header (mock/request :get "/api/budgets") token))
            budget-id (:id (first (:data (parse-body budgets-resp))))
            response (app (auth-header
                           (json-request :put (str "/api/budgets/" budget-id)
                                        {:amount 1000.00})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 1000.0 (:amount body)))))

    (testing "deletes budget"
      (let [budgets-resp (app (auth-header (mock/request :get "/api/budgets") token))
            budget-id (:id (first (:data (parse-body budgets-resp))))
            response (app (auth-header
                           (mock/request :delete (str "/api/budgets/" budget-id))
                           token))]
        (is (= 204 (:status response)))))))

(deftest budget-spend-tracking-test
  (let [app (create-app (config))
        token (register-and-get-token app)
        cat-id (get-default-expense-category-id app token)
        today (str (LocalDate/now))
        current-month (str (YearMonth/now))]

    ;; Create budget: 500 for this category
    (app (auth-header (json-request :post "/api/budgets"
                                    {:category_id cat-id :amount 500.00}) token))

    ;; Create expense transaction in this category for today
    (app (auth-header (json-request :post "/api/transactions"
                                    {:type "expense" :amount 420.00
                                     :category_id cat-id :date today}) token))

    (testing "budget shows warning status at 84%"
      (let [response (app (auth-header
                           (mock/request :get (str "/api/budgets?month=" current-month))
                           token))
            budget (first (:data (parse-body response)))]
        (is (= 420.0 (:spent budget)))
        (is (= 80.0 (:remaining budget)))
        (is (= "warning" (:status budget)))))

    ;; Add another expense to go over budget
    (app (auth-header (json-request :post "/api/transactions"
                                    {:type "expense" :amount 100.00
                                     :category_id cat-id :date today}) token))

    (testing "budget shows over_budget status at 104%"
      (let [response (app (auth-header (mock/request :get "/api/budgets") token))
            budget (first (:data (parse-body response)))]
        (is (= 520.0 (:spent budget)))
        (is (= "over_budget" (:status budget)))))))

(deftest budget-validation-test
  (let [app (create-app (config))
        token (register-and-get-token app)]

    (testing "rejects income category"
      (let [cats-resp (app (auth-header (mock/request :get "/api/categories") token))
            income-cat-id (:id (first (filter #(= "income" (:type %))
                                              (:data (parse-body cats-resp)))))
            response (app (auth-header
                           (json-request :post "/api/budgets"
                                        {:category_id income-cat-id :amount 500.00})
                           token))]
        (is (= 400 (:status response)))))))
```

- [ ] **Step 9: Run tests and verify**

Run: `cd C:\Users\Mathe\source\cointrail && lein test cointrail.budgets.handlers-test`
Expected: All tests pass.

- [ ] **Step 10: Commit**

```bash
git add src/cointrail/budgets/ test/cointrail/budgets/ resources/migrations/009-* src/cointrail/routes.clj src/cointrail/swagger.clj
git commit -m "feat: monthly budgets with spend tracking and status alerts"
```

---

### Task 3: Goals — Savings Goals with Deposits and Withdrawals

**Files:**
- Create: `src/cointrail/goals/spec.clj`
- Create: `src/cointrail/goals/repository.clj`
- Create: `src/cointrail/goals/service.clj`
- Create: `src/cointrail/goals/handlers.clj`
- Create: `test/cointrail/goals/handlers_test.clj`
- Create: `resources/migrations/010-create-goals.up.sql`
- Create: `resources/migrations/010-create-goals.down.sql`
- Create: `resources/migrations/011-create-goal-deposits.up.sql`
- Create: `resources/migrations/011-create-goal-deposits.down.sql`
- Modify: `src/cointrail/routes.clj`
- Modify: `src/cointrail/swagger.clj`

**Interfaces:**
- Consumes: `cointrail.db/execute!`, `cointrail.db/execute-one!`, `cointrail.db/datasource`, `cointrail.util/parse-uuid`, `cointrail.auth.spec/validate`, `next.jdbc/with-transaction`
- Produces:
  - `cointrail.goals.repository/find-by-user [user-id status-filter]` → vec of maps
  - `cointrail.goals.repository/find-by-id [id]` → map or nil
  - `cointrail.goals.repository/insert! [goal]` → map
  - `cointrail.goals.repository/update! [id updates]` → map
  - `cointrail.goals.repository/delete! [id]` → nil
  - `cointrail.goals.repository/insert-deposit! [conn deposit]` → map
  - `cointrail.goals.repository/update-amount! [conn id new-amount]` → map
  - `cointrail.goals.repository/find-deposits [goal-id]` → vec of maps
  - `cointrail.goals.service/list-goals [user-id status-filter]` → vec of enriched maps
  - `cointrail.goals.service/create-goal! [user-id data]` → map
  - `cointrail.goals.service/update-goal! [user-id id data]` → map
  - `cointrail.goals.service/delete-goal! [user-id id]` → nil
  - `cointrail.goals.service/deposit! [user-id id data]` → enriched goal map
  - `cointrail.goals.service/withdraw! [user-id id data]` → enriched goal map
  - `cointrail.goals.service/list-deposits [user-id id]` → vec of maps

- [ ] **Step 1: Create goal migrations**

Create `resources/migrations/010-create-goals.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    target_amount DECIMAL(12,2) NOT NULL CHECK (target_amount > 0),
    current_amount DECIMAL(12,2) NOT NULL DEFAULT 0 CHECK (current_amount >= 0),
    deadline DATE,
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'reached', 'cancelled')),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
```

Create `resources/migrations/010-create-goals.down.sql`:
```sql
DROP TABLE IF EXISTS goals;
```

Create `resources/migrations/011-create-goal-deposits.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS goal_deposits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    goal_id UUID NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    amount DECIMAL(12,2) NOT NULL,
    note VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT now()
);
```

Create `resources/migrations/011-create-goal-deposits.down.sql`:
```sql
DROP TABLE IF EXISTS goal_deposits;
```

- [ ] **Step 2: Create goal spec**

Create `src/cointrail/goals/spec.clj`:
```clojure
(ns cointrail.goals.spec
  (:require [clojure.spec.alpha :as s]))

(s/def ::name (s/and string? #(>= (count %) 1) #(<= (count %) 100)))
(s/def ::target-amount (s/and number? pos?))
(s/def ::deadline (s/nilable (s/and string? #(re-matches #"\d{4}-\d{2}-\d{2}" %))))
(s/def ::amount (s/and number? pos?))
(s/def ::note (s/nilable (s/and string? #(<= (count %) 255))))

(s/def ::create-goal (s/keys :req-un [::name ::target-amount] :opt-un [::deadline]))
(s/def ::update-goal (s/keys :opt-un [::name ::target-amount ::deadline]))
(s/def ::deposit-request (s/keys :req-un [::amount] :opt-un [::note]))
```

- [ ] **Step 3: Create goal repository**

Create `src/cointrail/goals/repository.clj`:
```clojure
(ns cointrail.goals.repository
  (:require [cointrail.db :as db]
            [honey.sql :as sql]
            [honey.sql.helpers :as h]))

(defn find-by-user
  ([user-id] (find-by-user user-id nil))
  ([user-id status-filter]
   (db/execute!
    (-> (h/select :*)
        (h/from :goals)
        (h/where [:= :user-id user-id])
        (cond-> status-filter (h/where [:= :status status-filter]))
        (h/order-by [:created-at :desc])
        sql/format))))

(defn find-by-id [id]
  (db/execute-one!
   (-> (h/select :*)
       (h/from :goals)
       (h/where [:= :id id])
       sql/format)))

(defn insert! [goal]
  (db/execute-one!
   (-> (h/insert-into :goals)
       (h/values [goal])
       (h/returning :*)
       sql/format)))

(defn update! [id updates]
  (db/execute-one!
   (-> (h/update :goals)
       (h/set (assoc updates :updated-at :%now))
       (h/where [:= :id id])
       (h/returning :*)
       sql/format)))

(defn delete! [id]
  (db/execute-one!
   (-> (h/delete-from :goals)
       (h/where [:= :id id])
       sql/format)))

(defn insert-deposit! [conn deposit]
  (db/execute-one! conn
   (-> (h/insert-into :goal-deposits)
       (h/values [deposit])
       (h/returning :*)
       sql/format)))

(defn update-amount! [conn id new-amount]
  (db/execute-one! conn
   (-> (h/update :goals)
       (h/set {:current-amount new-amount :updated-at :%now})
       (h/where [:= :id id])
       (h/returning :*)
       sql/format)))

(defn update-status! [conn id status]
  (db/execute-one! conn
   (-> (h/update :goals)
       (h/set {:status status :updated-at :%now})
       (h/where [:= :id id])
       sql/format)))

(defn find-deposits [goal-id]
  (db/execute!
   (-> (h/select :*)
       (h/from :goal-deposits)
       (h/where [:= :goal-id goal-id])
       (h/order-by [:created-at :desc])
       sql/format)))
```

- [ ] **Step 4: Create goal service**

Create `src/cointrail/goals/service.clj`:
```clojure
(ns cointrail.goals.service
  (:require [cointrail.goals.repository :as repo]
            [cointrail.db :as db]
            [next.jdbc :as jdbc])
  (:import [java.math RoundingMode]
           [java.time LocalDate]
           [java.time.temporal ChronoUnit]))

(defn- enrich-goal [goal]
  (let [target (bigdec (:target-amount goal))
        current (bigdec (:current-amount goal))
        pct (if (pos? (.compareTo target BigDecimal/ZERO))
              (-> (BigDecimal. 100)
                  (.multiply current)
                  (.divide target 1 RoundingMode/HALF_UP)
                  double)
              0.0)
        deadline (:deadline goal)
        days-remaining (when deadline
                         (let [today (LocalDate/now)
                               dl (if (instance? LocalDate deadline)
                                    deadline
                                    (LocalDate/parse (str deadline)))]
                           (max 0 (.between ChronoUnit/DAYS today dl))))]
    (assoc goal
           :percentage pct
           :days-remaining days-remaining)))

(defn- assert-owner! [goal user-id]
  (cond
    (nil? goal)
    (throw (ex-info "Not found" {:status 404
                                 :body {:error "not_found"
                                        :message "Goal not found"}}))
    (not= (:user-id goal) user-id)
    (throw (ex-info "Forbidden" {:status 403
                                 :body {:error "forbidden"
                                        :message "Goal belongs to another user"}}))))

(defn list-goals [user-id status-filter]
  (mapv enrich-goal (repo/find-by-user user-id status-filter)))

(defn create-goal! [user-id data]
  (let [goal-data (cond-> {:user-id user-id
                           :name (:name data)
                           :target-amount (:target-amount data)}
                    (:deadline data) (assoc :deadline (LocalDate/parse (:deadline data))))]
    (enrich-goal (repo/insert! goal-data))))

(defn update-goal! [user-id id data]
  (let [goal (repo/find-by-id id)]
    (assert-owner! goal user-id)
    (when (empty? (select-keys data [:name :target-amount :deadline]))
      (throw (ex-info "Empty update" {:status 400
                                      :body {:error "validation_error"
                                             :message "at least one field required"}})))
    (let [updates (cond-> {}
                    (:name data)          (assoc :name (:name data))
                    (:target-amount data) (assoc :target-amount (:target-amount data))
                    (contains? data :deadline)
                    (assoc :deadline (when (:deadline data)
                                      (LocalDate/parse (:deadline data)))))]
      (enrich-goal (repo/update! id updates)))))

(defn delete-goal! [user-id id]
  (let [goal (repo/find-by-id id)]
    (assert-owner! goal user-id)
    (repo/delete! id)
    nil))

(defn deposit! [user-id id data]
  (let [goal (repo/find-by-id id)]
    (assert-owner! goal user-id)
    (when (not= (:status goal) "active")
      (throw (ex-info "Goal not active" {:status 400
                                         :body {:error "bad_request"
                                                :message "Goal is not active"}})))
    (let [amount (bigdec (:amount data))
          new-amount (.add (bigdec (:current-amount goal)) amount)]
      (jdbc/with-transaction [tx @db/datasource]
        (repo/insert-deposit! tx {:goal-id id :amount amount :note (:note data)})
        (repo/update-amount! tx id new-amount)
        (when (>= (.compareTo new-amount (bigdec (:target-amount goal))) 0)
          (repo/update-status! tx id "reached")))
      (enrich-goal (repo/find-by-id id)))))

(defn withdraw! [user-id id data]
  (let [goal (repo/find-by-id id)]
    (assert-owner! goal user-id)
    (when (not= (:status goal) "active")
      (throw (ex-info "Goal not active" {:status 400
                                         :body {:error "bad_request"
                                                :message "Goal is not active"}})))
    (let [amount (bigdec (:amount data))
          current (bigdec (:current-amount goal))
          new-amount (.subtract current amount)]
      (when (neg? (.compareTo new-amount BigDecimal/ZERO))
        (throw (ex-info "Insufficient balance" {:status 400
                                                :body {:error "bad_request"
                                                       :message "Insufficient balance"}})))
      (jdbc/with-transaction [tx @db/datasource]
        (repo/insert-deposit! tx {:goal-id id :amount (.negate amount) :note (:note data)})
        (repo/update-amount! tx id new-amount))
      (enrich-goal (repo/find-by-id id)))))

(defn list-deposits [user-id id]
  (let [goal (repo/find-by-id id)]
    (assert-owner! goal user-id)
    (repo/find-deposits id)))
```

- [ ] **Step 5: Create goal handlers**

Create `src/cointrail/goals/handlers.clj`:
```clojure
(ns cointrail.goals.handlers
  (:require [cointrail.goals.service :as service]
            [cointrail.goals.spec :as goal-spec]
            [cointrail.auth.spec :refer [validate]]
            [cointrail.util :refer [parse-uuid]]))

(defn- user-id [request]
  (java.util.UUID/fromString (get-in request [:identity :user-id])))

(def ^:private invalid-id
  {:status 400 :body {:error "bad_request" :message "Invalid id"}})

(defn list-handler [request]
  (let [status-filter (get-in request [:params :status])]
    {:status 200
     :body {:data (service/list-goals (user-id request) status-filter)}}))

(defn create-handler [request]
  (let [body (:body request)]
    (if-let [errors (validate ::goal-spec/create-goal body)]
      {:status 400 :body errors}
      (try
        {:status 201
         :body (service/create-goal! (user-id request) body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn update-handler [request]
  (let [id (parse-uuid (get-in request [:params :id]))
        body (:body request)]
    (if (nil? id)
      invalid-id
      (try
        {:status 200
         :body (service/update-goal! (user-id request) id body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn delete-handler [request]
  (if-let [id (parse-uuid (get-in request [:params :id]))]
    (try
      (service/delete-goal! (user-id request) id)
      {:status 204 :body nil}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))
    invalid-id))

(defn deposit-handler [request]
  (let [id (parse-uuid (get-in request [:params :id]))
        body (:body request)]
    (if (nil? id)
      invalid-id
      (if-let [errors (validate ::goal-spec/deposit-request body)]
        {:status 400 :body errors}
        (try
          {:status 200
           :body (service/deposit! (user-id request) id body)}
          (catch clojure.lang.ExceptionInfo e
            (let [data (ex-data e)]
              {:status (:status data) :body (:body data)})))))))

(defn withdraw-handler [request]
  (let [id (parse-uuid (get-in request [:params :id]))
        body (:body request)]
    (if (nil? id)
      invalid-id
      (if-let [errors (validate ::goal-spec/deposit-request body)]
        {:status 400 :body errors}
        (try
          {:status 200
           :body (service/withdraw! (user-id request) id body)}
          (catch clojure.lang.ExceptionInfo e
            (let [data (ex-data e)]
              {:status (:status data) :body (:body data)})))))))

(defn deposits-handler [request]
  (if-let [id (parse-uuid (get-in request [:params :id]))]
    (try
      {:status 200
       :body {:data (service/list-deposits (user-id request) id)}}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))
    invalid-id))
```

- [ ] **Step 6: Add goal routes**

Modify `src/cointrail/routes.clj`:

Add to `:require`: `[cointrail.goals.handlers :as goals]`

Add routes inside `(context "/api" [] ...)`:
```clojure
    (context "/goals" []
      (mw/require-auth
       (routes
        (GET "/" [] goals/list-handler)
        (POST "/" [] goals/create-handler)
        (PUT "/:id" [] goals/update-handler)
        (DELETE "/:id" [] goals/delete-handler)
        (POST "/:id/deposit" [] goals/deposit-handler)
        (POST "/:id/withdraw" [] goals/withdraw-handler)
        (GET "/:id/deposits" [] goals/deposits-handler))))
```

- [ ] **Step 7: Update Swagger spec with goal endpoints**

Modify `src/cointrail/swagger.clj` — add to the `:paths` map:

```clojure
    "/goals"
    {:get {:tags ["Goals"] :summary "List savings goals"
           :security [{:Bearer []}]
           :parameters [{:in "query" :name "status" :type "string"
                         :enum ["active" "reached" "cancelled"]}]
           :responses {200 {:description "Goals with progress"}}}
     :post {:tags ["Goals"] :summary "Create savings goal"
            :security [{:Bearer []}]
            :parameters [{:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["name" "target_amount"]
                                   :properties {:name {:type "string" :maxLength 100}
                                                :target_amount {:type "number" :minimum 0.01}
                                                :deadline {:type "string" :format "date"}}}}]
            :responses {201 {:description "Goal created"}}}}
    "/goals/{id}"
    {:put {:tags ["Goals"] :summary "Update goal"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}
                        {:in "body" :name "body"
                         :schema {:type "object"
                                  :properties {:name {:type "string"}
                                               :target_amount {:type "number"}
                                               :deadline {:type "string" :format "date"}}}}]
           :responses {200 {:description "Updated"}}}
     :delete {:tags ["Goals"] :summary "Delete goal"
              :security [{:Bearer []}]
              :parameters [{:in "path" :name "id" :type "string" :required true}]
              :responses {204 {:description "Deleted"}}}}
    "/goals/{id}/deposit"
    {:post {:tags ["Goals"] :summary "Deposit into goal"
            :security [{:Bearer []}]
            :parameters [{:in "path" :name "id" :type "string" :required true}
                         {:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["amount"]
                                   :properties {:amount {:type "number" :minimum 0.01}
                                                :note {:type "string" :maxLength 255}}}}]
            :responses {200 {:description "Goal with updated balance"}}}}
    "/goals/{id}/withdraw"
    {:post {:tags ["Goals"] :summary "Withdraw from goal"
            :security [{:Bearer []}]
            :parameters [{:in "path" :name "id" :type "string" :required true}
                         {:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["amount"]
                                   :properties {:amount {:type "number" :minimum 0.01}
                                                :note {:type "string" :maxLength 255}}}}]
            :responses {200 {:description "Goal with updated balance"}}}}
    "/goals/{id}/deposits"
    {:get {:tags ["Goals"] :summary "List goal deposit/withdrawal history"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}]
           :responses {200 {:description "Deposit history"}}}}
```

- [ ] **Step 8: Write goal integration tests**

Create `test/cointrail/goals/handlers_test.clj`:
```clojure
(ns cointrail.goals.handlers-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (db/execute! ["DELETE FROM goal_deposits"])
    (db/execute! ["DELETE FROM goals"])
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

(deftest goal-crud-test
  (let [app (create-app (config))
        token (register-and-get-token app)]

    (testing "creates a goal"
      (let [response (app (auth-header
                           (json-request :post "/api/goals"
                                        {:name "Viagem Europa"
                                         :target_amount 15000.00
                                         :deadline "2027-06-01"})
                           token))
            body (parse-body response)]
        (is (= 201 (:status response)))
        (is (= "Viagem Europa" (:name body)))
        (is (= 15000.0 (:target-amount body)))
        (is (= 0 (:current-amount body)))
        (is (= 0.0 (:percentage body)))
        (is (= "active" (:status body)))
        (is (number? (:days-remaining body)))))

    (testing "lists goals"
      (let [response (app (auth-header (mock/request :get "/api/goals") token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 1 (count (:data body))))))

    (testing "filters by status"
      (let [response (app (auth-header
                           (mock/request :get "/api/goals?status=reached")
                           token))
            body (parse-body response)]
        (is (= 0 (count (:data body))))))

    (testing "updates a goal"
      (let [goals-resp (app (auth-header (mock/request :get "/api/goals") token))
            goal-id (:id (first (:data (parse-body goals-resp))))
            response (app (auth-header
                           (json-request :put (str "/api/goals/" goal-id)
                                        {:name "Viagem Japão"})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= "Viagem Japão" (:name body)))))

    (testing "deletes a goal"
      ;; Create a second goal to delete
      (app (auth-header (json-request :post "/api/goals"
                                      {:name "Carro" :target_amount 50000}) token))
      (let [goals-resp (app (auth-header (mock/request :get "/api/goals") token))
            goals (:data (parse-body goals-resp))
            delete-id (:id (second goals))
            response (app (auth-header
                           (mock/request :delete (str "/api/goals/" delete-id))
                           token))]
        (is (= 204 (:status response)))))))

(deftest goal-deposit-withdraw-test
  (let [app (create-app (config))
        token (register-and-get-token app)
        ;; Create goal
        create-resp (app (auth-header
                          (json-request :post "/api/goals"
                                       {:name "Emergency Fund" :target_amount 1000.00})
                          token))
        goal-id (:id (parse-body create-resp))]

    (testing "deposits into goal"
      (let [response (app (auth-header
                           (json-request :post (str "/api/goals/" goal-id "/deposit")
                                        {:amount 400.00 :note "First deposit"})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 400.0 (:current-amount body)))
        (is (= 40.0 (:percentage body)))
        (is (= "active" (:status body)))))

    (testing "withdraws from goal"
      (let [response (app (auth-header
                           (json-request :post (str "/api/goals/" goal-id "/withdraw")
                                        {:amount 100.00 :note "Emergency"})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 300.0 (:current-amount body)))))

    (testing "cannot withdraw more than balance"
      (let [response (app (auth-header
                           (json-request :post (str "/api/goals/" goal-id "/withdraw")
                                        {:amount 500.00})
                           token))]
        (is (= 400 (:status response)))))

    (testing "deposit history"
      (let [response (app (auth-header
                           (mock/request :get (str "/api/goals/" goal-id "/deposits"))
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 2 (count (:data body))))))

    (testing "goal auto-reaches when target met"
      (app (auth-header
            (json-request :post (str "/api/goals/" goal-id "/deposit")
                         {:amount 700.00})
            token))
      (let [response (app (auth-header
                           (mock/request :get "/api/goals?status=reached")
                           token))
            body (parse-body response)]
        (is (= 1 (count (:data body))))
        (is (= "reached" (:status (first (:data body)))))))

    (testing "cannot deposit into reached goal"
      (let [response (app (auth-header
                           (json-request :post (str "/api/goals/" goal-id "/deposit")
                                        {:amount 50.00})
                           token))]
        (is (= 400 (:status response)))))))
```

- [ ] **Step 9: Run tests and verify**

Run: `cd C:\Users\Mathe\source\cointrail && lein test cointrail.goals.handlers-test`
Expected: All tests pass.

- [ ] **Step 10: Commit**

```bash
git add src/cointrail/goals/ test/cointrail/goals/ resources/migrations/010-* resources/migrations/011-* src/cointrail/routes.clj src/cointrail/swagger.clj
git commit -m "feat: savings goals with deposit, withdraw, and auto-reach"
```

---

### Task 4: Recurring Transactions — Rules, Scheduler, and Auto-Generation

**Files:**
- Create: `src/cointrail/recurring/spec.clj`
- Create: `src/cointrail/recurring/repository.clj`
- Create: `src/cointrail/recurring/service.clj`
- Create: `src/cointrail/recurring/handlers.clj`
- Create: `src/cointrail/scheduler.clj`
- Create: `test/cointrail/recurring/handlers_test.clj`
- Create: `test/cointrail/scheduler_test.clj`
- Create: `resources/migrations/012-create-recurring-transactions.up.sql`
- Create: `resources/migrations/012-create-recurring-transactions.down.sql`
- Create: `resources/migrations/013-index-recurring-next-due.up.sql`
- Create: `resources/migrations/013-index-recurring-next-due.down.sql`
- Modify: `src/cointrail/core.clj`
- Modify: `src/cointrail/routes.clj`
- Modify: `src/cointrail/swagger.clj`

**Interfaces:**
- Consumes: `cointrail.db/execute!`, `cointrail.db/execute-one!`, `cointrail.db/datasource`, `cointrail.util/parse-uuid`, `cointrail.auth.spec/validate`, `next.jdbc/with-transaction`, `cointrail.transactions.repository/insert!`
- Produces:
  - `cointrail.recurring.repository/find-by-user [user-id active-filter]` → vec of maps
  - `cointrail.recurring.repository/find-by-id [id]` → map or nil
  - `cointrail.recurring.repository/insert! [rule]` → map
  - `cointrail.recurring.repository/update! [id updates]` → map
  - `cointrail.recurring.repository/deactivate! [id]` → nil
  - `cointrail.recurring.repository/find-due []` → vec of rules where `next_due_date <= today AND active`
  - `cointrail.recurring.repository/advance! [conn id next-due last-generated]` → nil
  - `cointrail.recurring.service/list-recurring [user-id active-filter]` → vec
  - `cointrail.recurring.service/create-recurring! [user-id data]` → map
  - `cointrail.recurring.service/update-recurring! [user-id id data]` → map
  - `cointrail.recurring.service/delete-recurring! [user-id id]` → nil (soft delete)
  - `cointrail.recurring.service/skip-next! [user-id id]` → map
  - `cointrail.scheduler/process-recurring!` → nil (processes all due rules)
  - `cointrail.scheduler/start!` → ScheduledExecutorService
  - `cointrail.scheduler/stop! [executor]` → nil

- [ ] **Step 1: Create recurring transaction migrations**

Create `resources/migrations/012-create-recurring-transactions.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS recurring_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    type VARCHAR(7) NOT NULL CHECK (type IN ('income', 'expense')),
    amount DECIMAL(12,2) NOT NULL CHECK (amount > 0),
    description VARCHAR(255) NOT NULL,
    frequency VARCHAR(10) NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly', 'yearly')),
    start_date DATE NOT NULL,
    end_date DATE,
    next_due_date DATE NOT NULL,
    last_generated_date DATE,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
```

Create `resources/migrations/012-create-recurring-transactions.down.sql`:
```sql
DROP TABLE IF EXISTS recurring_transactions;
```

Create `resources/migrations/013-index-recurring-next-due.up.sql`:
```sql
CREATE INDEX IF NOT EXISTS idx_recurring_next_due ON recurring_transactions(next_due_date) WHERE active = true;
```

Create `resources/migrations/013-index-recurring-next-due.down.sql`:
```sql
DROP INDEX IF EXISTS idx_recurring_next_due;
```

- [ ] **Step 2: Create recurring spec**

Create `src/cointrail/recurring/spec.clj`:
```clojure
(ns cointrail.recurring.spec
  (:require [clojure.spec.alpha :as s]))

(s/def ::type #{"income" "expense"})
(s/def ::amount (s/and number? pos?))
(s/def ::description (s/and string? #(>= (count %) 1) #(<= (count %) 255)))
(s/def ::category-id (s/nilable string?))
(s/def ::frequency #{"daily" "weekly" "monthly" "yearly"})
(s/def ::start-date (s/and string? #(re-matches #"\d{4}-\d{2}-\d{2}" %)))
(s/def ::end-date (s/nilable (s/and string? #(re-matches #"\d{4}-\d{2}-\d{2}" %))))

(s/def ::create-recurring (s/keys :req-un [::type ::amount ::description ::frequency ::start-date]
                                  :opt-un [::category-id ::end-date]))
(s/def ::update-recurring (s/keys :opt-un [::amount ::description ::category-id ::end-date]))
```

- [ ] **Step 3: Create recurring repository**

Create `src/cointrail/recurring/repository.clj`:
```clojure
(ns cointrail.recurring.repository
  (:require [cointrail.db :as db]
            [honey.sql :as sql]
            [honey.sql.helpers :as h])
  (:import [java.time LocalDate]))

(defn find-by-user
  ([user-id] (find-by-user user-id nil))
  ([user-id active-filter]
   (db/execute!
    (-> (h/select :r.* [:c.name :category-name] [:c.icon :category-icon])
        (h/from [:recurring-transactions :r])
        (h/left-join [:categories :c] [:= :r.category-id :c.id])
        (h/where [:= :r.user-id user-id])
        (cond->
          (some? active-filter) (h/where [:= :r.active active-filter]))
        (h/order-by [:r.created-at :desc])
        sql/format))))

(defn find-by-id [id]
  (db/execute-one!
   (-> (h/select :*)
       (h/from :recurring-transactions)
       (h/where [:= :id id])
       sql/format)))

(defn insert! [rule]
  (db/execute-one!
   (-> (h/insert-into :recurring-transactions)
       (h/values [rule])
       (h/returning :*)
       sql/format)))

(defn update! [id updates]
  (db/execute-one!
   (-> (h/update :recurring-transactions)
       (h/set (assoc updates :updated-at :%now))
       (h/where [:= :id id])
       (h/returning :*)
       sql/format)))

(defn deactivate! [id]
  (db/execute-one!
   (-> (h/update :recurring-transactions)
       (h/set {:active false :updated-at :%now})
       (h/where [:= :id id])
       sql/format)))

(defn find-due []
  (db/execute!
   (-> (h/select :*)
       (h/from :recurring-transactions)
       (h/where [:= :active true]
                [:<= :next-due-date (LocalDate/now)])
       sql/format)))

(defn advance! [conn id next-due last-generated]
  (db/execute-one! conn
   (-> (h/update :recurring-transactions)
       (h/set {:next-due-date next-due
               :last-generated-date last-generated
               :updated-at :%now})
       (h/where [:= :id id])
       sql/format)))

(defn deactivate-conn! [conn id]
  (db/execute-one! conn
   (-> (h/update :recurring-transactions)
       (h/set {:active false :updated-at :%now})
       (h/where [:= :id id])
       sql/format)))
```

- [ ] **Step 4: Create recurring service**

Create `src/cointrail/recurring/service.clj`:
```clojure
(ns cointrail.recurring.service
  (:require [cointrail.recurring.repository :as repo])
  (:import [java.time LocalDate]))

(defn- parse-active-filter [s]
  (case s
    "true"  true
    "false" false
    nil))

(defn- calc-next-due [^LocalDate current frequency]
  (case frequency
    "daily"   (.plusDays current 1)
    "weekly"  (.plusDays current 7)
    "monthly" (.plusMonths current 1)
    "yearly"  (.plusYears current 1)))

(defn- assert-owner! [rule user-id]
  (cond
    (nil? rule)
    (throw (ex-info "Not found" {:status 404
                                 :body {:error "not_found"
                                        :message "Recurring rule not found"}}))
    (not= (:user-id rule) user-id)
    (throw (ex-info "Forbidden" {:status 403
                                 :body {:error "forbidden"
                                        :message "Rule belongs to another user"}}))))

(defn list-recurring [user-id active-str]
  (repo/find-by-user user-id (parse-active-filter active-str)))

(defn create-recurring! [user-id data]
  (let [start (LocalDate/parse (:start-date data))
        end (when (:end-date data) (LocalDate/parse (:end-date data)))
        today (LocalDate/now)]
    (when (.isBefore start today)
      (throw (ex-info "Invalid start date" {:status 400
                                            :body {:error "bad_request"
                                                   :message "start_date must be today or in the future"}})))
    (when (and end (.isBefore end start))
      (throw (ex-info "Invalid end date" {:status 400
                                          :body {:error "bad_request"
                                                 :message "end_date must be after start_date"}})))
    (let [rule (cond-> {:user-id user-id
                        :type (:type data)
                        :amount (:amount data)
                        :description (:description data)
                        :frequency (:frequency data)
                        :start-date start
                        :next-due-date start}
                 (:category-id data) (assoc :category-id (java.util.UUID/fromString (:category-id data)))
                 end (assoc :end-date end))]
      (repo/insert! rule))))

(defn update-recurring! [user-id id data]
  (let [rule (repo/find-by-id id)]
    (assert-owner! rule user-id)
    (when (empty? (select-keys data [:amount :description :category-id :end-date]))
      (throw (ex-info "Empty update" {:status 400
                                      :body {:error "validation_error"
                                             :message "at least one field required"}})))
    (let [updates (cond-> {}
                    (:amount data) (assoc :amount (:amount data))
                    (:description data) (assoc :description (:description data))
                    (contains? data :category-id)
                    (assoc :category-id (when (:category-id data)
                                          (java.util.UUID/fromString (:category-id data))))
                    (contains? data :end-date)
                    (assoc :end-date (when (:end-date data)
                                      (LocalDate/parse (:end-date data)))))]
      (repo/update! id updates))))

(defn delete-recurring! [user-id id]
  (let [rule (repo/find-by-id id)]
    (assert-owner! rule user-id)
    (repo/deactivate! id)
    nil))

(defn skip-next! [user-id id]
  (let [rule (repo/find-by-id id)]
    (assert-owner! rule user-id)
    (when-not (:active rule)
      (throw (ex-info "Inactive" {:status 400
                                  :body {:error "bad_request"
                                         :message "Rule is not active"}})))
    (let [current (if (instance? LocalDate (:next-due-date rule))
                    (:next-due-date rule)
                    (LocalDate/parse (str (:next-due-date rule))))
          next-due (calc-next-due current (:frequency rule))
          end (:end-date rule)]
      (if (and end (.isAfter next-due (if (instance? LocalDate end) end (LocalDate/parse (str end)))))
        (do (repo/deactivate! id)
            (assoc (repo/find-by-id id) :active false))
        (repo/update! id {:next-due-date next-due})))))
```

- [ ] **Step 5: Create scheduler**

Create `src/cointrail/scheduler.clj`:
```clojure
(ns cointrail.scheduler
  (:require [cointrail.recurring.repository :as recur-repo]
            [cointrail.db :as db]
            [next.jdbc :as jdbc]
            [honey.sql :as sql]
            [honey.sql.helpers :as h]
            [taoensso.timbre :as log])
  (:import [java.time LocalDate]
           [java.util.concurrent Executors ScheduledExecutorService TimeUnit]))

(defn- calc-next-due [^LocalDate current frequency]
  (case frequency
    "daily"   (.plusDays current 1)
    "weekly"  (.plusDays current 7)
    "monthly" (.plusMonths current 1)
    "yearly"  (.plusYears current 1)))

(defn- to-local-date [d]
  (if (instance? LocalDate d) d (LocalDate/parse (str d))))

(defn- process-rule! [rule]
  (let [today (LocalDate/now)
        frequency (:frequency rule)]
    (loop [due-date (to-local-date (:next-due-date rule))]
      (when-not (.isAfter due-date today)
        (jdbc/with-transaction [tx @db/datasource]
          ;; Create the transaction
          (db/execute-one! tx
           (-> (h/insert-into :transactions)
               (h/values [{:user-id (:user-id rule)
                           :category-id (:category-id rule)
                           :type (:type rule)
                           :amount (:amount rule)
                           :description (:description rule)
                           :date due-date
                           :notes (str "Auto-generated from recurring: " (:description rule))}])
               sql/format))
          ;; Advance the rule
          (let [next-due (calc-next-due due-date frequency)
                end (:end-date rule)]
            (if (and end (.isAfter next-due (to-local-date end)))
              (do
                (recur-repo/advance! tx (:id rule) next-due due-date)
                (recur-repo/deactivate-conn! tx (:id rule)))
              (do
                (recur-repo/advance! tx (:id rule) next-due due-date)
                (recur next-due)))))))))

(defn process-recurring! []
  (let [due-rules (recur-repo/find-due)]
    (when (seq due-rules)
      (log/info "Processing" (count due-rules) "recurring transaction rules"))
    (doseq [rule due-rules]
      (try
        (process-rule! rule)
        (catch Exception e
          (log/error e "Failed to process recurring rule" (:id rule)))))))

(defn start! []
  (log/info "Starting recurring transaction scheduler (1h interval)")
  (let [^ScheduledExecutorService executor (Executors/newSingleThreadScheduledExecutor)]
    (.scheduleAtFixedRate executor
                         ^Runnable process-recurring!
                         0 1 TimeUnit/HOURS)
    executor))

(defn stop! [^ScheduledExecutorService executor]
  (when executor
    (log/info "Stopping recurring transaction scheduler")
    (.shutdown executor)))
```

- [ ] **Step 6: Create recurring handlers**

Create `src/cointrail/recurring/handlers.clj`:
```clojure
(ns cointrail.recurring.handlers
  (:require [cointrail.recurring.service :as service]
            [cointrail.recurring.spec :as recur-spec]
            [cointrail.auth.spec :refer [validate]]
            [cointrail.util :refer [parse-uuid]]))

(defn- user-id [request]
  (java.util.UUID/fromString (get-in request [:identity :user-id])))

(def ^:private invalid-id
  {:status 400 :body {:error "bad_request" :message "Invalid id"}})

(defn list-handler [request]
  (let [active (get-in request [:params :active])]
    {:status 200
     :body {:data (service/list-recurring (user-id request) active)}}))

(defn create-handler [request]
  (let [body (:body request)]
    (if-let [errors (validate ::recur-spec/create-recurring body)]
      {:status 400 :body errors}
      (try
        {:status 201
         :body (service/create-recurring! (user-id request) body)}
        (catch clojure.lang.ExceptionInfo e
          (let [data (ex-data e)]
            {:status (:status data) :body (:body data)}))))))

(defn update-handler [request]
  (let [id (parse-uuid (get-in request [:params :id]))
        body (:body request)]
    (if (nil? id)
      invalid-id
      (if-let [errors (validate ::recur-spec/update-recurring body)]
        {:status 400 :body errors}
        (try
          {:status 200
           :body (service/update-recurring! (user-id request) id body)}
          (catch clojure.lang.ExceptionInfo e
            (let [data (ex-data e)]
              {:status (:status data) :body (:body data)})))))))

(defn delete-handler [request]
  (if-let [id (parse-uuid (get-in request [:params :id]))]
    (try
      (service/delete-recurring! (user-id request) id)
      {:status 204 :body nil}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))
    invalid-id))

(defn skip-handler [request]
  (if-let [id (parse-uuid (get-in request [:params :id]))]
    (try
      {:status 200
       :body (service/skip-next! (user-id request) id)}
      (catch clojure.lang.ExceptionInfo e
        (let [data (ex-data e)]
          {:status (:status data) :body (:body data)})))
    invalid-id))
```

- [ ] **Step 7: Add recurring routes and integrate scheduler**

Modify `src/cointrail/routes.clj`:

Add to `:require`: `[cointrail.recurring.handlers :as recurring]`

Add routes inside `(context "/api" [] ...)`:
```clojure
    (context "/recurring" []
      (mw/require-auth
       (routes
        (GET "/" [] recurring/list-handler)
        (POST "/" [] recurring/create-handler)
        (PUT "/:id" [] recurring/update-handler)
        (DELETE "/:id" [] recurring/delete-handler)
        (POST "/:id/skip" [] recurring/skip-handler))))
```

Modify `src/cointrail/core.clj` to start/stop the scheduler:

Add to `:require`: `[cointrail.scheduler :as scheduler]`

Replace the `-main` function:
```clojure
(defn -main [& _args]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (let [app (create-app cfg)
          sched (scheduler/start!)]
      (.addShutdownHook (Runtime/getRuntime)
                        (Thread. ^Runnable (fn [] (scheduler/stop! sched))))
      (log/info "Starting CoinTrail on port" (:port cfg))
      (run-jetty app {:port (:port cfg) :join? true}))))
```

- [ ] **Step 8: Update Swagger spec with recurring endpoints**

Modify `src/cointrail/swagger.clj` — add to the `:paths` map:

```clojure
    "/recurring"
    {:get {:tags ["Recurring"] :summary "List recurring transaction rules"
           :security [{:Bearer []}]
           :parameters [{:in "query" :name "active" :type "string" :enum ["true" "false"]}]
           :responses {200 {:description "Recurring rules"}}}
     :post {:tags ["Recurring"] :summary "Create recurring transaction rule"
            :security [{:Bearer []}]
            :parameters [{:in "body" :name "body" :required true
                          :schema {:type "object"
                                   :required ["type" "amount" "description" "frequency" "start_date"]
                                   :properties {:type {:type "string" :enum ["income" "expense"]}
                                                :amount {:type "number" :minimum 0.01}
                                                :description {:type "string"}
                                                :category_id {:type "string"}
                                                :frequency {:type "string" :enum ["daily" "weekly" "monthly" "yearly"]}
                                                :start_date {:type "string" :format "date"}
                                                :end_date {:type "string" :format "date"}}}}]
            :responses {201 {:description "Rule created"}}}}
    "/recurring/{id}"
    {:put {:tags ["Recurring"] :summary "Update recurring rule"
           :security [{:Bearer []}]
           :parameters [{:in "path" :name "id" :type "string" :required true}
                        {:in "body" :name "body"
                         :schema {:type "object"
                                  :properties {:amount {:type "number"}
                                               :description {:type "string"}
                                               :category_id {:type "string"}
                                               :end_date {:type "string" :format "date"}}}}]
           :responses {200 {:description "Updated"}}}
     :delete {:tags ["Recurring"] :summary "Deactivate recurring rule"
              :security [{:Bearer []}]
              :parameters [{:in "path" :name "id" :type "string" :required true}]
              :responses {204 {:description "Deactivated"}}}}
    "/recurring/{id}/skip"
    {:post {:tags ["Recurring"] :summary "Skip next occurrence"
            :security [{:Bearer []}]
            :parameters [{:in "path" :name "id" :type "string" :required true}]
            :responses {200 {:description "Rule with advanced next_due_date"}}}}
```

Also update the Swagger `:info :version` from `"0.1.0"` to `"0.2.0"`.

- [ ] **Step 9: Write recurring CRUD integration tests**

Create `test/cointrail/recurring/handlers_test.clj`:
```clojure
(ns cointrail.recurring.handlers-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [ring.mock.request :as mock]
            [cheshire.core :as json])
  (:import [java.time LocalDate]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (db/execute! ["DELETE FROM recurring_transactions"])
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

(deftest recurring-crud-test
  (let [app (create-app (config))
        token (register-and-get-token app)
        tomorrow (str (.plusDays (LocalDate/now) 1))]

    (testing "creates recurring rule"
      (let [response (app (auth-header
                           (json-request :post "/api/recurring"
                                        {:type "expense"
                                         :amount 1500.00
                                         :description "Aluguel"
                                         :frequency "monthly"
                                         :start_date tomorrow})
                           token))
            body (parse-body response)]
        (is (= 201 (:status response)))
        (is (= "Aluguel" (:description body)))
        (is (= "monthly" (:frequency body)))
        (is (= true (:active body)))))

    (testing "lists recurring rules"
      (let [response (app (auth-header (mock/request :get "/api/recurring") token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 1 (count (:data body))))))

    (testing "rejects past start_date"
      (let [yesterday (str (.minusDays (LocalDate/now) 1))
            response (app (auth-header
                           (json-request :post "/api/recurring"
                                        {:type "income" :amount 5000
                                         :description "Salário" :frequency "monthly"
                                         :start_date yesterday})
                           token))]
        (is (= 400 (:status response)))))

    (testing "updates recurring rule"
      (let [rules-resp (app (auth-header (mock/request :get "/api/recurring") token))
            rule-id (:id (first (:data (parse-body rules-resp))))
            response (app (auth-header
                           (json-request :put (str "/api/recurring/" rule-id)
                                        {:amount 1600.00})
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (= 1600.0 (:amount body)))))

    (testing "skips next occurrence"
      (let [rules-resp (app (auth-header (mock/request :get "/api/recurring") token))
            rule (first (:data (parse-body rules-resp)))
            rule-id (:id rule)
            response (app (auth-header
                           (mock/request :post (str "/api/recurring/" rule-id "/skip"))
                           token))
            body (parse-body response)]
        (is (= 200 (:status response)))
        (is (not= (:next-due-date rule) (:next-due-date body)))))

    (testing "soft deletes (deactivates)"
      (let [rules-resp (app (auth-header (mock/request :get "/api/recurring") token))
            rule-id (:id (first (:data (parse-body rules-resp))))
            response (app (auth-header
                           (mock/request :delete (str "/api/recurring/" rule-id))
                           token))]
        (is (= 204 (:status response)))
        ;; Verify it's inactive
        (let [all-resp (app (auth-header
                             (mock/request :get "/api/recurring?active=false")
                             token))
              rules (:data (parse-body all-resp))]
          (is (= 1 (count rules)))
          (is (= false (:active (first rules)))))))))
```

- [ ] **Step 10: Write scheduler unit test**

Create `test/cointrail/scheduler_test.clj`:
```clojure
(ns cointrail.scheduler-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [cointrail.db :as db]
            [cointrail.config :refer [config]]
            [cointrail.core :refer [create-app]]
            [cointrail.scheduler :as scheduler]
            [ring.mock.request :as mock]
            [cheshire.core :as json]
            [honey.sql :as sql]
            [honey.sql.helpers :as h])
  (:import [java.time LocalDate]))

(defn db-fixture [f]
  (let [cfg (config)]
    (db/init! (:database-url cfg))
    (db/execute! ["DELETE FROM recurring_transactions"])
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

(deftest scheduler-processes-due-rules
  (let [app (create-app (config))
        token (register-and-get-token app)
        today (LocalDate/now)
        yesterday (.minusDays today 1)]

    ;; Insert a recurring rule with next_due_date = yesterday (bypassing API validation)
    ;; by inserting directly into DB
    (let [user-resp (app (auth-header (mock/request :get "/api/categories") token))
          cats (:data (parse-body user-resp))
          cat-id (:id (first (filter #(= "expense" (:type %)) cats)))
          ;; Get user-id from JWT
          user-id-str (-> token
                          (clojure.string/split #"\.")
                          second
                          (java.util.Base64/getDecoder)
                          (.decode)
                          (String.)
                          (json/parse-string true)
                          :user-id)]

      ;; Insert rule directly with past due date
      (db/execute!
       (-> (h/insert-into :recurring-transactions)
           (h/values [{:user-id (java.util.UUID/fromString user-id-str)
                       :category-id (java.util.UUID/fromString cat-id)
                       :type "expense"
                       :amount 100.00M
                       :description "Daily coffee"
                       :frequency "daily"
                       :start-date yesterday
                       :next-due-date yesterday
                       :active true}])
           sql/format))

      (testing "process-recurring! creates transactions for overdue dates"
        (scheduler/process-recurring!)

        ;; Should have created 2 transactions (yesterday and today)
        (let [txns-resp (app (auth-header (mock/request :get "/api/transactions") token))
              txns (:data (parse-body txns-resp))
              auto-txns (filter #(and (:notes %)
                                      (.contains (:notes %) "Auto-generated"))
                                txns)]
          (is (>= (count auto-txns) 1))
          (is (every? #(= 100.0 (:amount %)) auto-txns))))

      (testing "rule's next_due_date advanced past today"
        (let [rules (db/execute!
                     (-> (h/select :*)
                         (h/from :recurring-transactions)
                         sql/format))
              rule (first rules)
              next-due (if (instance? LocalDate (:next-due-date rule))
                         (:next-due-date rule)
                         (LocalDate/parse (str (:next-due-date rule))))]
          (is (.isAfter next-due today)))))))

(deftest scheduler-deactivates-on-end-date
  (let [app (create-app (config))
        token (register-and-get-token app)
        today (LocalDate/now)
        yesterday (.minusDays today 1)]

    (let [user-resp (app (auth-header (mock/request :get "/api/categories") token))
          cats (:data (parse-body user-resp))
          cat-id (:id (first (filter #(= "income" (:type %)) cats)))
          user-id-str (-> token
                          (clojure.string/split #"\.")
                          second
                          (java.util.Base64/getDecoder)
                          (.decode)
                          (String.)
                          (json/parse-string true)
                          :user-id)]

      (db/execute!
       (-> (h/insert-into :recurring-transactions)
           (h/values [{:user-id (java.util.UUID/fromString user-id-str)
                       :category-id (java.util.UUID/fromString cat-id)
                       :type "income"
                       :amount 500.00M
                       :description "One-time bonus"
                       :frequency "daily"
                       :start-date yesterday
                       :next-due-date yesterday
                       :end-date today
                       :active true}])
           sql/format))

      (testing "deactivates rule when next_due passes end_date"
        (scheduler/process-recurring!)

        (let [rules (db/execute!
                     (-> (h/select :*)
                         (h/from :recurring-transactions)
                         sql/format))
              rule (first rules)]
          (is (= false (:active rule))))))))
```

- [ ] **Step 11: Run all tests and verify**

Run: `cd C:\Users\Mathe\source\cointrail && lein test`
Expected: All existing and new tests pass.

- [ ] **Step 12: Commit**

```bash
git add src/cointrail/recurring/ src/cointrail/scheduler.clj test/cointrail/recurring/ test/cointrail/scheduler_test.clj resources/migrations/012-* resources/migrations/013-* src/cointrail/core.clj src/cointrail/routes.clj src/cointrail/swagger.clj
git commit -m "feat: recurring transactions with scheduled job and auto-generation"
```

---

### Task 5: Swagger Version Bump and Final Integration Verification

**Files:**
- Modify: `src/cointrail/swagger.clj` (version bump if not done in Task 4)
- Modify: `src/cointrail/routes.clj` (verify health handler version)

**Interfaces:**
- Consumes: All prior tasks' endpoints
- Produces: Nothing new — verification-only task

- [ ] **Step 1: Update API version in health handler**

Modify `src/cointrail/routes.clj` — change version in `health-handler` from `"0.1.0"` to `"0.2.0"`.

- [ ] **Step 2: Run full test suite**

Run: `cd C:\Users\Mathe\source\cointrail && lein test`
Expected: All tests pass (tags, budgets, goals, recurring, scheduler, plus all existing tests).

- [ ] **Step 3: Verify Docker build**

Run: `cd C:\Users\Mathe\source\cointrail && docker compose build`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add src/cointrail/routes.clj src/cointrail/swagger.clj
git commit -m "chore: bump API version to 0.2.0"
```
