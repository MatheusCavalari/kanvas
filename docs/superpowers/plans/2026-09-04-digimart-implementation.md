# DigiMart Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a digital products marketplace with .NET 9 microservices, Kafka event-driven communication, and a React + TypeScript frontend.

**Architecture:** 4 .NET 9 microservices (Identity, Catalog, Order, Notification) communicating asynchronously via Kafka. Each service owns its own PostgreSQL database. Redis for catalog caching. React SPA frontend talking directly to each service via HTTP. Everything runs locally via docker-compose.

**Tech Stack:** .NET 9, C#, PostgreSQL, Apache Kafka, Redis, xUnit, Testcontainers, React 18, TypeScript, Vite, Tailwind CSS, React Router v6, Playwright, GitHub Actions, Docker, Render

## Global Constraints

- .NET 9 (latest stable) for all backend services
- C# 13 with nullable reference types enabled, implicit usings enabled
- PostgreSQL 16 — one instance, separate databases per service (`digimart_identity`, `digimart_catalog`, `digimart_order`)
- Kafka via Confluent.Kafka NuGet package
- Redis via StackExchange.Redis
- EF Core 9 with code-first migrations for data access
- xUnit 2.9+ with Moq for unit tests, Testcontainers for integration tests
- JWT Bearer auth — shared signing key across services via config
- No API Gateway — frontend calls each service directly
- No cross-service foreign keys — services reference external IDs as plain UUIDs
- Node 20+, React 18, Vite 6, Tailwind CSS v4, React Router v6
- All commits in conventional-commit format (`feat:`, `fix:`, `test:`, `chore:`, `docs:`)

## Phases Overview

| Phase | Focus | Deliverable |
|-------|-------|-------------|
| **1** | Repo scaffold + Docker Compose + Shared library | All services boot, DB migrations run, Kafka connects |
| **2** | Identity Service | Register, login, JWT auth working end-to-end |
| **3** | Catalog Service | CRUD products + categories, Redis cache, seller auth |
| **4** | Order Service + Kafka events | Create order, simulate payment, publish events |
| **5** | Notification Service + event flow | Consume all Kafka events, log simulated emails |
| **6** | Catalog consumes PaymentConfirmed | ProductAccess + download endpoint, full event chain |
| **7** | Frontend — Auth + Catalog browsing | React app: register, login, browse products |
| **8** | Frontend — Checkout + Seller panel | Cart, checkout, order history, library, seller CRUD |
| **9** | CI/CD + E2E | GitHub Actions, Playwright tests, Docker CI |
| **10** | Deploy + README | Render deploy, professional README with screenshots |

---

## Phase 1: Repo Scaffold + Docker Compose + Shared Library

### Task 1: Initialize repository and solution structure

**Files:**
- Create: `backend/DigiMart.sln`
- Create: `backend/src/DigiMart.Identity/DigiMart.Identity.csproj`
- Create: `backend/src/DigiMart.Catalog/DigiMart.Catalog.csproj`
- Create: `backend/src/DigiMart.Order/DigiMart.Order.csproj`
- Create: `backend/src/DigiMart.Notification/DigiMart.Notification.csproj`
- Create: `backend/src/DigiMart.Shared/DigiMart.Shared.csproj`
- Create: `.gitignore`
- Create: `LICENSE`

**Interfaces:**
- Produces: Solution file referencing all 5 projects; each Web API project targets `net9.0`; Notification is a Worker Service; Shared is a class library referenced by all others.

- [ ] **Step 1: Create the repository root and .gitignore**

```bash
mkdir digimart
cd digimart
git init
```

Create `.gitignore`:
```gitignore
## .NET
bin/
obj/
*.user
*.suo
*.vs/
*.DotSettings.user
publish/

## Node
node_modules/
dist/
.env
.env.local

## IDE
.idea/
.vscode/

## OS
.DS_Store
Thumbs.db

## Docker
docker-compose.override.yml
```

Create `LICENSE` with MIT license, copyright `Matheus Cavalari`.

- [ ] **Step 2: Create the .NET solution and projects**

```bash
cd backend
dotnet new sln -n DigiMart

dotnet new webapi -n DigiMart.Identity -o src/DigiMart.Identity --no-openapi
dotnet new webapi -n DigiMart.Catalog -o src/DigiMart.Catalog --no-openapi
dotnet new webapi -n DigiMart.Order -o src/DigiMart.Order --no-openapi
dotnet new worker -n DigiMart.Notification -o src/DigiMart.Notification
dotnet new classlib -n DigiMart.Shared -o src/DigiMart.Shared

dotnet sln add src/DigiMart.Identity
dotnet sln add src/DigiMart.Catalog
dotnet sln add src/DigiMart.Order
dotnet sln add src/DigiMart.Notification
dotnet sln add src/DigiMart.Shared
```

- [ ] **Step 3: Add project references — every service references Shared**

```bash
cd backend
dotnet add src/DigiMart.Identity reference src/DigiMart.Shared
dotnet add src/DigiMart.Catalog reference src/DigiMart.Shared
dotnet add src/DigiMart.Order reference src/DigiMart.Shared
dotnet add src/DigiMart.Notification reference src/DigiMart.Shared
```

- [ ] **Step 4: Verify the solution builds**

```bash
cd backend
dotnet build
```

Expected: Build succeeded with 0 errors.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: initialize solution with 4 services + shared library"
```

---

### Task 2: Shared library — event contracts and common infrastructure

**Files:**
- Create: `backend/src/DigiMart.Shared/Events/OrderPlacedEvent.cs`
- Create: `backend/src/DigiMart.Shared/Events/PaymentConfirmedEvent.cs`
- Create: `backend/src/DigiMart.Shared/Events/AccessGrantedEvent.cs`
- Create: `backend/src/DigiMart.Shared/Events/OrderItemDto.cs`
- Create: `backend/src/DigiMart.Shared/Messaging/IEventPublisher.cs`
- Create: `backend/src/DigiMart.Shared/Messaging/KafkaEventPublisher.cs`
- Create: `backend/src/DigiMart.Shared/Messaging/KafkaSettings.cs`
- Create: `backend/src/DigiMart.Shared/Auth/JwtSettings.cs`
- Create: `backend/src/DigiMart.Shared/Auth/ServiceCollectionExtensions.cs`

**Interfaces:**
- Produces:
  - `OrderPlacedEvent { OrderId: Guid, BuyerId: Guid, Items: List<OrderItemDto>, TotalAmount: decimal, OccurredAt: DateTime }`
  - `PaymentConfirmedEvent { OrderId: Guid, BuyerId: Guid, Items: List<OrderItemDto>, OccurredAt: DateTime }`
  - `AccessGrantedEvent { BuyerId: Guid, ProductId: Guid, DownloadUrl: string, OccurredAt: DateTime }`
  - `OrderItemDto { ProductId: Guid, ProductTitle: string, UnitPrice: decimal }`
  - `IEventPublisher.PublishAsync<T>(string topic, string key, T @event, CancellationToken ct)`
  - `KafkaEventPublisher` — implementation using Confluent.Kafka, serializes to JSON
  - `KafkaSettings { BootstrapServers: string }` — bound from config section `"Kafka"`
  - `JwtSettings { Secret: string, Issuer: string, Audience: string, ExpirationMinutes: int }` — bound from `"Jwt"`
  - `AddJwtAuthentication(this IServiceCollection, IConfiguration)` — configures JWT Bearer validation

- [ ] **Step 1: Add NuGet packages to Shared**

```bash
cd backend
dotnet add src/DigiMart.Shared package Confluent.Kafka --version 2.8.0
dotnet add src/DigiMart.Shared package Microsoft.AspNetCore.Authentication.JwtBearer --version 9.0.0
dotnet add src/DigiMart.Shared package Microsoft.Extensions.Options.ConfigurationExtensions --version 9.0.0
dotnet add src/DigiMart.Shared package System.Text.Json --version 9.0.0
```

- [ ] **Step 2: Create event contracts**

Create `backend/src/DigiMart.Shared/Events/OrderItemDto.cs`:
```csharp
namespace DigiMart.Shared.Events;

public sealed record OrderItemDto(
    Guid ProductId,
    string ProductTitle,
    decimal UnitPrice);
```

Create `backend/src/DigiMart.Shared/Events/OrderPlacedEvent.cs`:
```csharp
namespace DigiMart.Shared.Events;

public sealed record OrderPlacedEvent(
    Guid OrderId,
    Guid BuyerId,
    List<OrderItemDto> Items,
    decimal TotalAmount,
    DateTime OccurredAt);
```

Create `backend/src/DigiMart.Shared/Events/PaymentConfirmedEvent.cs`:
```csharp
namespace DigiMart.Shared.Events;

public sealed record PaymentConfirmedEvent(
    Guid OrderId,
    Guid BuyerId,
    List<OrderItemDto> Items,
    DateTime OccurredAt);
```

Create `backend/src/DigiMart.Shared/Events/AccessGrantedEvent.cs`:
```csharp
namespace DigiMart.Shared.Events;

public sealed record AccessGrantedEvent(
    Guid BuyerId,
    Guid ProductId,
    string DownloadUrl,
    DateTime OccurredAt);
```

- [ ] **Step 3: Create Kafka publisher interface and implementation**

Create `backend/src/DigiMart.Shared/Messaging/KafkaSettings.cs`:
```csharp
namespace DigiMart.Shared.Messaging;

public sealed class KafkaSettings
{
    public string BootstrapServers { get; set; } = "localhost:9092";
}
```

Create `backend/src/DigiMart.Shared/Messaging/IEventPublisher.cs`:
```csharp
namespace DigiMart.Shared.Messaging;

public interface IEventPublisher
{
    Task PublishAsync<T>(string topic, string key, T @event, CancellationToken ct = default);
}
```

Create `backend/src/DigiMart.Shared/Messaging/KafkaEventPublisher.cs`:
```csharp
using System.Text.Json;
using Confluent.Kafka;
using Microsoft.Extensions.Options;

namespace DigiMart.Shared.Messaging;

public sealed class KafkaEventPublisher : IEventPublisher, IDisposable
{
    private readonly IProducer<string, string> _producer;

    public KafkaEventPublisher(IOptions<KafkaSettings> settings)
    {
        var config = new ProducerConfig
        {
            BootstrapServers = settings.Value.BootstrapServers,
            Acks = Acks.All
        };
        _producer = new ProducerBuilder<string, string>(config).Build();
    }

    public async Task PublishAsync<T>(string topic, string key, T @event, CancellationToken ct = default)
    {
        var json = JsonSerializer.Serialize(@event);
        var message = new Message<string, string> { Key = key, Value = json };
        await _producer.ProduceAsync(topic, message, ct);
    }

    public void Dispose() => _producer.Dispose();
}
```

- [ ] **Step 4: Create JWT auth configuration helper**

Create `backend/src/DigiMart.Shared/Auth/JwtSettings.cs`:
```csharp
namespace DigiMart.Shared.Auth;

public sealed class JwtSettings
{
    public string Secret { get; set; } = string.Empty;
    public string Issuer { get; set; } = "DigiMart";
    public string Audience { get; set; } = "DigiMart";
    public int ExpirationMinutes { get; set; } = 60;
    public int RefreshExpirationDays { get; set; } = 7;
}
```

Create `backend/src/DigiMart.Shared/Auth/ServiceCollectionExtensions.cs`:
```csharp
using System.Text;
using Microsoft.AspNetCore.Authentication.JwtBearer;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.IdentityModel.Tokens;

namespace DigiMart.Shared.Auth;

public static class ServiceCollectionExtensions
{
    public static IServiceCollection AddJwtAuthentication(
        this IServiceCollection services,
        IConfiguration configuration)
    {
        var jwtSettings = configuration.GetSection("Jwt").Get<JwtSettings>()!;
        services.Configure<JwtSettings>(configuration.GetSection("Jwt"));

        services.AddAuthentication(options =>
        {
            options.DefaultAuthenticateScheme = JwtBearerDefaults.AuthenticationScheme;
            options.DefaultChallengeScheme = JwtBearerDefaults.AuthenticationScheme;
        })
        .AddJwtBearer(options =>
        {
            options.TokenValidationParameters = new TokenValidationParameters
            {
                ValidateIssuer = true,
                ValidateAudience = true,
                ValidateLifetime = true,
                ValidateIssuerSigningKey = true,
                ValidIssuer = jwtSettings.Issuer,
                ValidAudience = jwtSettings.Audience,
                IssuerSigningKey = new SymmetricSecurityKey(
                    Encoding.UTF8.GetBytes(jwtSettings.Secret)),
                ClockSkew = TimeSpan.Zero
            };
        });

        services.AddAuthorization();
        return services;
    }
}
```

- [ ] **Step 5: Verify build**

```bash
cd backend
dotnet build
```

Expected: Build succeeded.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(shared): add Kafka event contracts, publisher, and JWT auth config"
```

---

### Task 3: Docker Compose for local development

**Files:**
- Create: `docker-compose.yml`
- Create: `backend/src/DigiMart.Identity/Dockerfile`
- Create: `backend/src/DigiMart.Catalog/Dockerfile`
- Create: `backend/src/DigiMart.Order/Dockerfile`
- Create: `backend/src/DigiMart.Notification/Dockerfile`

**Interfaces:**
- Produces: `docker-compose up` brings up PostgreSQL (port 5432), Kafka + Zookeeper (port 9092), Redis (port 6379), and all 4 .NET services (ports 5001-5004).

- [ ] **Step 1: Create Dockerfiles for each service**

Create `backend/src/DigiMart.Identity/Dockerfile`:
```dockerfile
FROM mcr.microsoft.com/dotnet/aspnet:9.0 AS base
WORKDIR /app
EXPOSE 8080

FROM mcr.microsoft.com/dotnet/sdk:9.0 AS build
WORKDIR /src
COPY ["src/DigiMart.Shared/DigiMart.Shared.csproj", "src/DigiMart.Shared/"]
COPY ["src/DigiMart.Identity/DigiMart.Identity.csproj", "src/DigiMart.Identity/"]
RUN dotnet restore "src/DigiMart.Identity/DigiMart.Identity.csproj"
COPY . .
WORKDIR "/src/src/DigiMart.Identity"
RUN dotnet publish -c Release -o /app/publish

FROM base AS final
WORKDIR /app
COPY --from=build /app/publish .
ENTRYPOINT ["dotnet", "DigiMart.Identity.dll"]
```

Create `backend/src/DigiMart.Catalog/Dockerfile` — same structure, replacing `Identity` with `Catalog` in all paths.

Create `backend/src/DigiMart.Order/Dockerfile` — same structure, replacing `Identity` with `Order` in all paths.

Create `backend/src/DigiMart.Notification/Dockerfile` — same structure, replacing `Identity` with `Notification` in all paths, and removing the `EXPOSE 8080` line (worker has no HTTP port).

- [ ] **Step 2: Create docker-compose.yml**

Create `docker-compose.yml` at repo root:
```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: digimart
      POSTGRES_PASSWORD: digimart
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./scripts/init-databases.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U digimart"]
      interval: 5s
      timeout: 3s
      retries: 5

  zookeeper:
    image: confluentinc/cp-zookeeper:7.7.0
    environment:
      ZOOKEEPER_CLIENT_PORT: 2181
      ZOOKEEPER_TICK_TIME: 2000

  kafka:
    image: confluentinc/cp-kafka:7.7.0
    depends_on:
      - zookeeper
    ports:
      - "9092:9092"
    environment:
      KAFKA_BROKER_ID: 1
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:29092,PLAINTEXT_HOST://localhost:9092
      KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT
      KAFKA_INTER_BROKER_LISTENER_NAME: PLAINTEXT
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
      KAFKA_AUTO_CREATE_TOPICS_ENABLE: "true"
    healthcheck:
      test: ["CMD", "kafka-broker-api-versions", "--bootstrap-server", "localhost:9092"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  identity:
    build:
      context: ./backend
      dockerfile: src/DigiMart.Identity/Dockerfile
    ports:
      - "5001:8080"
    environment:
      ConnectionStrings__DefaultConnection: "Host=postgres;Database=digimart_identity;Username=digimart;Password=digimart"
      Jwt__Secret: "super-secret-key-for-dev-min-32-chars!!"
      Jwt__Issuer: "DigiMart"
      Jwt__Audience: "DigiMart"
      Kafka__BootstrapServers: "kafka:29092"
    depends_on:
      postgres:
        condition: service_healthy
      kafka:
        condition: service_healthy

  catalog:
    build:
      context: ./backend
      dockerfile: src/DigiMart.Catalog/Dockerfile
    ports:
      - "5002:8080"
    environment:
      ConnectionStrings__DefaultConnection: "Host=postgres;Database=digimart_catalog;Username=digimart;Password=digimart"
      Jwt__Secret: "super-secret-key-for-dev-min-32-chars!!"
      Jwt__Issuer: "DigiMart"
      Jwt__Audience: "DigiMart"
      Kafka__BootstrapServers: "kafka:29092"
      Redis__ConnectionString: "redis:6379"
    depends_on:
      postgres:
        condition: service_healthy
      kafka:
        condition: service_healthy
      redis:
        condition: service_healthy

  order:
    build:
      context: ./backend
      dockerfile: src/DigiMart.Order/Dockerfile
    ports:
      - "5003:8080"
    environment:
      ConnectionStrings__DefaultConnection: "Host=postgres;Database=digimart_order;Username=digimart;Password=digimart"
      Jwt__Secret: "super-secret-key-for-dev-min-32-chars!!"
      Jwt__Issuer: "DigiMart"
      Jwt__Audience: "DigiMart"
      Kafka__BootstrapServers: "kafka:29092"
    depends_on:
      postgres:
        condition: service_healthy
      kafka:
        condition: service_healthy

  notification:
    build:
      context: ./backend
      dockerfile: src/DigiMart.Notification/Dockerfile
    environment:
      Kafka__BootstrapServers: "kafka:29092"
    depends_on:
      kafka:
        condition: service_healthy

volumes:
  postgres_data:
```

- [ ] **Step 3: Create database init script**

Create `scripts/init-databases.sql`:
```sql
CREATE DATABASE digimart_identity;
CREATE DATABASE digimart_catalog;
CREATE DATABASE digimart_order;
```

- [ ] **Step 4: Add appsettings.Development.json to each service**

Create `backend/src/DigiMart.Identity/appsettings.Development.json`:
```json
{
  "Logging": {
    "LogLevel": {
      "Default": "Information",
      "Microsoft.AspNetCore": "Warning"
    }
  },
  "ConnectionStrings": {
    "DefaultConnection": "Host=localhost;Database=digimart_identity;Username=digimart;Password=digimart"
  },
  "Jwt": {
    "Secret": "super-secret-key-for-dev-min-32-chars!!",
    "Issuer": "DigiMart",
    "Audience": "DigiMart",
    "ExpirationMinutes": 60,
    "RefreshExpirationDays": 7
  },
  "Kafka": {
    "BootstrapServers": "localhost:9092"
  }
}
```

Create equivalent files for Catalog (add `"Redis": { "ConnectionString": "localhost:6379" }`), Order (same as Identity minus Redis), and Notification (only Logging + Kafka sections).

- [ ] **Step 5: Verify docker-compose syntax**

```bash
docker compose config
```

Expected: Valid YAML output with no errors.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore: add Docker Compose with PostgreSQL, Kafka, Redis, and service Dockerfiles"
```

---

## Phase 2: Identity Service

### Task 4: Identity domain and data layer

**Files:**
- Create: `backend/src/DigiMart.Identity/Domain/User.cs`
- Create: `backend/src/DigiMart.Identity/Domain/Role.cs`
- Create: `backend/src/DigiMart.Identity/Data/IdentityDbContext.cs`
- Create: `backend/src/DigiMart.Identity/Data/Migrations/` (auto-generated)
- Create: `backend/tests/DigiMart.Identity.Tests/DigiMart.Identity.Tests.csproj`
- Create: `backend/tests/DigiMart.Identity.Tests/Domain/UserTests.cs`

**Interfaces:**
- Produces:
  - `User { Id: Guid, Email: string, PasswordHash: string, FullName: string, Role: Role, CreatedAt: DateTime, UpdatedAt: DateTime }` with `Create(email, passwordHash, fullName, role)` factory and `UpdatePassword(hash)` method
  - `Role` enum: `Buyer = 0, Seller = 1, Admin = 2`
  - `IdentityDbContext` with `DbSet<User> Users`

- [ ] **Step 1: Add EF Core packages to Identity**

```bash
cd backend
dotnet add src/DigiMart.Identity package Npgsql.EntityFrameworkCore.PostgreSQL --version 9.0.0
dotnet add src/DigiMart.Identity package Microsoft.EntityFrameworkCore.Design --version 9.0.0
```

- [ ] **Step 2: Create test project**

```bash
cd backend
dotnet new xunit -n DigiMart.Identity.Tests -o tests/DigiMart.Identity.Tests
dotnet sln add tests/DigiMart.Identity.Tests
dotnet add tests/DigiMart.Identity.Tests reference src/DigiMart.Identity
dotnet add tests/DigiMart.Identity.Tests reference src/DigiMart.Shared
dotnet add tests/DigiMart.Identity.Tests package Moq --version 4.20.72
```

- [ ] **Step 3: Write failing test for User domain entity**

Create `backend/tests/DigiMart.Identity.Tests/Domain/UserTests.cs`:
```csharp
using DigiMart.Identity.Domain;

namespace DigiMart.Identity.Tests.Domain;

public class UserTests
{
    [Fact]
    public void Create_WithValidData_ReturnsUser()
    {
        var user = User.Create("test@example.com", "hashed", "Test User", Role.Buyer);

        Assert.NotEqual(Guid.Empty, user.Id);
        Assert.Equal("test@example.com", user.Email);
        Assert.Equal("hashed", user.PasswordHash);
        Assert.Equal("Test User", user.FullName);
        Assert.Equal(Role.Buyer, user.Role);
        Assert.True(user.CreatedAt <= DateTime.UtcNow);
    }

    [Theory]
    [InlineData("")]
    [InlineData(null)]
    public void Create_WithEmptyEmail_ThrowsArgumentException(string? email)
    {
        Assert.Throws<ArgumentException>(() =>
            User.Create(email!, "hashed", "Test User", Role.Buyer));
    }

    [Fact]
    public void Create_AsSeller_HasSellerRole()
    {
        var user = User.Create("seller@example.com", "hashed", "Seller", Role.Seller);
        Assert.Equal(Role.Seller, user.Role);
    }
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
cd backend
dotnet test tests/DigiMart.Identity.Tests --filter "UserTests" -v n
```

Expected: FAIL — `User` class does not exist.

- [ ] **Step 5: Implement domain entities**

Create `backend/src/DigiMart.Identity/Domain/Role.cs`:
```csharp
namespace DigiMart.Identity.Domain;

public enum Role
{
    Buyer = 0,
    Seller = 1,
    Admin = 2
}
```

Create `backend/src/DigiMart.Identity/Domain/User.cs`:
```csharp
namespace DigiMart.Identity.Domain;

public sealed class User
{
    public Guid Id { get; private set; }
    public string Email { get; private set; } = string.Empty;
    public string PasswordHash { get; private set; } = string.Empty;
    public string FullName { get; private set; } = string.Empty;
    public Role Role { get; private set; }
    public DateTime CreatedAt { get; private set; }
    public DateTime UpdatedAt { get; private set; }

    private User() { }

    public static User Create(string email, string passwordHash, string fullName, Role role)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(email);
        ArgumentException.ThrowIfNullOrWhiteSpace(passwordHash);
        ArgumentException.ThrowIfNullOrWhiteSpace(fullName);

        var now = DateTime.UtcNow;
        return new User
        {
            Id = Guid.NewGuid(),
            Email = email.ToLowerInvariant(),
            PasswordHash = passwordHash,
            FullName = fullName,
            Role = role,
            CreatedAt = now,
            UpdatedAt = now
        };
    }

    public void UpdatePassword(string newHash)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(newHash);
        PasswordHash = newHash;
        UpdatedAt = DateTime.UtcNow;
    }
}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
cd backend
dotnet test tests/DigiMart.Identity.Tests --filter "UserTests" -v n
```

Expected: All 3 tests PASS.

- [ ] **Step 7: Create EF Core DbContext**

Create `backend/src/DigiMart.Identity/Data/IdentityDbContext.cs`:
```csharp
using DigiMart.Identity.Domain;
using Microsoft.EntityFrameworkCore;

namespace DigiMart.Identity.Data;

public sealed class IdentityDbContext : DbContext
{
    public DbSet<User> Users => Set<User>();

    public IdentityDbContext(DbContextOptions<IdentityDbContext> options) : base(options) { }

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        modelBuilder.Entity<User>(entity =>
        {
            entity.HasKey(u => u.Id);
            entity.HasIndex(u => u.Email).IsUnique();
            entity.Property(u => u.Email).HasMaxLength(256).IsRequired();
            entity.Property(u => u.PasswordHash).IsRequired();
            entity.Property(u => u.FullName).HasMaxLength(200).IsRequired();
            entity.Property(u => u.Role).HasConversion<string>().HasMaxLength(20);
        });
    }
}
```

- [ ] **Step 8: Generate initial migration**

```bash
cd backend/src/DigiMart.Identity
dotnet ef migrations add InitialCreate --output-dir Data/Migrations
```

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat(identity): add User domain entity, DbContext, and initial migration"
```

---

### Task 5: Identity API endpoints (register, login, refresh, me)

**Files:**
- Create: `backend/src/DigiMart.Identity/Services/IAuthService.cs`
- Create: `backend/src/DigiMart.Identity/Services/AuthService.cs`
- Create: `backend/src/DigiMart.Identity/Services/ITokenService.cs`
- Create: `backend/src/DigiMart.Identity/Services/TokenService.cs`
- Create: `backend/src/DigiMart.Identity/Endpoints/AuthEndpoints.cs`
- Create: `backend/src/DigiMart.Identity/Contracts/RegisterRequest.cs`
- Create: `backend/src/DigiMart.Identity/Contracts/LoginRequest.cs`
- Create: `backend/src/DigiMart.Identity/Contracts/AuthResponse.cs`
- Create: `backend/src/DigiMart.Identity/Contracts/UserResponse.cs`
- Modify: `backend/src/DigiMart.Identity/Program.cs`
- Create: `backend/tests/DigiMart.Identity.Tests/Services/AuthServiceTests.cs`

**Interfaces:**
- Consumes: `User.Create()`, `IdentityDbContext`, `JwtSettings`, `AddJwtAuthentication()`
- Produces:
  - `IAuthService.RegisterAsync(RegisterRequest, CancellationToken) → AuthResponse`
  - `IAuthService.LoginAsync(LoginRequest, CancellationToken) → AuthResponse`
  - `IAuthService.RefreshAsync(string refreshToken, CancellationToken) → AuthResponse`
  - `ITokenService.GenerateAccessToken(User) → string`
  - `ITokenService.GenerateRefreshToken() → string`
  - `RegisterRequest { Email, Password, FullName, Role }`
  - `LoginRequest { Email, Password }`
  - `AuthResponse { AccessToken, RefreshToken, ExpiresAt }`
  - `UserResponse { Id, Email, FullName, Role }`
  - Endpoints: `POST /api/auth/register`, `POST /api/auth/login`, `POST /api/auth/refresh`, `GET /api/auth/me`

- [ ] **Step 1: Create request/response contracts**

Create `backend/src/DigiMart.Identity/Contracts/RegisterRequest.cs`:
```csharp
using System.ComponentModel.DataAnnotations;

namespace DigiMart.Identity.Contracts;

public sealed record RegisterRequest(
    [Required, EmailAddress] string Email,
    [Required, MinLength(6)] string Password,
    [Required] string FullName,
    [Required] string Role);
```

Create `backend/src/DigiMart.Identity/Contracts/LoginRequest.cs`:
```csharp
using System.ComponentModel.DataAnnotations;

namespace DigiMart.Identity.Contracts;

public sealed record LoginRequest(
    [Required, EmailAddress] string Email,
    [Required] string Password);
```

Create `backend/src/DigiMart.Identity/Contracts/AuthResponse.cs`:
```csharp
namespace DigiMart.Identity.Contracts;

public sealed record AuthResponse(
    string AccessToken,
    string RefreshToken,
    DateTime ExpiresAt);
```

Create `backend/src/DigiMart.Identity/Contracts/UserResponse.cs`:
```csharp
namespace DigiMart.Identity.Contracts;

public sealed record UserResponse(
    Guid Id,
    string Email,
    string FullName,
    string Role);
```

- [ ] **Step 2: Create TokenService**

Create `backend/src/DigiMart.Identity/Services/ITokenService.cs`:
```csharp
using DigiMart.Identity.Domain;

namespace DigiMart.Identity.Services;

public interface ITokenService
{
    string GenerateAccessToken(User user);
    string GenerateRefreshToken();
}
```

Create `backend/src/DigiMart.Identity/Services/TokenService.cs`:
```csharp
using System.IdentityModel.Tokens.Jwt;
using System.Security.Claims;
using System.Security.Cryptography;
using System.Text;
using DigiMart.Identity.Domain;
using DigiMart.Shared.Auth;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Tokens;

namespace DigiMart.Identity.Services;

public sealed class TokenService : ITokenService
{
    private readonly JwtSettings _settings;

    public TokenService(IOptions<JwtSettings> settings)
    {
        _settings = settings.Value;
    }

    public string GenerateAccessToken(User user)
    {
        var key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(_settings.Secret));
        var credentials = new SigningCredentials(key, SecurityAlgorithms.HmacSha256);

        var claims = new[]
        {
            new Claim(JwtRegisteredClaimNames.Sub, user.Id.ToString()),
            new Claim(JwtRegisteredClaimNames.Email, user.Email),
            new Claim(ClaimTypes.Name, user.FullName),
            new Claim(ClaimTypes.Role, user.Role.ToString()),
            new Claim(JwtRegisteredClaimNames.Jti, Guid.NewGuid().ToString())
        };

        var token = new JwtSecurityToken(
            issuer: _settings.Issuer,
            audience: _settings.Audience,
            claims: claims,
            expires: DateTime.UtcNow.AddMinutes(_settings.ExpirationMinutes),
            signingCredentials: credentials);

        return new JwtSecurityTokenHandler().WriteToken(token);
    }

    public string GenerateRefreshToken()
    {
        var randomBytes = RandomNumberGenerator.GetBytes(64);
        return Convert.ToBase64String(randomBytes);
    }
}
```

- [ ] **Step 3: Write failing test for AuthService**

Create `backend/tests/DigiMart.Identity.Tests/Services/AuthServiceTests.cs`:
```csharp
using DigiMart.Identity.Contracts;
using DigiMart.Identity.Data;
using DigiMart.Identity.Domain;
using DigiMart.Identity.Services;
using Microsoft.EntityFrameworkCore;
using Moq;

namespace DigiMart.Identity.Tests.Services;

public class AuthServiceTests
{
    private readonly IdentityDbContext _db;
    private readonly Mock<ITokenService> _tokenService;
    private readonly AuthService _sut;

    public AuthServiceTests()
    {
        var options = new DbContextOptionsBuilder<IdentityDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        _db = new IdentityDbContext(options);
        _tokenService = new Mock<ITokenService>();
        _tokenService.Setup(t => t.GenerateAccessToken(It.IsAny<User>())).Returns("access-token");
        _tokenService.Setup(t => t.GenerateRefreshToken()).Returns("refresh-token");
        _sut = new AuthService(_db, _tokenService.Object);
    }

    [Fact]
    public async Task Register_WithValidData_CreatesUserAndReturnsTokens()
    {
        var request = new RegisterRequest("test@example.com", "password123", "Test User", "Buyer");

        var result = await _sut.RegisterAsync(request, CancellationToken.None);

        Assert.Equal("access-token", result.AccessToken);
        Assert.Equal("refresh-token", result.RefreshToken);
        Assert.Single(_db.Users);
    }

    [Fact]
    public async Task Register_WithDuplicateEmail_ThrowsInvalidOperationException()
    {
        var request = new RegisterRequest("dup@example.com", "password123", "User", "Buyer");
        await _sut.RegisterAsync(request, CancellationToken.None);

        await Assert.ThrowsAsync<InvalidOperationException>(
            () => _sut.RegisterAsync(request, CancellationToken.None));
    }

    [Fact]
    public async Task Login_WithValidCredentials_ReturnsTokens()
    {
        await _sut.RegisterAsync(
            new RegisterRequest("login@example.com", "password123", "User", "Buyer"),
            CancellationToken.None);

        var result = await _sut.LoginAsync(
            new LoginRequest("login@example.com", "password123"),
            CancellationToken.None);

        Assert.Equal("access-token", result.AccessToken);
    }

    [Fact]
    public async Task Login_WithWrongPassword_ThrowsUnauthorizedAccessException()
    {
        await _sut.RegisterAsync(
            new RegisterRequest("wrong@example.com", "password123", "User", "Buyer"),
            CancellationToken.None);

        await Assert.ThrowsAsync<UnauthorizedAccessException>(
            () => _sut.LoginAsync(
                new LoginRequest("wrong@example.com", "bad-password"),
                CancellationToken.None));
    }
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
cd backend
dotnet test tests/DigiMart.Identity.Tests --filter "AuthServiceTests" -v n
```

Expected: FAIL — `AuthService` does not exist.

- [ ] **Step 5: Implement AuthService**

Add BCrypt package:
```bash
dotnet add src/DigiMart.Identity package BCrypt.Net-Next --version 4.0.3
dotnet add tests/DigiMart.Identity.Tests package Microsoft.EntityFrameworkCore.InMemory --version 9.0.0
```

Create `backend/src/DigiMart.Identity/Services/IAuthService.cs`:
```csharp
using DigiMart.Identity.Contracts;

namespace DigiMart.Identity.Services;

public interface IAuthService
{
    Task<AuthResponse> RegisterAsync(RegisterRequest request, CancellationToken ct);
    Task<AuthResponse> LoginAsync(LoginRequest request, CancellationToken ct);
    Task<AuthResponse> RefreshAsync(string refreshToken, CancellationToken ct);
}
```

Create `backend/src/DigiMart.Identity/Services/AuthService.cs`:
```csharp
using DigiMart.Identity.Contracts;
using DigiMart.Identity.Data;
using DigiMart.Identity.Domain;
using Microsoft.EntityFrameworkCore;

namespace DigiMart.Identity.Services;

public sealed class AuthService : IAuthService
{
    private readonly IdentityDbContext _db;
    private readonly ITokenService _tokenService;

    public AuthService(IdentityDbContext db, ITokenService tokenService)
    {
        _db = db;
        _tokenService = tokenService;
    }

    public async Task<AuthResponse> RegisterAsync(RegisterRequest request, CancellationToken ct)
    {
        var exists = await _db.Users.AnyAsync(u => u.Email == request.Email.ToLowerInvariant(), ct);
        if (exists)
            throw new InvalidOperationException("Email already registered.");

        if (!Enum.TryParse<Role>(request.Role, true, out var role) || role == Role.Admin)
            throw new ArgumentException("Invalid role. Use Buyer or Seller.");

        var passwordHash = BCrypt.Net.BCrypt.HashPassword(request.Password);
        var user = User.Create(request.Email, passwordHash, request.FullName, role);

        _db.Users.Add(user);
        await _db.SaveChangesAsync(ct);

        var accessToken = _tokenService.GenerateAccessToken(user);
        var refreshToken = _tokenService.GenerateRefreshToken();

        return new AuthResponse(accessToken, refreshToken, DateTime.UtcNow.AddHours(1));
    }

    public async Task<AuthResponse> LoginAsync(LoginRequest request, CancellationToken ct)
    {
        var user = await _db.Users.FirstOrDefaultAsync(
            u => u.Email == request.Email.ToLowerInvariant(), ct);

        if (user is null || !BCrypt.Net.BCrypt.Verify(request.Password, user.PasswordHash))
            throw new UnauthorizedAccessException("Invalid email or password.");

        var accessToken = _tokenService.GenerateAccessToken(user);
        var refreshToken = _tokenService.GenerateRefreshToken();

        return new AuthResponse(accessToken, refreshToken, DateTime.UtcNow.AddHours(1));
    }

    public async Task<AuthResponse> RefreshAsync(string refreshToken, CancellationToken ct)
    {
        // Simplified: in production, refresh tokens would be stored and validated
        throw new NotImplementedException("Refresh token validation not implemented in MVP.");
    }
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd backend
dotnet test tests/DigiMart.Identity.Tests --filter "AuthServiceTests" -v n
```

Expected: All 4 tests PASS.

- [ ] **Step 7: Wire up Program.cs and API endpoints**

Create `backend/src/DigiMart.Identity/Endpoints/AuthEndpoints.cs`:
```csharp
using System.Security.Claims;
using DigiMart.Identity.Contracts;
using DigiMart.Identity.Data;
using DigiMart.Identity.Services;
using Microsoft.EntityFrameworkCore;

namespace DigiMart.Identity.Endpoints;

public static class AuthEndpoints
{
    public static void MapAuthEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/auth");

        group.MapPost("/register", async (RegisterRequest request, IAuthService authService, CancellationToken ct) =>
        {
            try
            {
                var response = await authService.RegisterAsync(request, ct);
                return Results.Ok(response);
            }
            catch (InvalidOperationException ex)
            {
                return Results.Conflict(new { error = ex.Message });
            }
            catch (ArgumentException ex)
            {
                return Results.BadRequest(new { error = ex.Message });
            }
        });

        group.MapPost("/login", async (LoginRequest request, IAuthService authService, CancellationToken ct) =>
        {
            try
            {
                var response = await authService.LoginAsync(request, ct);
                return Results.Ok(response);
            }
            catch (UnauthorizedAccessException)
            {
                return Results.Unauthorized();
            }
        });

        group.MapPost("/refresh", async (RefreshRequest request, IAuthService authService, CancellationToken ct) =>
        {
            try
            {
                var response = await authService.RefreshAsync(request.RefreshToken, ct);
                return Results.Ok(response);
            }
            catch (UnauthorizedAccessException)
            {
                return Results.Unauthorized();
            }
        });

        group.MapGet("/me", async (ClaimsPrincipal principal, IdentityDbContext db, CancellationToken ct) =>
        {
            var userId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var user = await db.Users.FindAsync([userId], ct);
            if (user is null) return Results.NotFound();

            return Results.Ok(new UserResponse(user.Id, user.Email, user.FullName, user.Role.ToString()));
        }).RequireAuthorization();
    }
}

public sealed record RefreshRequest(string RefreshToken);
```

Replace `backend/src/DigiMart.Identity/Program.cs`:
```csharp
using DigiMart.Identity.Data;
using DigiMart.Identity.Endpoints;
using DigiMart.Identity.Services;
using DigiMart.Shared.Auth;
using Microsoft.EntityFrameworkCore;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddDbContext<IdentityDbContext>(options =>
    options.UseNpgsql(builder.Configuration.GetConnectionString("DefaultConnection")));

builder.Services.AddJwtAuthentication(builder.Configuration);
builder.Services.AddScoped<IAuthService, AuthService>();
builder.Services.AddScoped<ITokenService, TokenService>();

builder.Services.AddCors(options =>
{
    options.AddDefaultPolicy(policy =>
        policy.AllowAnyOrigin().AllowAnyMethod().AllowAnyHeader());
});

var app = builder.Build();

using (var scope = app.Services.CreateScope())
{
    var db = scope.ServiceProvider.GetRequiredService<IdentityDbContext>();
    await db.Database.MigrateAsync();
}

app.UseCors();
app.UseAuthentication();
app.UseAuthorization();
app.MapAuthEndpoints();

app.Run();
```

- [ ] **Step 8: Verify build**

```bash
cd backend
dotnet build
```

Expected: Build succeeded.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat(identity): add auth endpoints — register, login, refresh, me"
```

---

## Phase 3: Catalog Service

### Task 6: Catalog domain, data layer, and CRUD endpoints

**Files:**
- Create: `backend/src/DigiMart.Catalog/Domain/Product.cs`
- Create: `backend/src/DigiMart.Catalog/Domain/Category.cs`
- Create: `backend/src/DigiMart.Catalog/Domain/ProductStatus.cs`
- Create: `backend/src/DigiMart.Catalog/Domain/ProductAccess.cs`
- Create: `backend/src/DigiMart.Catalog/Data/CatalogDbContext.cs`
- Create: `backend/src/DigiMart.Catalog/Contracts/*.cs` (request/response DTOs)
- Create: `backend/src/DigiMart.Catalog/Endpoints/ProductEndpoints.cs`
- Create: `backend/src/DigiMart.Catalog/Endpoints/CategoryEndpoints.cs`
- Modify: `backend/src/DigiMart.Catalog/Program.cs`
- Create: `backend/tests/DigiMart.Catalog.Tests/DigiMart.Catalog.Tests.csproj`
- Create: `backend/tests/DigiMart.Catalog.Tests/Domain/ProductTests.cs`

**Interfaces:**
- Consumes: `AddJwtAuthentication()`, `JwtSettings`
- Produces:
  - `Product { Id, SellerId, CategoryId, Title, Slug, Description, Price, FileUrl, ThumbnailUrl, Status, CreatedAt, UpdatedAt }` with `Create()` factory
  - `Category { Id, Name, Slug, Description }`
  - `ProductAccess { Id, ProductId, BuyerId, OrderId, GrantedAt, ExpiresAt }`
  - `ProductStatus` enum: `Draft, Active, Inactive`
  - `CatalogDbContext` with `DbSet<Product>`, `DbSet<Category>`, `DbSet<ProductAccess>`
  - Endpoints: full CRUD as per spec (GET/POST/PUT/DELETE products, GET categories, GET purchases, GET download)

- [ ] **Step 1: Add packages and create test project**

```bash
cd backend
dotnet add src/DigiMart.Catalog package Npgsql.EntityFrameworkCore.PostgreSQL --version 9.0.0
dotnet add src/DigiMart.Catalog package Microsoft.EntityFrameworkCore.Design --version 9.0.0
dotnet add src/DigiMart.Catalog package StackExchange.Redis --version 2.8.16

dotnet new xunit -n DigiMart.Catalog.Tests -o tests/DigiMart.Catalog.Tests
dotnet sln add tests/DigiMart.Catalog.Tests
dotnet add tests/DigiMart.Catalog.Tests reference src/DigiMart.Catalog
dotnet add tests/DigiMart.Catalog.Tests reference src/DigiMart.Shared
dotnet add tests/DigiMart.Catalog.Tests package Microsoft.EntityFrameworkCore.InMemory --version 9.0.0
dotnet add tests/DigiMart.Catalog.Tests package Moq --version 4.20.72
```

- [ ] **Step 2: Write failing test for Product domain entity**

Create `backend/tests/DigiMart.Catalog.Tests/Domain/ProductTests.cs`:
```csharp
using DigiMart.Catalog.Domain;

namespace DigiMart.Catalog.Tests.Domain;

public class ProductTests
{
    [Fact]
    public void Create_WithValidData_ReturnsProduct()
    {
        var sellerId = Guid.NewGuid();
        var categoryId = Guid.NewGuid();

        var product = Product.Create(sellerId, categoryId, "My Course", "Learn C#", 29.90m, "https://files.example.com/course.zip", "https://img.example.com/thumb.jpg");

        Assert.NotEqual(Guid.Empty, product.Id);
        Assert.Equal(sellerId, product.SellerId);
        Assert.Equal("My Course", product.Title);
        Assert.Equal("my-course", product.Slug);
        Assert.Equal(29.90m, product.Price);
        Assert.Equal(ProductStatus.Draft, product.Status);
    }

    [Fact]
    public void Create_WithNegativePrice_ThrowsArgumentException()
    {
        Assert.Throws<ArgumentException>(() =>
            Product.Create(Guid.NewGuid(), Guid.NewGuid(), "Test", "Desc", -1m, "url", "thumb"));
    }

    [Fact]
    public void Activate_ChangesStatusToActive()
    {
        var product = Product.Create(Guid.NewGuid(), Guid.NewGuid(), "Test", "Desc", 10m, "url", "thumb");
        product.Activate();
        Assert.Equal(ProductStatus.Active, product.Status);
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd backend
dotnet test tests/DigiMart.Catalog.Tests --filter "ProductTests" -v n
```

Expected: FAIL — `Product` class does not exist.

- [ ] **Step 4: Implement domain entities**

Create `backend/src/DigiMart.Catalog/Domain/ProductStatus.cs`:
```csharp
namespace DigiMart.Catalog.Domain;

public enum ProductStatus
{
    Draft = 0,
    Active = 1,
    Inactive = 2
}
```

Create `backend/src/DigiMart.Catalog/Domain/Category.cs`:
```csharp
namespace DigiMart.Catalog.Domain;

public sealed class Category
{
    public Guid Id { get; private set; }
    public string Name { get; private set; } = string.Empty;
    public string Slug { get; private set; } = string.Empty;
    public string Description { get; private set; } = string.Empty;

    private Category() { }

    public static Category Create(string name, string description)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(name);
        return new Category
        {
            Id = Guid.NewGuid(),
            Name = name,
            Slug = name.ToLowerInvariant().Replace(' ', '-'),
            Description = description
        };
    }
}
```

Create `backend/src/DigiMart.Catalog/Domain/Product.cs`:
```csharp
using System.Text.RegularExpressions;

namespace DigiMart.Catalog.Domain;

public sealed partial class Product
{
    public Guid Id { get; private set; }
    public Guid SellerId { get; private set; }
    public Guid CategoryId { get; private set; }
    public string Title { get; private set; } = string.Empty;
    public string Slug { get; private set; } = string.Empty;
    public string Description { get; private set; } = string.Empty;
    public decimal Price { get; private set; }
    public string FileUrl { get; private set; } = string.Empty;
    public string ThumbnailUrl { get; private set; } = string.Empty;
    public ProductStatus Status { get; private set; }
    public DateTime CreatedAt { get; private set; }
    public DateTime UpdatedAt { get; private set; }

    public Category? Category { get; private set; }

    private Product() { }

    public static Product Create(
        Guid sellerId, Guid categoryId, string title, string description,
        decimal price, string fileUrl, string thumbnailUrl)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(title);
        if (price < 0) throw new ArgumentException("Price must be non-negative.", nameof(price));

        var now = DateTime.UtcNow;
        return new Product
        {
            Id = Guid.NewGuid(),
            SellerId = sellerId,
            CategoryId = categoryId,
            Title = title,
            Slug = GenerateSlug(title),
            Description = description,
            Price = price,
            FileUrl = fileUrl,
            ThumbnailUrl = thumbnailUrl,
            Status = ProductStatus.Draft,
            CreatedAt = now,
            UpdatedAt = now
        };
    }

    public void Update(string title, string description, decimal price, Guid categoryId, string fileUrl, string thumbnailUrl)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(title);
        if (price < 0) throw new ArgumentException("Price must be non-negative.", nameof(price));

        Title = title;
        Slug = GenerateSlug(title);
        Description = description;
        Price = price;
        CategoryId = categoryId;
        FileUrl = fileUrl;
        ThumbnailUrl = thumbnailUrl;
        UpdatedAt = DateTime.UtcNow;
    }

    public void Activate() { Status = ProductStatus.Active; UpdatedAt = DateTime.UtcNow; }
    public void Deactivate() { Status = ProductStatus.Inactive; UpdatedAt = DateTime.UtcNow; }

    private static string GenerateSlug(string title) =>
        SlugRegex().Replace(title.ToLowerInvariant().Replace(' ', '-'), "");

    [GeneratedRegex("[^a-z0-9-]")]
    private static partial Regex SlugRegex();
}
```

Create `backend/src/DigiMart.Catalog/Domain/ProductAccess.cs`:
```csharp
namespace DigiMart.Catalog.Domain;

public sealed class ProductAccess
{
    public Guid Id { get; private set; }
    public Guid ProductId { get; private set; }
    public Guid BuyerId { get; private set; }
    public Guid OrderId { get; private set; }
    public DateTime GrantedAt { get; private set; }
    public DateTime? ExpiresAt { get; private set; }

    private ProductAccess() { }

    public static ProductAccess Grant(Guid productId, Guid buyerId, Guid orderId, DateTime? expiresAt = null)
    {
        return new ProductAccess
        {
            Id = Guid.NewGuid(),
            ProductId = productId,
            BuyerId = buyerId,
            OrderId = orderId,
            GrantedAt = DateTime.UtcNow,
            ExpiresAt = expiresAt
        };
    }

    public bool IsValid() => ExpiresAt is null || ExpiresAt > DateTime.UtcNow;
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd backend
dotnet test tests/DigiMart.Catalog.Tests --filter "ProductTests" -v n
```

Expected: All 3 tests PASS.

- [ ] **Step 6: Create DbContext, contracts, endpoints, and Program.cs**

Create `backend/src/DigiMart.Catalog/Data/CatalogDbContext.cs`:
```csharp
using DigiMart.Catalog.Domain;
using Microsoft.EntityFrameworkCore;

namespace DigiMart.Catalog.Data;

public sealed class CatalogDbContext : DbContext
{
    public DbSet<Product> Products => Set<Product>();
    public DbSet<Category> Categories => Set<Category>();
    public DbSet<ProductAccess> ProductAccesses => Set<ProductAccess>();

    public CatalogDbContext(DbContextOptions<CatalogDbContext> options) : base(options) { }

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        modelBuilder.Entity<Product>(entity =>
        {
            entity.HasKey(p => p.Id);
            entity.HasIndex(p => p.Slug).IsUnique();
            entity.Property(p => p.Title).HasMaxLength(300).IsRequired();
            entity.Property(p => p.Slug).HasMaxLength(300).IsRequired();
            entity.Property(p => p.Price).HasColumnType("decimal(18,2)");
            entity.Property(p => p.Status).HasConversion<string>().HasMaxLength(20);
            entity.HasOne(p => p.Category).WithMany().HasForeignKey(p => p.CategoryId);
        });

        modelBuilder.Entity<Category>(entity =>
        {
            entity.HasKey(c => c.Id);
            entity.HasIndex(c => c.Slug).IsUnique();
            entity.Property(c => c.Name).HasMaxLength(100).IsRequired();
            entity.Property(c => c.Slug).HasMaxLength(100).IsRequired();
        });

        modelBuilder.Entity<ProductAccess>(entity =>
        {
            entity.HasKey(pa => pa.Id);
            entity.HasIndex(pa => new { pa.ProductId, pa.BuyerId }).IsUnique();
        });
    }
}
```

Create `backend/src/DigiMart.Catalog/Contracts/CreateProductRequest.cs`:
```csharp
using System.ComponentModel.DataAnnotations;

namespace DigiMart.Catalog.Contracts;

public sealed record CreateProductRequest(
    [Required] string Title,
    [Required] string Description,
    [Required, Range(0, double.MaxValue)] decimal Price,
    [Required] Guid CategoryId,
    [Required] string FileUrl,
    string? ThumbnailUrl);

public sealed record UpdateProductRequest(
    [Required] string Title,
    [Required] string Description,
    [Required, Range(0, double.MaxValue)] decimal Price,
    [Required] Guid CategoryId,
    [Required] string FileUrl,
    string? ThumbnailUrl);

public sealed record ProductResponse(
    Guid Id,
    Guid SellerId,
    string Title,
    string Slug,
    string Description,
    decimal Price,
    string ThumbnailUrl,
    string Status,
    string CategoryName,
    DateTime CreatedAt);

public sealed record ProductDetailResponse(
    Guid Id,
    Guid SellerId,
    string Title,
    string Slug,
    string Description,
    decimal Price,
    string FileUrl,
    string ThumbnailUrl,
    string Status,
    Guid CategoryId,
    string CategoryName,
    DateTime CreatedAt,
    DateTime UpdatedAt);

public sealed record ProductListResponse(
    List<ProductResponse> Items,
    int TotalCount,
    int Page,
    int PageSize);

public sealed record PurchasedProductResponse(
    Guid ProductId,
    string Title,
    string ThumbnailUrl,
    DateTime GrantedAt);
```

Create `backend/src/DigiMart.Catalog/Endpoints/CategoryEndpoints.cs`:
```csharp
using DigiMart.Catalog.Data;
using DigiMart.Catalog.Domain;
using Microsoft.EntityFrameworkCore;

namespace DigiMart.Catalog.Endpoints;

public static class CategoryEndpoints
{
    public static void MapCategoryEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/catalog/categories");

        group.MapGet("/", async (CatalogDbContext db, CancellationToken ct) =>
        {
            var categories = await db.Categories
                .OrderBy(c => c.Name)
                .Select(c => new { c.Id, c.Name, c.Slug, c.Description })
                .ToListAsync(ct);
            return Results.Ok(categories);
        });
    }
}
```

Create `backend/src/DigiMart.Catalog/Endpoints/ProductEndpoints.cs`:
```csharp
using System.Security.Claims;
using DigiMart.Catalog.Contracts;
using DigiMart.Catalog.Data;
using DigiMart.Catalog.Domain;
using Microsoft.EntityFrameworkCore;

namespace DigiMart.Catalog.Endpoints;

public static class ProductEndpoints
{
    public static void MapProductEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/catalog");

        group.MapGet("/products", async (
            CatalogDbContext db, CancellationToken ct,
            string? search, Guid? categoryId, int page = 1, int pageSize = 12) =>
        {
            var query = db.Products
                .Include(p => p.Category)
                .Where(p => p.Status == ProductStatus.Active)
                .AsQueryable();

            if (!string.IsNullOrWhiteSpace(search))
                query = query.Where(p => p.Title.Contains(search) || p.Description.Contains(search));
            if (categoryId.HasValue)
                query = query.Where(p => p.CategoryId == categoryId.Value);

            var totalCount = await query.CountAsync(ct);
            var items = await query
                .OrderByDescending(p => p.CreatedAt)
                .Skip((page - 1) * pageSize)
                .Take(pageSize)
                .Select(p => new ProductResponse(
                    p.Id, p.SellerId, p.Title, p.Slug, p.Description,
                    p.Price, p.ThumbnailUrl, p.Status.ToString(),
                    p.Category!.Name, p.CreatedAt))
                .ToListAsync(ct);

            return Results.Ok(new ProductListResponse(items, totalCount, page, pageSize));
        });

        group.MapGet("/products/{slug}", async (string slug, CatalogDbContext db, CancellationToken ct) =>
        {
            var product = await db.Products
                .Include(p => p.Category)
                .FirstOrDefaultAsync(p => p.Slug == slug, ct);
            if (product is null) return Results.NotFound();

            return Results.Ok(new ProductDetailResponse(
                product.Id, product.SellerId, product.Title, product.Slug,
                product.Description, product.Price, product.FileUrl,
                product.ThumbnailUrl, product.Status.ToString(),
                product.CategoryId, product.Category!.Name,
                product.CreatedAt, product.UpdatedAt));
        });

        group.MapPost("/products", async (
            CreateProductRequest request, ClaimsPrincipal principal,
            CatalogDbContext db, CancellationToken ct) =>
        {
            var sellerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var product = Product.Create(
                sellerId, request.CategoryId, request.Title, request.Description,
                request.Price, request.FileUrl, request.ThumbnailUrl ?? "");
            product.Activate();

            db.Products.Add(product);
            await db.SaveChangesAsync(ct);

            return Results.Created($"/api/catalog/products/{product.Slug}",
                new { product.Id, product.Slug });
        }).RequireAuthorization(policy => policy.RequireRole("Seller"));

        group.MapPut("/products/{id:guid}", async (
            Guid id, UpdateProductRequest request, ClaimsPrincipal principal,
            CatalogDbContext db, CancellationToken ct) =>
        {
            var sellerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var product = await db.Products.FindAsync([id], ct);
            if (product is null) return Results.NotFound();
            if (product.SellerId != sellerId) return Results.Forbid();

            product.Update(request.Title, request.Description, request.Price,
                request.CategoryId, request.FileUrl, request.ThumbnailUrl ?? "");
            await db.SaveChangesAsync(ct);

            return Results.NoContent();
        }).RequireAuthorization(policy => policy.RequireRole("Seller"));

        group.MapDelete("/products/{id:guid}", async (
            Guid id, ClaimsPrincipal principal, CatalogDbContext db, CancellationToken ct) =>
        {
            var sellerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var product = await db.Products.FindAsync([id], ct);
            if (product is null) return Results.NotFound();
            if (product.SellerId != sellerId) return Results.Forbid();

            db.Products.Remove(product);
            await db.SaveChangesAsync(ct);

            return Results.NoContent();
        }).RequireAuthorization(policy => policy.RequireRole("Seller"));

        group.MapGet("/products/my", async (
            ClaimsPrincipal principal, CatalogDbContext db, CancellationToken ct) =>
        {
            var sellerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var products = await db.Products
                .Include(p => p.Category)
                .Where(p => p.SellerId == sellerId)
                .OrderByDescending(p => p.CreatedAt)
                .Select(p => new ProductResponse(
                    p.Id, p.SellerId, p.Title, p.Slug, p.Description,
                    p.Price, p.ThumbnailUrl, p.Status.ToString(),
                    p.Category!.Name, p.CreatedAt))
                .ToListAsync(ct);

            return Results.Ok(products);
        }).RequireAuthorization(policy => policy.RequireRole("Seller"));

        group.MapGet("/purchases", async (
            ClaimsPrincipal principal, CatalogDbContext db, CancellationToken ct) =>
        {
            var buyerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var purchases = await db.ProductAccesses
                .Where(pa => pa.BuyerId == buyerId && (pa.ExpiresAt == null || pa.ExpiresAt > DateTime.UtcNow))
                .Join(db.Products, pa => pa.ProductId, p => p.Id,
                    (pa, p) => new PurchasedProductResponse(p.Id, p.Title, p.ThumbnailUrl, pa.GrantedAt))
                .ToListAsync(ct);

            return Results.Ok(purchases);
        }).RequireAuthorization();

        group.MapGet("/purchases/{productId:guid}/download", async (
            Guid productId, ClaimsPrincipal principal, CatalogDbContext db, CancellationToken ct) =>
        {
            var buyerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var access = await db.ProductAccesses
                .FirstOrDefaultAsync(pa => pa.ProductId == productId && pa.BuyerId == buyerId, ct);

            if (access is null || !access.IsValid())
                return Results.Forbid();

            var product = await db.Products.FindAsync([productId], ct);
            if (product is null) return Results.NotFound();

            return Results.Ok(new { product.FileUrl });
        }).RequireAuthorization();
    }
}
```

Replace `backend/src/DigiMart.Catalog/Program.cs`:
```csharp
using DigiMart.Catalog.Data;
using DigiMart.Catalog.Endpoints;
using DigiMart.Shared.Auth;
using DigiMart.Shared.Messaging;
using Microsoft.EntityFrameworkCore;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddDbContext<CatalogDbContext>(options =>
    options.UseNpgsql(builder.Configuration.GetConnectionString("DefaultConnection")));

builder.Services.AddJwtAuthentication(builder.Configuration);
builder.Services.Configure<KafkaSettings>(builder.Configuration.GetSection("Kafka"));
builder.Services.AddSingleton<IEventPublisher, KafkaEventPublisher>();

builder.Services.AddCors(options =>
{
    options.AddDefaultPolicy(policy =>
        policy.AllowAnyOrigin().AllowAnyMethod().AllowAnyHeader());
});

var app = builder.Build();

using (var scope = app.Services.CreateScope())
{
    var db = scope.ServiceProvider.GetRequiredService<CatalogDbContext>();
    await db.Database.MigrateAsync();
}

app.UseCors();
app.UseAuthentication();
app.UseAuthorization();
app.MapProductEndpoints();
app.MapCategoryEndpoints();

app.Run();
```

- [ ] **Step 7: Generate migration and verify build**

```bash
cd backend/src/DigiMart.Catalog
dotnet ef migrations add InitialCreate --output-dir Data/Migrations
cd ../..
dotnet build
```

Expected: Build succeeded.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(catalog): add products CRUD, categories, purchases, and download endpoints"
```

---

## Phase 4: Order Service + Kafka Events

### Task 7: Order domain, data layer, and endpoints with Kafka publishing

**Files:**
- Create: `backend/src/DigiMart.Order/Domain/Order.cs`
- Create: `backend/src/DigiMart.Order/Domain/OrderItem.cs`
- Create: `backend/src/DigiMart.Order/Domain/OrderStatus.cs`
- Create: `backend/src/DigiMart.Order/Data/OrderDbContext.cs`
- Create: `backend/src/DigiMart.Order/Contracts/*.cs`
- Create: `backend/src/DigiMart.Order/Endpoints/OrderEndpoints.cs`
- Modify: `backend/src/DigiMart.Order/Program.cs`
- Create: `backend/tests/DigiMart.Order.Tests/DigiMart.Order.Tests.csproj`
- Create: `backend/tests/DigiMart.Order.Tests/Domain/OrderTests.cs`

**Interfaces:**
- Consumes: `IEventPublisher.PublishAsync()`, `OrderPlacedEvent`, `PaymentConfirmedEvent`, `AddJwtAuthentication()`
- Produces:
  - `Order { Id, BuyerId, Status, TotalAmount, CreatedAt, UpdatedAt, Items }` with `Create()`, `ConfirmPayment()`
  - `OrderItem { Id, OrderId, ProductId, ProductTitle, UnitPrice, Quantity }`
  - `OrderStatus` enum: `Pending, Confirmed, Cancelled`
  - Endpoints: `POST /api/orders`, `POST /api/orders/{id}/pay`, `GET /api/orders`, `GET /api/orders/{id}`

- [ ] **Step 1: Add packages and create test project**

```bash
cd backend
dotnet add src/DigiMart.Order package Npgsql.EntityFrameworkCore.PostgreSQL --version 9.0.0
dotnet add src/DigiMart.Order package Microsoft.EntityFrameworkCore.Design --version 9.0.0

dotnet new xunit -n DigiMart.Order.Tests -o tests/DigiMart.Order.Tests
dotnet sln add tests/DigiMart.Order.Tests
dotnet add tests/DigiMart.Order.Tests reference src/DigiMart.Order
dotnet add tests/DigiMart.Order.Tests reference src/DigiMart.Shared
dotnet add tests/DigiMart.Order.Tests package Microsoft.EntityFrameworkCore.InMemory --version 9.0.0
dotnet add tests/DigiMart.Order.Tests package Moq --version 4.20.72
```

- [ ] **Step 2: Write failing test for Order domain entity**

Create `backend/tests/DigiMart.Order.Tests/Domain/OrderTests.cs`:
```csharp
using DigiMart.Order.Domain;

namespace DigiMart.Order.Tests.Domain;

public class OrderTests
{
    [Fact]
    public void Create_WithItems_CalculatesTotalAmount()
    {
        var items = new List<(Guid ProductId, string Title, decimal Price)>
        {
            (Guid.NewGuid(), "Course A", 29.90m),
            (Guid.NewGuid(), "Template B", 15.00m)
        };

        var order = Domain.Order.Create(Guid.NewGuid(), items);

        Assert.Equal(44.90m, order.TotalAmount);
        Assert.Equal(OrderStatus.Pending, order.Status);
        Assert.Equal(2, order.Items.Count);
    }

    [Fact]
    public void Create_WithEmptyItems_ThrowsArgumentException()
    {
        Assert.Throws<ArgumentException>(() =>
            Domain.Order.Create(Guid.NewGuid(), new List<(Guid, string, decimal)>()));
    }

    [Fact]
    public void ConfirmPayment_ChangesStatusToConfirmed()
    {
        var items = new List<(Guid ProductId, string Title, decimal Price)>
        {
            (Guid.NewGuid(), "Course", 10m)
        };
        var order = Domain.Order.Create(Guid.NewGuid(), items);

        order.ConfirmPayment();

        Assert.Equal(OrderStatus.Confirmed, order.Status);
    }

    [Fact]
    public void ConfirmPayment_WhenAlreadyConfirmed_ThrowsInvalidOperationException()
    {
        var items = new List<(Guid ProductId, string Title, decimal Price)>
        {
            (Guid.NewGuid(), "Course", 10m)
        };
        var order = Domain.Order.Create(Guid.NewGuid(), items);
        order.ConfirmPayment();

        Assert.Throws<InvalidOperationException>(() => order.ConfirmPayment());
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd backend
dotnet test tests/DigiMart.Order.Tests --filter "OrderTests" -v n
```

Expected: FAIL — `Order` class does not exist.

- [ ] **Step 4: Implement domain entities**

Create `backend/src/DigiMart.Order/Domain/OrderStatus.cs`:
```csharp
namespace DigiMart.Order.Domain;

public enum OrderStatus
{
    Pending = 0,
    Confirmed = 1,
    Cancelled = 2
}
```

Create `backend/src/DigiMart.Order/Domain/OrderItem.cs`:
```csharp
namespace DigiMart.Order.Domain;

public sealed class OrderItem
{
    public Guid Id { get; private set; }
    public Guid OrderId { get; private set; }
    public Guid ProductId { get; private set; }
    public string ProductTitle { get; private set; } = string.Empty;
    public decimal UnitPrice { get; private set; }
    public int Quantity { get; private set; }

    private OrderItem() { }

    internal static OrderItem Create(Guid orderId, Guid productId, string productTitle, decimal unitPrice)
    {
        return new OrderItem
        {
            Id = Guid.NewGuid(),
            OrderId = orderId,
            ProductId = productId,
            ProductTitle = productTitle,
            UnitPrice = unitPrice,
            Quantity = 1
        };
    }
}
```

Create `backend/src/DigiMart.Order/Domain/Order.cs`:
```csharp
namespace DigiMart.Order.Domain;

public sealed class Order
{
    public Guid Id { get; private set; }
    public Guid BuyerId { get; private set; }
    public OrderStatus Status { get; private set; }
    public decimal TotalAmount { get; private set; }
    public DateTime CreatedAt { get; private set; }
    public DateTime UpdatedAt { get; private set; }
    public List<OrderItem> Items { get; private set; } = new();

    private Order() { }

    public static Order Create(Guid buyerId, List<(Guid ProductId, string Title, decimal Price)> items)
    {
        if (items.Count == 0)
            throw new ArgumentException("Order must have at least one item.", nameof(items));

        var now = DateTime.UtcNow;
        var order = new Order
        {
            Id = Guid.NewGuid(),
            BuyerId = buyerId,
            Status = OrderStatus.Pending,
            CreatedAt = now,
            UpdatedAt = now
        };

        foreach (var (productId, title, price) in items)
        {
            order.Items.Add(OrderItem.Create(order.Id, productId, title, price));
        }

        order.TotalAmount = order.Items.Sum(i => i.UnitPrice * i.Quantity);
        return order;
    }

    public void ConfirmPayment()
    {
        if (Status != OrderStatus.Pending)
            throw new InvalidOperationException($"Cannot confirm payment for order with status {Status}.");
        Status = OrderStatus.Confirmed;
        UpdatedAt = DateTime.UtcNow;
    }
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd backend
dotnet test tests/DigiMart.Order.Tests --filter "OrderTests" -v n
```

Expected: All 4 tests PASS.

- [ ] **Step 6: Create DbContext, contracts, endpoints, and Program.cs**

Create `backend/src/DigiMart.Order/Data/OrderDbContext.cs`:
```csharp
using Microsoft.EntityFrameworkCore;

namespace DigiMart.Order.Data;

public sealed class OrderDbContext : DbContext
{
    public DbSet<Domain.Order> Orders => Set<Domain.Order>();
    public DbSet<Domain.OrderItem> OrderItems => Set<Domain.OrderItem>();

    public OrderDbContext(DbContextOptions<OrderDbContext> options) : base(options) { }

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        modelBuilder.Entity<Domain.Order>(entity =>
        {
            entity.HasKey(o => o.Id);
            entity.Property(o => o.TotalAmount).HasColumnType("decimal(18,2)");
            entity.Property(o => o.Status).HasConversion<string>().HasMaxLength(20);
            entity.HasMany(o => o.Items).WithOne().HasForeignKey(i => i.OrderId);
        });

        modelBuilder.Entity<Domain.OrderItem>(entity =>
        {
            entity.HasKey(i => i.Id);
            entity.Property(i => i.ProductTitle).HasMaxLength(300).IsRequired();
            entity.Property(i => i.UnitPrice).HasColumnType("decimal(18,2)");
        });
    }
}
```

Create `backend/src/DigiMart.Order/Contracts/OrderContracts.cs`:
```csharp
using System.ComponentModel.DataAnnotations;

namespace DigiMart.Order.Contracts;

public sealed record CreateOrderRequest(
    [Required, MinLength(1)] List<OrderItemRequest> Items);

public sealed record OrderItemRequest(
    [Required] Guid ProductId,
    [Required] string ProductTitle,
    [Required] decimal UnitPrice);

public sealed record OrderResponse(
    Guid Id,
    string Status,
    decimal TotalAmount,
    List<OrderItemResponse> Items,
    DateTime CreatedAt);

public sealed record OrderItemResponse(
    Guid ProductId,
    string ProductTitle,
    decimal UnitPrice,
    int Quantity);
```

Create `backend/src/DigiMart.Order/Endpoints/OrderEndpoints.cs`:
```csharp
using System.Security.Claims;
using DigiMart.Order.Contracts;
using DigiMart.Order.Data;
using DigiMart.Shared.Events;
using DigiMart.Shared.Messaging;
using Microsoft.EntityFrameworkCore;

namespace DigiMart.Order.Endpoints;

public static class OrderEndpoints
{
    public static void MapOrderEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/orders").RequireAuthorization();

        group.MapPost("/", async (
            CreateOrderRequest request, ClaimsPrincipal principal,
            OrderDbContext db, IEventPublisher publisher, CancellationToken ct) =>
        {
            var buyerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var items = request.Items
                .Select(i => (i.ProductId, i.ProductTitle, i.UnitPrice))
                .ToList();

            var order = Domain.Order.Create(buyerId, items);
            db.Orders.Add(order);
            await db.SaveChangesAsync(ct);

            var @event = new OrderPlacedEvent(
                order.Id, buyerId,
                order.Items.Select(i => new OrderItemDto(i.ProductId, i.ProductTitle, i.UnitPrice)).ToList(),
                order.TotalAmount, DateTime.UtcNow);
            await publisher.PublishAsync("order.placed", order.Id.ToString(), @event, ct);

            return Results.Created($"/api/orders/{order.Id}", MapToResponse(order));
        });

        group.MapPost("/{id:guid}/pay", async (
            Guid id, ClaimsPrincipal principal,
            OrderDbContext db, IEventPublisher publisher, CancellationToken ct) =>
        {
            var buyerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var order = await db.Orders.Include(o => o.Items)
                .FirstOrDefaultAsync(o => o.Id == id && o.BuyerId == buyerId, ct);

            if (order is null) return Results.NotFound();

            try
            {
                order.ConfirmPayment();
            }
            catch (InvalidOperationException ex)
            {
                return Results.BadRequest(new { error = ex.Message });
            }

            await db.SaveChangesAsync(ct);

            var @event = new PaymentConfirmedEvent(
                order.Id, buyerId,
                order.Items.Select(i => new OrderItemDto(i.ProductId, i.ProductTitle, i.UnitPrice)).ToList(),
                DateTime.UtcNow);
            await publisher.PublishAsync("payment.confirmed", order.Id.ToString(), @event, ct);

            return Results.Ok(MapToResponse(order));
        });

        group.MapGet("/", async (
            ClaimsPrincipal principal, OrderDbContext db, CancellationToken ct) =>
        {
            var buyerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var orders = await db.Orders.Include(o => o.Items)
                .Where(o => o.BuyerId == buyerId)
                .OrderByDescending(o => o.CreatedAt)
                .ToListAsync(ct);

            return Results.Ok(orders.Select(MapToResponse));
        });

        group.MapGet("/{id:guid}", async (
            Guid id, ClaimsPrincipal principal, OrderDbContext db, CancellationToken ct) =>
        {
            var buyerId = Guid.Parse(principal.FindFirstValue(ClaimTypes.NameIdentifier)!);
            var order = await db.Orders.Include(o => o.Items)
                .FirstOrDefaultAsync(o => o.Id == id && o.BuyerId == buyerId, ct);

            if (order is null) return Results.NotFound();
            return Results.Ok(MapToResponse(order));
        });
    }

    private static OrderResponse MapToResponse(Domain.Order order) =>
        new(order.Id, order.Status.ToString(), order.TotalAmount,
            order.Items.Select(i => new OrderItemResponse(
                i.ProductId, i.ProductTitle, i.UnitPrice, i.Quantity)).ToList(),
            order.CreatedAt);
}
```

Replace `backend/src/DigiMart.Order/Program.cs`:
```csharp
using DigiMart.Order.Data;
using DigiMart.Order.Endpoints;
using DigiMart.Shared.Auth;
using DigiMart.Shared.Messaging;
using Microsoft.EntityFrameworkCore;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddDbContext<OrderDbContext>(options =>
    options.UseNpgsql(builder.Configuration.GetConnectionString("DefaultConnection")));

builder.Services.AddJwtAuthentication(builder.Configuration);
builder.Services.Configure<KafkaSettings>(builder.Configuration.GetSection("Kafka"));
builder.Services.AddSingleton<IEventPublisher, KafkaEventPublisher>();

builder.Services.AddCors(options =>
{
    options.AddDefaultPolicy(policy =>
        policy.AllowAnyOrigin().AllowAnyMethod().AllowAnyHeader());
});

var app = builder.Build();

using (var scope = app.Services.CreateScope())
{
    var db = scope.ServiceProvider.GetRequiredService<OrderDbContext>();
    await db.Database.MigrateAsync();
}

app.UseCors();
app.UseAuthentication();
app.UseAuthorization();
app.MapOrderEndpoints();

app.Run();
```

- [ ] **Step 7: Generate migration and verify build**

```bash
cd backend/src/DigiMart.Order
dotnet ef migrations add InitialCreate --output-dir Data/Migrations
cd ../..
dotnet build
```

Expected: Build succeeded.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(order): add order creation, payment simulation, and Kafka event publishing"
```

---

## Phase 5: Notification Service

### Task 8: Notification worker consuming Kafka events

**Files:**
- Create: `backend/src/DigiMart.Notification/Consumers/OrderPlacedConsumer.cs`
- Create: `backend/src/DigiMart.Notification/Consumers/PaymentConfirmedConsumer.cs`
- Create: `backend/src/DigiMart.Notification/Consumers/AccessGrantedConsumer.cs`
- Create: `backend/src/DigiMart.Notification/KafkaConsumerWorker.cs`
- Modify: `backend/src/DigiMart.Notification/Program.cs`

**Interfaces:**
- Consumes: `OrderPlacedEvent`, `PaymentConfirmedEvent`, `AccessGrantedEvent`, `KafkaSettings`
- Produces: Log output simulating email notifications for each event type

- [ ] **Step 1: Add Kafka package**

```bash
cd backend
dotnet add src/DigiMart.Notification package Confluent.Kafka --version 2.8.0
```

- [ ] **Step 2: Create Kafka consumer worker**

Create `backend/src/DigiMart.Notification/KafkaConsumerWorker.cs`:
```csharp
using System.Text.Json;
using Confluent.Kafka;
using DigiMart.Shared.Events;
using DigiMart.Shared.Messaging;
using Microsoft.Extensions.Options;

namespace DigiMart.Notification;

public sealed class KafkaConsumerWorker : BackgroundService
{
    private readonly ILogger<KafkaConsumerWorker> _logger;
    private readonly KafkaSettings _settings;
    private readonly string[] _topics = ["order.placed", "payment.confirmed", "access.granted"];

    public KafkaConsumerWorker(ILogger<KafkaConsumerWorker> logger, IOptions<KafkaSettings> settings)
    {
        _logger = logger;
        _settings = settings.Value;
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        await Task.Yield();

        var config = new ConsumerConfig
        {
            BootstrapServers = _settings.BootstrapServers,
            GroupId = "notification-service",
            AutoOffsetReset = AutoOffsetReset.Earliest,
            EnableAutoCommit = true
        };

        using var consumer = new ConsumerBuilder<string, string>(config).Build();
        consumer.Subscribe(_topics);

        _logger.LogInformation("Notification consumer started, subscribed to: {Topics}", string.Join(", ", _topics));

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                var result = consumer.Consume(stoppingToken);
                HandleMessage(result.Topic, result.Message.Value);
            }
            catch (ConsumeException ex)
            {
                _logger.LogError(ex, "Error consuming message");
            }
        }

        consumer.Close();
    }

    private void HandleMessage(string topic, string json)
    {
        switch (topic)
        {
            case "order.placed":
                var orderPlaced = JsonSerializer.Deserialize<OrderPlacedEvent>(json)!;
                _logger.LogInformation(
                    "[EMAIL] Order confirmation → Buyer {BuyerId}: Order {OrderId}, Total: {Total:C}, Items: {ItemCount}",
                    orderPlaced.BuyerId, orderPlaced.OrderId, orderPlaced.TotalAmount, orderPlaced.Items.Count);
                break;

            case "payment.confirmed":
                var paymentConfirmed = JsonSerializer.Deserialize<PaymentConfirmedEvent>(json)!;
                _logger.LogInformation(
                    "[EMAIL] Payment confirmed → Buyer {BuyerId}: Order {OrderId} paid successfully",
                    paymentConfirmed.BuyerId, paymentConfirmed.OrderId);
                break;

            case "access.granted":
                var accessGranted = JsonSerializer.Deserialize<AccessGrantedEvent>(json)!;
                _logger.LogInformation(
                    "[EMAIL] Download ready → Buyer {BuyerId}: Product {ProductId}, URL: {DownloadUrl}",
                    accessGranted.BuyerId, accessGranted.ProductId, accessGranted.DownloadUrl);
                break;

            default:
                _logger.LogWarning("Unknown topic: {Topic}", topic);
                break;
        }
    }
}
```

- [ ] **Step 3: Wire up Program.cs**

Replace `backend/src/DigiMart.Notification/Program.cs`:
```csharp
using DigiMart.Notification;
using DigiMart.Shared.Messaging;

var builder = Host.CreateApplicationBuilder(args);

builder.Services.Configure<KafkaSettings>(builder.Configuration.GetSection("Kafka"));
builder.Services.AddHostedService<KafkaConsumerWorker>();

var host = builder.Build();
host.Run();
```

- [ ] **Step 4: Verify build**

```bash
cd backend
dotnet build
```

Expected: Build succeeded.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(notification): add Kafka consumer worker for order, payment, and access events"
```

---

## Phase 6: Catalog Consumes PaymentConfirmed + Full Event Chain

### Task 9: Catalog Kafka consumer for PaymentConfirmed → ProductAccess + AccessGranted event

**Files:**
- Create: `backend/src/DigiMart.Catalog/Consumers/PaymentConfirmedConsumer.cs`
- Modify: `backend/src/DigiMart.Catalog/Program.cs`

**Interfaces:**
- Consumes: `PaymentConfirmedEvent`, `IEventPublisher`, `AccessGrantedEvent`, `CatalogDbContext`, `ProductAccess.Grant()`
- Produces: On `PaymentConfirmed`, creates `ProductAccess` records for each item and publishes `AccessGrantedEvent` per product

- [ ] **Step 1: Create the Kafka consumer**

Create `backend/src/DigiMart.Catalog/Consumers/PaymentConfirmedConsumer.cs`:
```csharp
using System.Text.Json;
using Confluent.Kafka;
using DigiMart.Catalog.Data;
using DigiMart.Catalog.Domain;
using DigiMart.Shared.Events;
using DigiMart.Shared.Messaging;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace DigiMart.Catalog.Consumers;

public sealed class PaymentConfirmedConsumer : BackgroundService
{
    private readonly ILogger<PaymentConfirmedConsumer> _logger;
    private readonly IServiceScopeFactory _scopeFactory;
    private readonly KafkaSettings _kafkaSettings;

    public PaymentConfirmedConsumer(
        ILogger<PaymentConfirmedConsumer> logger,
        IServiceScopeFactory scopeFactory,
        IOptions<KafkaSettings> kafkaSettings)
    {
        _logger = logger;
        _scopeFactory = scopeFactory;
        _kafkaSettings = kafkaSettings.Value;
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        await Task.Yield();

        var config = new ConsumerConfig
        {
            BootstrapServers = _kafkaSettings.BootstrapServers,
            GroupId = "catalog-service",
            AutoOffsetReset = AutoOffsetReset.Earliest,
            EnableAutoCommit = true
        };

        using var consumer = new ConsumerBuilder<string, string>(config).Build();
        consumer.Subscribe("payment.confirmed");

        _logger.LogInformation("Catalog consumer started, subscribed to: payment.confirmed");

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                var result = consumer.Consume(stoppingToken);
                await HandlePaymentConfirmed(result.Message.Value, stoppingToken);
            }
            catch (ConsumeException ex)
            {
                _logger.LogError(ex, "Error consuming payment.confirmed");
            }
        }

        consumer.Close();
    }

    private async Task HandlePaymentConfirmed(string json, CancellationToken ct)
    {
        var @event = JsonSerializer.Deserialize<PaymentConfirmedEvent>(json)!;
        _logger.LogInformation("Processing PaymentConfirmed for Order {OrderId}", @event.OrderId);

        using var scope = _scopeFactory.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<CatalogDbContext>();
        var publisher = scope.ServiceProvider.GetRequiredService<IEventPublisher>();

        foreach (var item in @event.Items)
        {
            var alreadyGranted = await db.ProductAccesses
                .AnyAsync(pa => pa.ProductId == item.ProductId && pa.BuyerId == @event.BuyerId, ct);

            if (alreadyGranted)
            {
                _logger.LogInformation("Access already granted for Product {ProductId} to Buyer {BuyerId}", item.ProductId, @event.BuyerId);
                continue;
            }

            var access = ProductAccess.Grant(item.ProductId, @event.BuyerId, @event.OrderId);
            db.ProductAccesses.Add(access);

            var product = await db.Products.FindAsync([item.ProductId], ct);
            var downloadUrl = product?.FileUrl ?? "unknown";

            var accessEvent = new AccessGrantedEvent(@event.BuyerId, item.ProductId, downloadUrl, DateTime.UtcNow);
            await publisher.PublishAsync("access.granted", item.ProductId.ToString(), accessEvent, ct);

            _logger.LogInformation("Access granted for Product {ProductId} to Buyer {BuyerId}", item.ProductId, @event.BuyerId);
        }

        await db.SaveChangesAsync(ct);
    }
}
```

- [ ] **Step 2: Register the consumer in Program.cs**

Add to `backend/src/DigiMart.Catalog/Program.cs`, before `var app = builder.Build();`:
```csharp
builder.Services.AddHostedService<DigiMart.Catalog.Consumers.PaymentConfirmedConsumer>();
```

- [ ] **Step 3: Verify build**

```bash
cd backend
dotnet build
```

Expected: Build succeeded.

- [ ] **Step 4: Test the full event chain with docker-compose**

```bash
docker compose up --build -d
```

Then test via curl:
1. Register a seller: `POST http://localhost:5001/api/auth/register` with `{"email":"seller@test.com","password":"123456","fullName":"Seller","role":"Seller"}`
2. Create a product with the seller's JWT
3. Register a buyer
4. Create an order with the buyer's JWT
5. Pay the order
6. Check `docker compose logs notification` for all 3 simulated emails
7. Check `GET /api/catalog/purchases` returns the product

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(catalog): consume PaymentConfirmed events to grant product access and publish AccessGranted"
```

---

## Phase 7: Frontend — Auth + Catalog Browsing

### Task 10: React project scaffold + auth pages

**Files:**
- Create: `frontend/` — Vite + React + TS project
- Create: `frontend/src/api/identityApi.ts`
- Create: `frontend/src/contexts/AuthContext.tsx`
- Create: `frontend/src/hooks/useAuth.ts`
- Create: `frontend/src/pages/Login.tsx`
- Create: `frontend/src/pages/Register.tsx`
- Create: `frontend/src/routes/PrivateRoute.tsx`
- Create: `frontend/src/routes/SellerRoute.tsx`
- Create: `frontend/src/components/Layout.tsx`
- Create: `frontend/src/components/Navbar.tsx`
- Create: `frontend/src/types/auth.ts`

**Interfaces:**
- Produces: Working login/register flow against Identity service at `localhost:5001`

- [ ] **Step 1: Create Vite React project**

```bash
npm create vite@latest frontend -- --template react-ts
cd frontend
npm install
npm install react-router-dom axios tailwindcss @tailwindcss/vite
```

- [ ] **Step 2: Configure Tailwind with Vite plugin**

Update `frontend/vite.config.ts`:
```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 3000,
  },
});
```

Replace `frontend/src/index.css`:
```css
@import "tailwindcss";
```

- [ ] **Step 3: Create types, API client, and auth context**

Create `frontend/src/types/auth.ts`:
```typescript
export interface User {
  id: string;
  email: string;
  fullName: string;
  role: 'Buyer' | 'Seller';
}

export interface AuthResponse {
  accessToken: string;
  refreshToken: string;
  expiresAt: string;
}

export interface RegisterRequest {
  email: string;
  password: string;
  fullName: string;
  role: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}
```

Create `frontend/src/api/identityApi.ts`:
```typescript
import axios from 'axios';
import type { AuthResponse, LoginRequest, RegisterRequest, User } from '../types/auth';

const api = axios.create({
  baseURL: import.meta.env.VITE_IDENTITY_URL || 'http://localhost:5001',
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('accessToken');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

export const identityApi = {
  register: (data: RegisterRequest) => api.post<AuthResponse>('/api/auth/register', data),
  login: (data: LoginRequest) => api.post<AuthResponse>('/api/auth/login', data),
  me: () => api.get<User>('/api/auth/me'),
};
```

Create `frontend/src/contexts/AuthContext.tsx`:
```tsx
import { createContext, useCallback, useEffect, useState, type ReactNode } from 'react';
import { identityApi } from '../api/identityApi';
import type { User, LoginRequest, RegisterRequest } from '../types/auth';

interface AuthContextType {
  user: User | null;
  loading: boolean;
  login: (data: LoginRequest) => Promise<void>;
  register: (data: RegisterRequest) => Promise<void>;
  logout: () => void;
}

export const AuthContext = createContext<AuthContextType>(null!);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchUser = useCallback(async () => {
    try {
      const { data } = await identityApi.me();
      setUser(data);
    } catch {
      localStorage.removeItem('accessToken');
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const token = localStorage.getItem('accessToken');
    if (token) fetchUser();
    else setLoading(false);
  }, [fetchUser]);

  const login = async (request: LoginRequest) => {
    const { data } = await identityApi.login(request);
    localStorage.setItem('accessToken', data.accessToken);
    await fetchUser();
  };

  const register = async (request: RegisterRequest) => {
    const { data } = await identityApi.register(request);
    localStorage.setItem('accessToken', data.accessToken);
    await fetchUser();
  };

  const logout = () => {
    localStorage.removeItem('accessToken');
    setUser(null);
  };

  return (
    <AuthContext.Provider value={{ user, loading, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  );
}
```

Create `frontend/src/hooks/useAuth.ts`:
```typescript
import { useContext } from 'react';
import { AuthContext } from '../contexts/AuthContext';

export function useAuth() {
  return useContext(AuthContext);
}
```

- [ ] **Step 4: Create route guards, layout, and auth pages**

Create `frontend/src/routes/PrivateRoute.tsx`:
```tsx
import { Navigate } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';

export function PrivateRoute({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <div className="flex justify-center p-8">Loading...</div>;
  return user ? <>{children}</> : <Navigate to="/login" />;
}
```

Create `frontend/src/routes/SellerRoute.tsx`:
```tsx
import { Navigate } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';

export function SellerRoute({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <div className="flex justify-center p-8">Loading...</div>;
  if (!user) return <Navigate to="/login" />;
  if (user.role !== 'Seller') return <Navigate to="/" />;
  return <>{children}</>;
}
```

Create `frontend/src/components/Navbar.tsx` and `frontend/src/components/Layout.tsx` with Tailwind-styled navigation bar showing login/register or user info + logout, and a main layout wrapper.

Create `frontend/src/pages/Login.tsx` and `frontend/src/pages/Register.tsx` — standard forms calling `useAuth().login()` and `useAuth().register()` respectively, with email/password fields, error display, and redirect to `/` on success. Register also includes fullName and role (Buyer/Seller) selector.

- [ ] **Step 5: Wire up App.tsx with React Router**

Replace `frontend/src/App.tsx`:
```tsx
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { AuthProvider } from './contexts/AuthContext';
import { Layout } from './components/Layout';
import { Login } from './pages/Login';
import { Register } from './pages/Register';

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/login" element={<Login />} />
            <Route path="/register" element={<Register />} />
            <Route path="/" element={<div className="text-center py-20 text-2xl">DigiMart — Coming soon</div>} />
          </Route>
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  );
}
```

- [ ] **Step 6: Verify frontend starts and auth works**

```bash
cd frontend
npm run dev
```

Test register and login against running backend (`docker compose up`).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(frontend): add React project scaffold with auth (login, register, JWT context)"
```

---

### Task 11: Catalog browsing pages (Home, product list, product detail)

**Files:**
- Create: `frontend/src/api/catalogApi.ts`
- Create: `frontend/src/types/catalog.ts`
- Create: `frontend/src/pages/Home.tsx`
- Create: `frontend/src/pages/Products.tsx`
- Create: `frontend/src/pages/ProductDetail.tsx`
- Create: `frontend/src/components/ProductCard.tsx`
- Modify: `frontend/src/App.tsx`

**Interfaces:**
- Consumes: Catalog service at `localhost:5002`, `useAuth()` for JWT
- Produces: Public product browsing: home with featured products, `/products` with search/filter/pagination, `/products/:slug` detail page

- [ ] **Step 1: Create catalog types and API client**

Create `frontend/src/types/catalog.ts`:
```typescript
export interface Product {
  id: string;
  sellerId: string;
  title: string;
  slug: string;
  description: string;
  price: number;
  thumbnailUrl: string;
  status: string;
  categoryName: string;
  createdAt: string;
}

export interface ProductDetail extends Product {
  fileUrl: string;
  categoryId: string;
  updatedAt: string;
}

export interface ProductListResponse {
  items: Product[];
  totalCount: number;
  page: number;
  pageSize: number;
}

export interface Category {
  id: string;
  name: string;
  slug: string;
  description: string;
}
```

Create `frontend/src/api/catalogApi.ts`:
```typescript
import axios from 'axios';
import type { ProductListResponse, ProductDetail, Category } from '../types/catalog';

const api = axios.create({
  baseURL: import.meta.env.VITE_CATALOG_URL || 'http://localhost:5002',
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('accessToken');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

export const catalogApi = {
  listProducts: (params?: { search?: string; categoryId?: string; page?: number }) =>
    api.get<ProductListResponse>('/api/catalog/products', { params }),
  getProduct: (slug: string) =>
    api.get<ProductDetail>(`/api/catalog/products/${slug}`),
  getCategories: () =>
    api.get<Category[]>('/api/catalog/categories'),
};
```

- [ ] **Step 2: Create ProductCard component, Home, Products, and ProductDetail pages**

Create `frontend/src/components/ProductCard.tsx` — card displaying thumbnail, title, category, price with link to `/products/:slug`.

Create `frontend/src/pages/Home.tsx` — hero section + grid of latest products from `catalogApi.listProducts()`.

Create `frontend/src/pages/Products.tsx` — search bar, category filter dropdown, paginated product grid.

Create `frontend/src/pages/ProductDetail.tsx` — full product info with "Add to Cart" button (wired in Phase 8).

- [ ] **Step 3: Add routes to App.tsx**

Add to the Routes in `App.tsx`:
```tsx
<Route path="/" element={<Home />} />
<Route path="/products" element={<Products />} />
<Route path="/products/:slug" element={<ProductDetail />} />
```

- [ ] **Step 4: Verify browsing works against running backend**

Start frontend, navigate to `/`, `/products`, click a product. Verify data loads from Catalog service.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(frontend): add catalog browsing — home, product list with filters, product detail"
```

---

## Phase 8: Frontend — Checkout + Seller Panel

### Task 12: Cart, checkout, order history, library, and seller CRUD pages

**Files:**
- Create: `frontend/src/api/orderApi.ts`
- Create: `frontend/src/types/order.ts`
- Create: `frontend/src/contexts/CartContext.tsx`
- Create: `frontend/src/hooks/useCart.ts`
- Create: `frontend/src/pages/Checkout.tsx`
- Create: `frontend/src/pages/Orders.tsx`
- Create: `frontend/src/pages/Library.tsx`
- Create: `frontend/src/pages/seller/SellerProducts.tsx`
- Create: `frontend/src/pages/seller/ProductForm.tsx`
- Modify: `frontend/src/App.tsx`

**Interfaces:**
- Consumes: `orderApi`, `catalogApi`, `useAuth()`, `PrivateRoute`, `SellerRoute`
- Produces: Complete buyer flow (cart → checkout → orders → library → download) and seller flow (list my products → create/edit → manage)

- [ ] **Step 1: Create order types, API client, and cart context**

Create `frontend/src/types/order.ts`:
```typescript
export interface OrderItemRequest {
  productId: string;
  productTitle: string;
  unitPrice: number;
}

export interface CreateOrderRequest {
  items: OrderItemRequest[];
}

export interface OrderResponse {
  id: string;
  status: string;
  totalAmount: number;
  items: { productId: string; productTitle: string; unitPrice: number; quantity: number }[];
  createdAt: string;
}
```

Create `frontend/src/api/orderApi.ts`:
```typescript
import axios from 'axios';
import type { CreateOrderRequest, OrderResponse } from '../types/order';

const api = axios.create({
  baseURL: import.meta.env.VITE_ORDER_URL || 'http://localhost:5003',
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('accessToken');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

export const orderApi = {
  create: (data: CreateOrderRequest) => api.post<OrderResponse>('/api/orders', data),
  pay: (orderId: string) => api.post<OrderResponse>(`/api/orders/${orderId}/pay`),
  list: () => api.get<OrderResponse[]>('/api/orders'),
  get: (id: string) => api.get<OrderResponse>(`/api/orders/${id}`),
};
```

Create `frontend/src/contexts/CartContext.tsx`:
```tsx
import { createContext, useState, type ReactNode } from 'react';
import type { Product } from '../types/catalog';

export interface CartItem {
  productId: string;
  title: string;
  price: number;
  thumbnailUrl: string;
}

interface CartContextType {
  items: CartItem[];
  addItem: (product: Product) => void;
  removeItem: (productId: string) => void;
  clear: () => void;
  total: number;
}

export const CartContext = createContext<CartContextType>(null!);

export function CartProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<CartItem[]>([]);

  const addItem = (product: Product) => {
    setItems((prev) => {
      if (prev.some((i) => i.productId === product.id)) return prev;
      return [...prev, { productId: product.id, title: product.title, price: product.price, thumbnailUrl: product.thumbnailUrl }];
    });
  };

  const removeItem = (productId: string) => {
    setItems((prev) => prev.filter((i) => i.productId !== productId));
  };

  const clear = () => setItems([]);
  const total = items.reduce((sum, i) => sum + i.price, 0);

  return (
    <CartContext.Provider value={{ items, addItem, removeItem, clear, total }}>
      {children}
    </CartContext.Provider>
  );
}
```

Create `frontend/src/hooks/useCart.ts`:
```typescript
import { useContext } from 'react';
import { CartContext } from '../contexts/CartContext';

export function useCart() {
  return useContext(CartContext);
}
```

- [ ] **Step 2: Create buyer pages — Checkout, Orders, Library**

Create `frontend/src/pages/Checkout.tsx` — displays cart items, total, and "Pay" button. On pay: calls `orderApi.create()` then `orderApi.pay()`, clears cart, redirects to `/orders`.

Create `frontend/src/pages/Orders.tsx` — lists orders from `orderApi.list()` with status, total, date.

Create `frontend/src/pages/Library.tsx` — lists purchased products from `catalogApi` purchases endpoint, with download button calling the download endpoint.

- [ ] **Step 3: Create seller pages — SellerProducts, ProductForm**

Create `frontend/src/pages/seller/SellerProducts.tsx` — lists seller's own products from `catalogApi` my-products endpoint, with "New Product" button and edit/delete actions.

Create `frontend/src/pages/seller/ProductForm.tsx` — form for creating/editing a product (title, description, price, category, file URL, thumbnail URL). On submit calls `catalogApi` create or update.

Add the catalog API methods for seller operations to `frontend/src/api/catalogApi.ts`:
```typescript
export const catalogApi = {
  // ... existing methods ...
  myProducts: () => api.get<Product[]>('/api/catalog/products/my'),
  createProduct: (data: any) => api.post('/api/catalog/products', data),
  updateProduct: (id: string, data: any) => api.put(`/api/catalog/products/${id}`, data),
  deleteProduct: (id: string) => api.delete(`/api/catalog/products/${id}`),
  getPurchases: () => api.get<{ productId: string; title: string; thumbnailUrl: string; grantedAt: string }[]>('/api/catalog/purchases'),
  getDownloadUrl: (productId: string) => api.get<{ fileUrl: string }>(`/api/catalog/purchases/${productId}/download`),
};
```

- [ ] **Step 4: Add all routes to App.tsx with guards**

Update `App.tsx` Routes:
```tsx
<Route path="/checkout" element={<PrivateRoute><Checkout /></PrivateRoute>} />
<Route path="/orders" element={<PrivateRoute><Orders /></PrivateRoute>} />
<Route path="/library" element={<PrivateRoute><Library /></PrivateRoute>} />
<Route path="/seller/products" element={<SellerRoute><SellerProducts /></SellerRoute>} />
<Route path="/seller/products/new" element={<SellerRoute><ProductForm /></SellerRoute>} />
<Route path="/seller/products/:id/edit" element={<SellerRoute><ProductForm /></SellerRoute>} />
```

Wrap `<AuthProvider>` children with `<CartProvider>`.

- [ ] **Step 5: Test full buyer and seller flows**

1. Register as Seller → create a product → verify it appears on home
2. Register as Buyer → browse → add to cart → checkout → pay → see in orders → see in library → download

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(frontend): add cart, checkout, orders, library, and seller product management"
```

---

## Phase 9: CI/CD + E2E Tests

### Task 13: GitHub Actions CI workflows

**Files:**
- Create: `.github/workflows/backend-ci.yml`
- Create: `.github/workflows/frontend-ci.yml`

**Interfaces:**
- Produces: CI pipelines triggered on push/PR for respective directories

- [ ] **Step 1: Create backend CI workflow**

Create `.github/workflows/backend-ci.yml`:
```yaml
name: Backend CI

on:
  push:
    paths: ['backend/**']
  pull_request:
    paths: ['backend/**']

jobs:
  build-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup .NET
        uses: actions/setup-dotnet@v4
        with:
          dotnet-version: '9.0.x'

      - name: Restore
        run: dotnet restore
        working-directory: backend

      - name: Build
        run: dotnet build --no-restore
        working-directory: backend

      - name: Unit Tests
        run: dotnet test --no-build -v normal --filter "FullyQualifiedName!~Integration"
        working-directory: backend
```

- [ ] **Step 2: Create frontend CI workflow**

Create `.github/workflows/frontend-ci.yml`:
```yaml
name: Frontend CI

on:
  push:
    paths: ['frontend/**']
  pull_request:
    paths: ['frontend/**']

jobs:
  build-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
          cache-dependency-path: frontend/package-lock.json

      - name: Install
        run: npm ci
        working-directory: frontend

      - name: Lint
        run: npm run lint
        working-directory: frontend

      - name: Type Check
        run: npx tsc --noEmit
        working-directory: frontend

      - name: Build
        run: npm run build
        working-directory: frontend
```

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "ci: add GitHub Actions workflows for backend and frontend"
```

---

### Task 14: Playwright E2E tests

**Files:**
- Create: `e2e/package.json`
- Create: `e2e/playwright.config.ts`
- Create: `e2e/tests/purchase-flow.spec.ts`
- Create: `.github/workflows/e2e-ci.yml`

**Interfaces:**
- Consumes: All 4 services running via docker-compose, frontend at `localhost:3000`
- Produces: E2E test covering register → login → browse → purchase → library

- [ ] **Step 1: Initialize Playwright project**

```bash
mkdir e2e && cd e2e
npm init -y
npm install -D @playwright/test
npx playwright install chromium
```

- [ ] **Step 2: Create Playwright config**

Create `e2e/playwright.config.ts`:
```typescript
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  retries: 1,
  use: {
    baseURL: 'http://localhost:3000',
    headless: true,
  },
});
```

- [ ] **Step 3: Write purchase flow E2E test**

Create `e2e/tests/purchase-flow.spec.ts`:
```typescript
import { test, expect } from '@playwright/test';

test.describe('Purchase Flow', () => {
  const sellerEmail = `seller-${Date.now()}@test.com`;
  const buyerEmail = `buyer-${Date.now()}@test.com`;
  const password = 'password123';

  test('seller registers and creates a product', async ({ page }) => {
    await page.goto('/register');
    await page.fill('input[name="email"]', sellerEmail);
    await page.fill('input[name="password"]', password);
    await page.fill('input[name="fullName"]', 'Test Seller');
    await page.selectOption('select[name="role"]', 'Seller');
    await page.click('button[type="submit"]');
    await expect(page).toHaveURL('/');

    await page.goto('/seller/products/new');
    await page.fill('input[name="title"]', 'E2E Test Course');
    await page.fill('textarea[name="description"]', 'A course for E2E testing');
    await page.fill('input[name="price"]', '19.90');
    await page.fill('input[name="fileUrl"]', 'https://example.com/course.zip');
    await page.click('button[type="submit"]');
    await expect(page.locator('text=E2E Test Course')).toBeVisible();
  });

  test('buyer registers, purchases product, and sees it in library', async ({ page }) => {
    await page.goto('/register');
    await page.fill('input[name="email"]', buyerEmail);
    await page.fill('input[name="password"]', password);
    await page.fill('input[name="fullName"]', 'Test Buyer');
    await page.selectOption('select[name="role"]', 'Buyer');
    await page.click('button[type="submit"]');
    await expect(page).toHaveURL('/');

    await page.goto('/products');
    await page.click('text=E2E Test Course');
    await page.click('text=Add to Cart');
    await page.goto('/checkout');
    await page.click('text=Pay');
    await expect(page.locator('text=Confirmed')).toBeVisible();

    await page.goto('/library');
    await expect(page.locator('text=E2E Test Course')).toBeVisible();
  });
});
```

- [ ] **Step 4: Create E2E CI workflow**

Create `.github/workflows/e2e-ci.yml`:
```yaml
name: E2E CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Start services
        run: docker compose up -d --build --wait

      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Playwright
        run: |
          cd e2e
          npm ci
          npx playwright install chromium --with-deps

      - name: Start frontend
        run: |
          cd frontend
          npm ci
          npm run build
          npx serve -s dist -l 3000 &
          sleep 5

      - name: Run E2E tests
        run: cd e2e && npx playwright test

      - name: Teardown
        if: always()
        run: docker compose down -v
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test(e2e): add Playwright purchase flow test and E2E CI workflow"
```

---

## Phase 10: Deploy + README

### Task 15: Render deploy configuration and professional README

**Files:**
- Create: `render.yaml`
- Modify: `README.md`

**Interfaces:**
- Produces: Render blueprint for deploying all services + frontend, and a professional README with architecture diagram, screenshots placeholder, live demo link, and setup instructions

- [ ] **Step 1: Create Render blueprint**

Create `render.yaml`:
```yaml
services:
  - type: web
    name: digimart-identity
    runtime: docker
    dockerfilePath: backend/src/DigiMart.Identity/Dockerfile
    dockerContext: backend
    envVars:
      - key: ConnectionStrings__DefaultConnection
        fromDatabase:
          name: digimart-db
          property: connectionString
      - key: Jwt__Secret
        generateValue: true
      - key: Jwt__Issuer
        value: DigiMart
      - key: Jwt__Audience
        value: DigiMart
      - key: Kafka__BootstrapServers
        value: TO_BE_SET_UPSTASH

  - type: web
    name: digimart-catalog
    runtime: docker
    dockerfilePath: backend/src/DigiMart.Catalog/Dockerfile
    dockerContext: backend
    envVars:
      - key: ConnectionStrings__DefaultConnection
        fromDatabase:
          name: digimart-db
          property: connectionString
      - key: Jwt__Secret
        sync: false
      - key: Kafka__BootstrapServers
        value: TO_BE_SET_UPSTASH
      - key: Redis__ConnectionString
        fromService:
          name: digimart-redis
          type: redis
          property: connectionString

  - type: web
    name: digimart-order
    runtime: docker
    dockerfilePath: backend/src/DigiMart.Order/Dockerfile
    dockerContext: backend
    envVars:
      - key: ConnectionStrings__DefaultConnection
        fromDatabase:
          name: digimart-db
          property: connectionString
      - key: Jwt__Secret
        sync: false
      - key: Kafka__BootstrapServers
        value: TO_BE_SET_UPSTASH

  - type: worker
    name: digimart-notification
    runtime: docker
    dockerfilePath: backend/src/DigiMart.Notification/Dockerfile
    dockerContext: backend
    envVars:
      - key: Kafka__BootstrapServers
        value: TO_BE_SET_UPSTASH

  - type: web
    name: digimart-frontend
    runtime: static
    buildCommand: cd frontend && npm ci && npm run build
    staticPublishPath: frontend/dist
    headers:
      - path: /*
        name: Cache-Control
        value: public, max-age=0, must-revalidate

databases:
  - name: digimart-db
    plan: free
    databaseName: digimart
    user: digimart

  - name: digimart-redis
    type: redis
    plan: free
```

- [ ] **Step 2: Write professional README**

Replace `README.md` with a professional README including:
- Project name, badges (CI workflows), live demo link
- "What it is" — 3-4 sentence description of DigiMart
- Architecture diagram (text-based showing services + Kafka + Redis + PostgreSQL)
- Features list (auth, catalog, checkout, events, notifications)
- Tech stack table
- Screenshots section (placeholder for now)
- Getting started (prerequisites, clone, docker compose up, access URLs)
- API documentation section listing endpoints per service
- Project structure tree
- License

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "docs: add Render deploy blueprint and professional README"
```

- [ ] **Step 4: Create GitHub repository and push**

```bash
gh repo create MatheusCavalari/digimart --public --source=. --push
```
