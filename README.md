<div align="center">

# Nexora

### Scalable Social Platform Backend Built with Go, Microservices, Redis, WebSockets & Kubernetes

**Nexora** is a production-oriented social platform backend designed to demonstrate modern backend engineering practices, scalable architecture, real-time communication, service isolation, caching, security, and cloud-native deployment.

[![Go](https://img.shields.io/badge/Go-Backend-00ADD8?logo=go\&logoColor=white)](https://go.dev/)
[![Gin](https://img.shields.io/badge/Gin-Web_Framework-008ECF)](https://gin-gonic.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Database-4169E1?logo=postgresql\&logoColor=white)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-Cache_%26_Rate_Limiting-DC382D?logo=redis\&logoColor=white)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Containerized-2496ED?logo=docker\&logoColor=white)](https://www.docker.com/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-Orchestration-326CE5?logo=kubernetes\&logoColor=white)](https://kubernetes.io/)

</div>

---

## About Nexora

Nexora is a backend-first social networking platform built with **Go and Gin** using a **microservices architecture**.

The project focuses on solving real backend engineering problems such as:

* Authentication and authorization
* User management
* Social posts
* Real-time chat
* Notifications
* Redis caching
* Distributed rate limiting
* PostgreSQL data persistence
* WebSocket communication
* Docker containerization
* Kubernetes orchestration
* Service health monitoring
* Environment-based configuration
* Secure API design
* Horizontal scalability

The goal of Nexora is not simply to build CRUD APIs.

It is designed as a practical demonstration of how modern distributed backend systems can be structured, secured, deployed, and scaled.

---

## Why I Built Nexora

I created Nexora to strengthen my backend engineering knowledge beyond traditional monolithic applications.

The project allows me to work directly with concepts commonly used in production systems:

**Microservices Architecture → Service Communication → Redis → Rate Limiting → WebSockets → PostgreSQL → Containers → Kubernetes**

Through Nexora, I am focusing on writing maintainable Go services while learning how distributed systems behave under real-world requirements.

---

## Architecture

```text
                          ┌─────────────────────┐
                          │      Client App     │
                          │ Web / Mobile Client │
                          └──────────┬──────────┘
                                     │
                                     ▼
                          ┌─────────────────────┐
                          │    API Gateway /    │
                          │      Ingress        │
                          └──────────┬──────────┘
                                     │
                ┌────────────────────┼─────────────────────┐
                │                    │                     │
                ▼                    ▼                     ▼
        ┌──────────────┐     ┌──────────────┐      ┌──────────────┐
        │ Auth Service │     │ Post Service │      │ Chat Service │
        └──────┬───────┘     └──────┬───────┘      └──────┬───────┘
               │                    │                     │
               │                    │                 WebSockets
               │                    │                     │
               └────────────┬───────┴────────────┬────────┘
                            │                    │
                            ▼                    ▼
                    ┌──────────────┐      ┌──────────────┐
                    │ PostgreSQL   │      │    Redis     │
                    │ Persistence  │      │ Cache / Rate │
                    │              │      │   Limiting   │
                    └──────────────┘      └──────────────┘

                              │
                              ▼
                    ┌─────────────────────┐
                    │ Notification Service│
                    └─────────────────────┘
```

---

# Microservices

Nexora is divided into focused services instead of placing the entire system inside one application.

| Service                  | Responsibility                                                  |
| ------------------------ | --------------------------------------------------------------- |
| **Auth Service**         | Authentication, registration, users, sessions and authorization |
| **Post Service**         | Creating, retrieving, editing and deleting posts                |
| **Chat Service**         | Real-time messaging and WebSocket communication                 |
| **Notification Service** | User notifications and event-driven updates                     |
| **Redis**                | Caching, rate limiting and temporary distributed data           |
| **PostgreSQL**           | Persistent relational application data                          |

Each service is designed to remain independently maintainable and deployable.

---

# Authentication & User Management

The Auth Service is responsible for both authentication and user-related identity functionality.

Core responsibilities include:

```text
Register User
      ↓
Validate Request
      ↓
Hash Password
      ↓
Store User
      ↓
Generate Authentication Token
      ↓
Return Safe User Response
```

Planned and implemented security practices include:

* Password hashing
* JWT authentication
* Secure environment variables
* Role-based access control
* Request validation
* Rate limiting
* Authentication middleware
* Protected endpoints
* Safe error responses

---

# Redis Integration

Redis is an important infrastructure component in Nexora.

It is used for:

### Caching

Frequently requested information can be stored temporarily to reduce unnecessary database operations.

```text
Request
   │
   ▼
Check Redis
   │
   ├── Cache Hit ─────► Return Cached Data
   │
   └── Cache Miss
            │
            ▼
       PostgreSQL
            │
            ▼
       Store in Redis
            │
            ▼
       Return Response
```

### Rate Limiting

Redis is also used to enforce distributed rate limits across services.

Example:

```text
Client Request
      │
      ▼
Redis Counter
      │
      ├── Allowed ───► Continue Request
      │
      └── Limit Reached
                  │
                  ▼
             HTTP 429
```

This protects APIs against:

* Request flooding
* Brute-force attempts
* API abuse
* Accidental high traffic
* Excessive repeated requests

---

# Real-Time Communication

Nexora includes real-time functionality through **WebSockets**.

The Chat Service is designed to support features such as:

* Real-time messaging
* User connection management
* Online presence
* Message events
* Typing indicators
* Real-time notifications

Example flow:

```text
User A
  │
  │ WebSocket
  ▼
Chat Service
  │
  ├──── Store Message ────► PostgreSQL
  │
  └──── Push Event ───────► User B
```

---

# PostgreSQL

PostgreSQL is used as the primary persistent database.

It was selected because the platform contains strongly related data such as:

* Users
* Posts
* Messages
* Conversations
* Notifications
* Roles
* Authentication records

The database layer is designed with:

* Clear models
* Relationships
* Constraints
* Indexing
* Migrations
* Transactions where required
* Environment-based configuration

---

# Docker

Every microservice can run inside its own Docker container.

The local development environment can include:

```text
┌──────────────────────────┐
│       Docker Compose     │
├──────────────────────────┤
│ Auth Service             │
│ Post Service             │
│ Chat Service             │
│ Notification Service     │
│ PostgreSQL               │
│ Redis                    │
└──────────────────────────┘
```

Benefits include:

* Reproducible environments
* Simple local development
* Service isolation
* Easier deployment
* Consistent dependencies

---

# Kubernetes

Nexora is also being prepared for Kubernetes deployment.

Kubernetes configuration is organized around components such as:

```text
kubernetes/
│
├── services/
├── postgres/
├── redis/
└── ingress/
```

Kubernetes allows Nexora to explore production concepts such as:

* Pods
* Deployments
* Services
* ConfigMaps
* Secrets
* Ingress
* Service discovery
* Health checks
* Replica scaling
* Container recovery
* Rolling deployments

---

# Project Structure

```text
social-media-backend/
│
├── cmd/
│
├── services/
│   │
│   ├── auth-service/
│   ├── post-service/
│   ├── chat-service/
│   └── notification-service/
│
├── internal/
│
├── pkg/
│
├── docker/
│   │
│   ├── auth-service.Dockerfile
│   ├── post-service.Dockerfile
│   ├── chat-service.Dockerfile
│   ├── notification-service.Dockerfile
│   ├── docker-compose.yml
│   └── redis.conf
│
├── deployments/
│   └── kubernetes/
│       ├── database/
│       ├── ingress/
│       └── services/
│
├── kubernetes/
│   ├── ingress/
│   ├── postgres/
│   ├── redis/
│   └── services/
│
├── go.mod
├── go.sum
└── README.md
```

The structure may continue evolving as the architecture matures.

---

# Technology Stack

<div align="center">

| Area                    | Technology        |
| ----------------------- | ----------------- |
| Language                | **Go**            |
| HTTP Framework          | **Gin**           |
| Architecture            | **Microservices** |
| Primary Database        | **PostgreSQL**    |
| Cache                   | **Redis**         |
| Rate Limiting           | **Redis**         |
| Real-Time Communication | **WebSockets**    |
| Containers              | **Docker**        |
| Container Orchestration | **Kubernetes**    |
| API Style               | **REST**          |
| Authentication          | **JWT**           |
| Version Control         | **Git / GitHub**  |

</div>

---

# Example API Flow

```text
POST /auth/register
        │
        ▼
Request Validation
        │
        ▼
Rate Limiter
        │
        ▼
Auth Service
        │
        ▼
Password Hashing
        │
        ▼
PostgreSQL
        │
        ▼
Generate JWT
        │
        ▼
HTTP Response
```

---

# API Principles

The backend follows several API engineering principles.

### Consistent responses

Successful requests should return predictable response structures.

```json
{
  "success": true,
  "message": "User created successfully",
  "data": {}
}
```

Errors should also follow consistent formats.

```json
{
  "success": false,
  "message": "Invalid request"
}
```

---

# Security

Security is treated as part of the architecture rather than an afterthought.

Current and planned protections include:

* JWT authentication
* Password hashing
* Redis rate limiting
* Input validation
* Role authorization
* Environment variables
* Protected database credentials
* Kubernetes Secrets
* CORS configuration
* Secure error handling
* Request size limits
* HTTP security middleware

---

# Scalability

Nexora is designed so individual services can scale independently.

For example:

```text
                 Post Service
                      │
          ┌───────────┼───────────┐
          │           │           │
          ▼           ▼           ▼
      Instance 1  Instance 2  Instance 3
          │           │           │
          └───────────┼───────────┘
                      │
                      ▼
                     Redis
                      │
                      ▼
                  PostgreSQL
```

If traffic increases mainly on messaging, the Chat Service can scale without necessarily scaling unrelated services.

---

# Engineering Goals

This project focuses on developing practical experience in:

* Backend architecture
* Go application design
* Distributed systems
* Microservice boundaries
* Database modeling
* API security
* Caching
* Rate limiting
* Real-time communication
* Docker
* Kubernetes
* Infrastructure configuration
* Debugging production-style systems

---

# Getting Started

## 1. Clone the Repository

```bash
git clone https://github.com/YOUR_USERNAME/nexora.git

cd nexora
```

---

## 2. Configure Environment Variables

Create the required `.env` files for each service.

Example:

```env
PORT=8080

DB_HOST=postgres
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=nexora
DB_SSLMODE=disable

REDIS_HOST=redis
REDIS_PORT=6379

JWT_SECRET=change-this-secret
```

Never commit production secrets to GitHub.

---

## 3. Start with Docker

```bash
docker compose up --build
```

Check running containers:

```bash
docker ps
```

---

## 4. Check Redis

```bash
docker exec -it redis redis-cli
```

Then:

```bash
PING
```

Expected response:

```text
PONG
```

---

## 5. Run a Go Service Directly

Example:

```bash
go run ./cmd/server
```

---

# Development

Format Go code:

```bash
go fmt ./...
```

Run tests:

```bash
go test ./...
```

Run static checks:

```bash
go vet ./...
```

Download dependencies:

```bash
go mod tidy
```

---

# Current Development Focus

Nexora is actively being developed.

Current engineering focus includes:

* Strengthening microservice boundaries
* Connecting Redis across required services
* Applying distributed rate limiting
* Improving authentication
* Developing user functionality inside Auth Service
* Building WebSocket communication
* Improving database models
* Preparing Kubernetes deployment
* Adding health checks
* Improving observability
* Expanding automated testing

---

# Roadmap

```text
Authentication          █████████░
User Management         █████████░
Post Service            ███████░░░
Redis Integration       ████████░░
Rate Limiting           ████████░░
Chat Service            ██████░░░░
WebSockets              █████░░░░░
Notifications           ████░░░░░░
Docker                  ████████░░
Kubernetes              █████░░░░░
Testing                 █████░░░░░
Observability           ███░░░░░░░
```

The indicators above represent development direction rather than formal release percentages.

---

# Future Improvements

Planned improvements include:

* API Gateway
* Refresh token management
* Event-driven communication
* Message queues
* Structured logging
* Distributed tracing
* Prometheus metrics
* Grafana monitoring
* CI/CD
* Kubernetes autoscaling
* Improved integration testing
* Better failure handling
* Graceful shutdown
* Database replicas
* Centralized configuration
* Advanced caching policies

---

# What This Project Demonstrates

For recruiters and engineers reviewing this repository, Nexora demonstrates my practical experience and continued development in:

**Go backend development**

Building maintainable backend services with Go and Gin.

**System architecture**

Breaking a larger application into independently manageable services.

**Database engineering**

Working with PostgreSQL, relational models, migrations, and persistent application data.

**Performance engineering**

Using Redis for caching and reducing unnecessary work.

**API protection**

Implementing distributed rate limiting and authentication middleware.

**Real-time systems**

Building WebSocket-based communication for chat and notifications.

**DevOps fundamentals**

Containerizing applications with Docker and deploying services using Kubernetes.

**Production mindset**

Considering security, scalability, observability, configuration, failures, and infrastructure from the beginning of development.

---

# Screenshots

<div align="center">

### Architecture

<!-- Replace with your real image -->

<img src="./docs/images/architecture.png" width="850" alt="Nexora Architecture">

### Kubernetes Infrastructure

<img src="./docs/images/kubernetes.png" width="850" alt="Nexora Kubernetes Infrastructure">

### API Testing

<img src="./docs/images/api-testing.png" width="850" alt="Nexora API Testing">

</div>

---

# Repository Philosophy

> Build the backend as a system, not just a collection of endpoints.

Nexora is intentionally being developed incrementally.

Every new component is expected to improve one or more of these areas:

**Reliability · Security · Maintainability · Performance · Scalability**

---

<div align="center">

## Nexora

### Connect. Communicate. Scale.

Built with **Go** and a focus on modern backend engineering.

⭐ If you find the project interesting, consider starring the repository.

</div>
