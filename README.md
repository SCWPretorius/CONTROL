# CONTROL

**CONTROL** – A modular, event-driven personal assistant built with TypeScript and Node.js. Think of it as your own Jarvis-like system that intelligently handles tasks, integrates with multiple platforms, and learns from context.

## Overview

CONTROL is a production-ready autonomous assistant framework that processes events from various sources (Telegram, Discord, Google Calendar), uses LLM-based decision making to determine appropriate actions, and executes skills with built-in safety mechanisms including RBAC, rate limiting, and approval workflows.

## Features

- **🤖 LLM-Powered Decision Making**: Uses Ollama with configurable primary and fallback models (Qwen, Mistral, Phi) to intelligently decide which skills to execute
- **🔌 Multi-Platform Integrations**: Built-in support for Telegram, Discord, and Google Calendar
- **🎯 Extensible Skills System**: Modular skill architecture with hot-reloading capabilities
- **🧠 Context-Aware Memory**: Vector-based context retrieval for relevant historical information
- **🔒 Enterprise-Grade Security**: Role-based access control (RBAC), rate limiting, and approval workflows
- **📊 Comprehensive Tracing**: Full decision trace logging for debugging and monitoring
- **♻️ Resilient Execution**: Idempotent execution with retry logic and timeout handling
- **🔄 Self-Healing**: Automatic recovery of pending executions on restart
- **💾 Data Persistence**: Scheduled backups and memory cleanup
- **🏥 Health Monitoring**: Built-in heartbeat and health check endpoints

## Architecture

CONTROL follows an event-driven architecture:

```
Integration Sources → Event Bus → LLM Decision Engine → Skill Executor → Target Integration
                          ↓
                  Context Retrieval (Vector Store)
```

**Key Components:**

1. **Event Bus**: Central message broker for all system events
2. **Integration Loader**: Auto-discovers and loads integration modules
3. **Skill Registry**: Manages available skills with validation and permissions
4. **LLM Decider**: Analyzes events and context to select appropriate skills
5. **Skill Executor**: Safely executes skills with concurrency control and approval gates
6. **Context Store**: Maintains conversation history and relevant context
7. **Vector Store**: Enables semantic search for context retrieval
8. **Decision Tracer**: Logs every decision step for auditability

## Prerequisites

- **Node.js** >= 20.0.0
- **Ollama** running locally (default: `http://localhost:11434`)
- API credentials for integrations you want to use:
  - Telegram Bot Token (optional)
  - Discord Bot Token and Public Key (optional)
  - Google OAuth2 credentials (optional)

## Installation

```bash
# Clone the repository
git clone https://github.com/SCWPretorius/CONTROL.git
cd CONTROL

# Install dependencies
npm install

# Build the project
npm run build
```

## Configuration

Create a `.env` file in the project root:

```env
# Server Configuration
PORT=3000
HOST=0.0.0.0
WEBHOOK_BASE_URL=http://localhost:3000

# Ollama LLM Configuration
OLLAMA_URL=http://localhost:11434
LLM_PRIMARY_MODEL=qwen3:4b
LLM_FALLBACK_1=mistral:7b
LLM_FALLBACK_2=phi:2.7b

# Security
ENCRYPTION_KEY=your-32-character-encryption-key

# Telegram Integration (optional)
TELEGRAM_BOT_TOKEN=your-telegram-bot-token
TELEGRAM_WEBHOOK_SECRET=your-webhook-secret

# Discord Integration (optional)
DISCORD_BOT_TOKEN=your-discord-bot-token
DISCORD_PUBLIC_KEY=your-discord-public-key

# Google Calendar Integration (optional)
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
GOOGLE_REDIRECT_URI=http://localhost:3000/auth/google/callback

# System Tuning
HEARTBEAT_INTERVAL_MS=30000
```

### Directory Structure

CONTROL automatically creates the following data directories:

- `data/memory/` – Context files and conversation history
- `data/executions/` – Execution records and state
- `data/backups/` – Scheduled backups
- `logs/` – System logs and decision traces

## Usage

### Development Mode

```bash
npm run dev
```

This starts the application with hot-reload using `tsx watch`.

### Production Mode

```bash
npm start
```

### Running Tests

```bash
# Run all tests once
npm test

# Watch mode for development
npm run test:watch

# Type checking
npm run typecheck
```

## Project Structure

```
src/
├── index.ts                    # Main entry point and event handler
├── backup/                     # Backup management and scheduling
├── cleanup/                    # Memory and data cleanup routines
├── concurrency/                # Rate limiting and concurrency control
├── config/                     # Configuration and secrets management
├── events/                     # Event bus implementation
├── executor/                   # Skill execution engine with retry logic
├── heartbeat/                  # Health monitoring
├── integrations/               # Platform integrations (hot-loadable)
│   ├── discord.ts
│   ├── telegram.ts
│   ├── googleCalendar.ts
│   └── integrationLoader.ts
├── llm/                        # LLM decision engine and prompt building
├── logging/                    # Structured logging with Pino
├── memory/                     # Context storage and retrieval
│   ├── contextStore.ts         # Context file management
│   ├── contextRetriever.ts     # Semantic context retrieval
│   └── vectorStore.ts          # Vector embeddings for search
├── permissions/                # RBAC implementation
├── server/                     # Express HTTP server and approval routes
├── skills/                     # Skill modules (hot-loadable)
│   ├── sendMessage.ts
│   ├── queryCalendar.ts
│   ├── deleteCalendarEvent.ts
│   ├── updateContext.ts
│   └── skillRegistry.ts
├── tracing/                    # Decision and execution tracing
└── types/                      # TypeScript type definitions
```

## Core Concepts

### Events

All inputs to CONTROL are normalized into `NormalizedEvent` objects:

```typescript
interface NormalizedEvent {
  id: string;              // Unique event identifier
  traceId: string;         // Trace ID for distributed tracing
  source: string;          // Origin (e.g., 'telegram', 'discord')
  type: string;            // Event type (e.g., 'message', 'calendar-event')
  payload: Record<string, unknown>;  // Event-specific data
  timestamp: string;       // ISO timestamp
  userId?: string;         // User identifier
  role?: Role;             // User role for RBAC
  tenantId?: string;       // Multi-tenancy support
}
```

### Skills

Skills are executable actions with defined capabilities. Each skill includes:

- **Definition**: Name, description, parameters schema (Zod), tags
- **Execute Function**: Async handler that performs the action
- **Security**: `minRole`, `requiresApproval`, rate limits
- **Performance**: `maxConcurrent`, `timeoutMs`, `priority`

**Built-in Skills:**
- `sendMessage` – Send messages via integrations
- `queryCalendar` – Search calendar events
- `deleteCalendarEvent` – Remove calendar events
- `updateContext` – Store information in memory

### Integrations

Integrations connect CONTROL to external platforms. Each integration provides:

- **Poll**: Periodically fetch events (polling mode)
- **OnEvent**: Register webhook/event listeners (push mode)
- **Send**: Send data back to the platform

**Built-in Integrations:**
- Telegram (bot API with webhooks)
- Discord (bot with gateway events)
- Google Calendar (OAuth2 with Calendar API)

### Context & Memory

CONTROL maintains context to make informed decisions:

- **Context Files**: JSON documents with labels, categories, tags, and content
- **Vector Store**: Semantic embeddings for similarity-based retrieval
- **Context Retrieval**: Fetches top-K relevant contexts based on event content
- **Categories**: `personal`, `conversation`, `temporary`, `event-log`

### Decision Flow

1. **Event Received** → Normalized and assigned trace ID
2. **Context Retrieved** → Vector search for relevant memories
3. **LLM Decision** → Generate skill call with parameters
4. **Validation** → Check permissions, approval requirements, rate limits
5. **Execution** → Run skill with idempotency and retry logic
6. **Tracing** → Log full decision path

### RBAC (Role-Based Access Control)

Three roles with cascading permissions:
- **Guest**: Read-only access
- **User**: Standard skills and integrations
- **Admin**: All skills including sensitive operations

Skills specify `minRole` in their definition. The RBAC module enforces permissions before execution.

### Approval Workflows

Skills can require manual approval by setting `requiresApproval: true`. Approval requests are:
1. Stored in the execution store with `pending` status
2. Exposed via REST API (`GET /approvals`, `POST /approvals/:id/approve`)
3. Executed only after admin approval

### Rate Limiting

Per-skill rate limits prevent abuse:
- **dailyLimit**: Maximum executions per user per day
- **maxConcurrent**: Maximum simultaneous executions
- Enforced by `RateLimiter` with in-memory state

## Development

### Adding a New Skill

Create a file in `src/skills/` (e.g., `mySkill.ts`):

```typescript
import { z } from 'zod';
import { SkillModule, NormalizedEvent } from '../types/index.js';

const definition = {
  name: 'mySkill',
  description: 'Does something useful',
  paramsSchema: z.object({
    param1: z.string(),
  }),
  tags: ['utility'],
  minRole: 'user' as const,
  requiresApproval: false,
  dailyLimit: 100,
  maxConcurrent: 5,
  priority: 'normal' as const,
  timeoutMs: 10000,
};

const execute = async (params: { param1: string }, event: NormalizedEvent) => {
  // Your logic here
  return { success: true };
};

export default { definition, execute } as SkillModule;
```

Register it in [src/index.ts](src/index.ts) or place it in `src/skills/` for hot-loading.

### Adding a New Integration

Create a file in `src/integrations/` (e.g., `myIntegration.ts`):

```typescript
import { IntegrationModule, NormalizedEvent } from '../types/index.js';

const name = 'myIntegration';

const poll = async (): Promise<NormalizedEvent[] | null> => {
  // Fetch events
  return null;
};

const send = async (payload: Record<string, unknown>) => {
  // Send data to external system
};

const onEvent = (callback: (event: NormalizedEvent) => void) => {
  // Register webhook or event listener
};

export default { name, poll, send, onEvent } as IntegrationModule;
```

Register it in [src/index.ts](src/index.ts) or place it in `src/integrations/` for hot-loading.

### Logging

CONTROL uses [Pino](https://getpino.io/) for structured logging:

```typescript
import { logger } from './logging/logger.js';

logger.info({ userId, skill }, 'Executing skill');
logger.error({ err }, 'Operation failed');
```

Logs are written to:
- Console (pretty-printed in development)
- `logs/app.log` (JSON format)
- `logs/traces/` (decision traces)

### Testing

Tests are written with Vitest:

```bash
# Run all tests
npm test

# Watch mode
npm run test:watch
```

Example test structure:

```typescript
import { describe, it, expect } from 'vitest';

describe('MyModule', () => {
  it('should do something', () => {
    expect(true).toBe(true);
  });
});
```

## API Endpoints

CONTROL exposes a REST API for monitoring and control:

- `GET /health` – Health check endpoint
- `GET /approvals` – List pending approval requests
- `POST /approvals/:id/approve` – Approve a pending execution
- `POST /approvals/:id/reject` – Reject a pending execution

## Monitoring & Observability

### Decision Traces

Every event processing is fully traced and logged to `logs/traces/YYYY-MM-DD.jsonl`:

```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-02-22T10:30:00.000Z",
  "event": { ... },
  "retrievedContexts": [
    { "label": "User preferences", "score": 0.92 }
  ],
  "llmPrompt": "...",
  "llmResponse": "...",
  "decision": {
    "skill": "sendMessage",
    "params": { ... },
    "reasoning": "User requested notification"
  },
  "llmModel": "qwen3:4b",
  "executionTimeMs": 234,
  "status": "success"
}
```

### Heartbeat

The heartbeat module runs periodic health checks and can trigger alerts or recovery actions.

## Troubleshooting

### Ollama Connection Issues

Ensure Ollama is running:
```bash
ollama serve
```

Pull required models:
```bash
ollama pull qwen3:4b
ollama pull mistral:7b
ollama pull phi:2.7b
```

### Integration Webhooks

For Telegram/Discord webhooks to work, `WEBHOOK_BASE_URL` must be publicly accessible. Use ngrok for local development:

```bash
ngrok http 3000
# Update WEBHOOK_BASE_URL in .env with the ngrok URL
```

### Permission Errors

Check that skills specify appropriate `minRole` and events include `role` and `userId` fields.

## License

ISC

## Contributing

Contributions are welcome! Please open an issue or pull request on GitHub.

## Roadmap

- [ ] Web UI for approval workflows and monitoring
- [ ] More integrations (Slack, email, SMS)
- [ ] Voice interface support
- [ ] Multi-tenant isolation
- [ ] Cloud deployment guides (Docker, Kubernetes)
- [ ] Plugin marketplace

---

**Built with ❤️ using TypeScript, Node.js, and AI**