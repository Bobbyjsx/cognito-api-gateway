# Cognito API Gateway

High-performance, ultra-low latency API Gateway for the Cognito AI platform.

## Architecture & Responsibilities

- **Unified Surface**: Routes edge traffic seamlessly between **Cognito Backend** (AI models, chat, sessions, tasks, attachments, billing) and **Identity Service** (OAuth 2.0 PKCE, session management, JWKS discovery).
- **Zero-Allocation SSE Streaming**: Supports server-sent events (`/agent/chat/stream`) with immediate chunk flushing and no intermediate proxy buffering.
- **Edge Security & CORS**: Manages CORS headers, preflight requests, security headers (HSTS, nosniff, frame denial), and distributed token rotation passthrough.
- **Observability**: New Relic Go Agent APM, W3C distributed tracing (`traceparent`/`tracestate`), structured JSON logging with in-context APM trace correlation.
- **Reliability**: Configurable token-bucket rate limiting, panic recovery with error capture, and cached upstream readiness probes (`/health`, `/ready`, `/ready/backend`, `/ready/identity`).

## Upstream Routing Table

| Path Prefix | Destination Upstream | Description |
| :--- | :--- | :--- |
| `/health`, `/healthz`, `/ready` | Gateway Native | Health and readiness probes |
| `/api/v1/oauth/*` | `identity-service` | OAuth 2.0 authorization, token exchange, PKCE |
| `/api/v1/auth/*` | `identity-service` | Direct authentication sessions |
| `/.well-known/*` | `identity-service` | OpenID configuration and JWKS public keys |
| `/agent/*` | `cognito-backend` | AI Chat, SSE stream, sessions, and attachments |
| `/auth/*` | `cognito-backend` | Profile settings and custom instructions |
| `/config/*` | `cognito-backend` | Dynamic client configuration |
| `/stt/*` | `cognito-backend` | Speech-to-text audio transcription |
| `/tasks/*` | `cognito-backend` | Cloud Tasks background generation workers |
| `/billing/*`, `/webhooks/*` | `cognito-backend` | Paystack checkout, subscriptions, and webhooks |
| `/prompts/*` | `cognito-backend` | Reusable user prompt library |
| `/docs`, `/redoc`, `/openapi.json` | `cognito-backend` | OpenAPI specs and interactive documentation |

## Development

```bash
make test       # Run unit and integration tests with race detector
make lint       # Run go vet
make build      # Compile optimized statically linked binary
make run        # Run locally on port 8000
```
