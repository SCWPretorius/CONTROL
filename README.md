# CONTROL – Personal Intelligent Assistant

**CONTROL** is a production-ready, modular, event-driven personal assistant built with TypeScript and Node.js. It processes events from multiple platforms (Telegram, Discord, Google Calendar), uses LLM-based reasoning to determine actions, and executes tasks with built-in safety mechanisms.

Think of it as your own intelligent Jarvis-like system that learns from context and safely automates your daily tasks.

---

## Table of Contents

- [Quick Start](#quick-start)
- [Features](#features)
- [Architecture](#architecture)
- [Installation & Setup](#installation--setup)
- [Configuration](#configuration)
- [Project Structure](#project-structure)
- [Core Concepts](#core-concepts)
- [Development Guide](#development-guide)
- [API Reference](#api-reference)
- [Monitoring & Observability](#monitoring--observability)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)

---

## Quick Start

Get CONTROL running in 5 minutes:

```bash
# Prerequisites: Node.js 20+, Ollama running locally

# Clone and install
git clone https://github.com/SCWPretorius/CONTROL.git
cd CONTROL
npm install

# Set up environment (see Configuration section)
cp .env.example .env
# Edit .env with your API keys

# Build and run
npm run build
npm start

# Or development mode with hot-reload
npm run dev
```

Visit `http://localhost:3000/health` to verify the system is running.

---

## Features

| Feature | Description |
|---------|-------------|
| **🤖 LLM-Powered Intelligence** | Uses Ollama with configurable primary/fallback models (Qwen, Mistral, Phi) for intelligent decision-making |
| **🔌 Multi-Platform Support** | Native integrations for Telegram, Discord, and Google Calendar |
| **🎯 Extensible Skills** | Hot-reloadable modular skills system with automatic discovery |
| **🧠 Context-Aware Memory** | Vector-based semantic search for retrieving relevant historical information |
| **🔒 Enterprise Security** | RBAC, rate limiting, approval workflows, and encrypted secrets |
| **📊 Full Observability** | Comprehensive decision tracing, structured logging, and audit trails |
| **♻️ Resilient Execution** | Idempotent operations, automatic retries, timeout handling, and failure recovery |
| **🔄 Self-Healing** | Automatic recovery of pending executions on restart |
| **💾 Data Management** | Scheduled backups and automatic memory cleanup with retention policies |
| **🏥 Health Monitoring** | Built-in heartbeat monitoring and health check endpoints

## Architecture

CONTROL follows a **modular event-driven architecture** where all activity flows through an event bus and passes through multiple decision gates before execution.

### System Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│ Event Sources                                                   │
│ └─ Telegram, Discord, Google Calendar, Custom Webhooks         │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
                   ┌────────────────┐
                   │  Normalization │  Convert to NormalizedEvent
                   └────────┬───────┘
                            │
                            ▼
                   ┌────────────────┐
                   │  Event Bus     │  Route events to handlers
                   └────────┬───────┘
                            │
            ┌───────────────┼───────────────┐
            │               │               │
            ▼               ▼               ▼
      ┌──────────┐  ┌──────────────┐  ┌──────────┐
      │ Context  │  │  Permission  │  │   Rate   │
      │ Retrieval│  │   Checking   │  │  Limiting│
      └────┬─────┘  └──────┬───────┘  └────┬─────┘
           └───────────────┼───────────────┘
                           │
                           ▼
                  ┌────────────────┐
                  │ LLM Decision   │  Decide which skill to execute
                  │ Engine         │
                  └────────┬───────┘
                           │
                           ▼
                  ┌────────────────────────┐
                  │ Approval Gate (if req) │
                  └────────┬───────────────┘
                           │
                           ▼
                  ┌────────────────────────┐
                  │ Skill Executor         │  Execute with retries & timeouts
                  │ (Idempotent)           │
                  └────────┬───────────────┘
                           │
                           ▼
                  ┌────────────────────────┐
                  │ Integration Send       │  Send response back to source
                  └────────────────────────┘
```

### Key Components

| Component | Purpose |
|-----------|---------|
| **Event Bus** | Central message broker - receives and routes all events throughout the system |
| **Integration Loader** | Discovers and loads integration modules with hot-reload support |
| **Skill Registry** | Manages available skills, validates parameters, handles permissions |
| **LLM Decider** | Analyzes event + context and determines which skill to execute with what parameters |
| **Skill Executor** | Executes skills safely with concurrency control, retries, and timeouts |
| **Context Retriever** | Uses vector search to find relevant historical context for decisions |
| **Context Store** | Persists conversation history and memories as searchable documents |
| **Vector Store** | Maintains semantic embeddings for similarity-based context search |
| **Decision Tracer** | Logs every decision step for full audit trail and debugging |
| **Permission Manager** | Enforces RBAC and decides which users can access which skills |
| **Rate Limiter** | Prevents abuse by enforcing per-user and per-skill rate limits |

### Data Flow Example

1. User sends message via Telegram: `"Schedule meeting with Alice tomorrow at 2pm"`
2. Telegram integration normalizes into `NormalizedEvent`
3. Event routed through event bus
4. System retrieves relevant context (past meetings with Alice, user's calendar habits)
5. Permission checker verifies user has `calendar:write` permission
6. LLM Decider analyzes message + context and decides: `createCalendarEvent` skill
7. Rate limiter confirms user hasn't exceeded daily limits
8. Skill executor runs `createCalendarEvent` with validated parameters
9. Google Calendar integration creates the event
10. Response sent back: `"Meeting with Alice scheduled for tomorrow at 2pm"`
11. Full trace logged with timestamps, models used, and decision reasoning

## Installation & Setup

### Prerequisites

Before installing CONTROL, ensure you have:

| Requirement | Version | Notes |
|-------------|---------|-------|
| **Node.js** | >= 20.0.0 | Use `node --version` to check |
| **npm** | >= 10.0.0 | Comes with Node.js |
| **Ollama** | Latest | Download from [ollama.ai](https://ollama.ai) |
| **Git** | Any recent version | For cloning repository |

### Installation Steps

```bash
# 1. Clone the repository
git clone https://github.com/SCWPretorius/CONTROL.git
cd CONTROL

# 2. Install dependencies
npm install

# 3. Build TypeScript
npm run build

# 4. Create configuration (see next section)
cp .env.example .env
# Edit .env with your settings

# 5. Ensure Ollama is running in another terminal
ollama serve

# 6. In another terminal, pull required models
ollama pull qwen3:4b
ollama pull mistral:7b
ollama pull phi:2.7b

# 7. Start CONTROL
npm start
```

Verify installation by checking health:
```bash
curl http://localhost:3000/health
```

Expected response:
```json
{ "status": "healthy", "timestamp": "2026-02-23T12:34:56.000Z" }
```

---

## Configuration

CONTROL uses environment variables for configuration. Create a `.env` file in the project root.

### Complete Configuration Reference

```env
# ============================================================================
# SERVER CONFIGURATION
# ============================================================================
PORT=3000
HOST=0.0.0.0
WEBHOOK_BASE_URL=http://localhost:3000

# ============================================================================
# LLM CONFIGURATION (Ollama)
# ============================================================================
OLLAMA_URL=http://localhost:11434
LLM_PRIMARY_MODEL=qwen3:4b
LLM_FALLBACK_1=mistral:7b
LLM_FALLBACK_2=phi:2.7b
LLM_TEMPERATURE=0.7
LLM_CONTEXT_SIZE=2048

# ============================================================================
# SECURITY
# ============================================================================
ENCRYPTION_KEY=your-32-character-encryption-key-here
JWT_SECRET=your-jwt-secret-key-here
ADMIN_PASSWORD=your-admin-password

# ============================================================================
# TELEGRAM INTEGRATION (Optional)
# ============================================================================
TELEGRAM_BOT_TOKEN=123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefg
TELEGRAM_WEBHOOK_SECRET=your-webhook-secret-here

# ============================================================================
# DISCORD INTEGRATION (Optional)
# ============================================================================
DISCORD_BOT_TOKEN=your-discord-bot-token-here
DISCORD_PUBLIC_KEY=your-discord-public-key-here

# ============================================================================
# GOOGLE CALENDAR INTEGRATION (Optional)
# ============================================================================
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret-here
GOOGLE_REDIRECT_URI=http://localhost:3000/auth/google/callback

# ============================================================================
# SYSTEM TUNING
# ============================================================================
HEARTBEAT_INTERVAL_MS=30000
BACKUP_SCHEDULE_CRON=0 2 * * *
CLEANUP_INTERVAL_MS=3600000
MAX_CONCURRENT_SKILLS=10
SKILL_TIMEOUT_MS=30000

# ============================================================================
# LOGGING
# ============================================================================
LOG_LEVEL=info
PRETTY_LOGS=true

# ============================================================================
# DATA STORAGE
# ============================================================================
DATA_DIR=./data
```

### Configuration Details

#### LLM Models

CONTROL supports multiple LLM models with fallback:

1. **Primary Model** (`LLM_PRIMARY_MODEL`): Used for all decisions
2. **Fallback 1** (`LLM_FALLBACK_1`): Used if primary fails
3. **Fallback 2** (`LLM_FALLBACK_2`): Final fallback option

Supported models (via Ollama):
- `qwen3:4b` - Fast, good reasoning (recommended)
- `mistral:7b` - Balanced performance
- `phi:2.7b` - Lightweight, fast

#### Integration API Keys

**Getting Each Token:**

**Telegram:**
1. Message `@BotFather` on Telegram
2. Create new bot with `/newbot`
3. Copy the token to `TELEGRAM_BOT_TOKEN`

**Discord:**
1. Go to [Discord Developer Portal](https://discord.com/developers/applications)
2. Create new application
3. Under "Bot", create bot and copy token to `DISCORD_BOT_TOKEN`
4. Copy Public Key to `DISCORD_PUBLIC_KEY`

**Google Calendar:**
1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create new project
3. Enable Google Calendar API
4. Create OAuth credentials (Web application)
5. Copy Client ID and Secret to env variables

#### Security Best Practices

```env
# IMPORTANT: Generate strong encryption key
# On Linux/Mac: openssl rand -base64 32
# On Windows: Use PowerShell:
# [Convert]::ToBase64String([System.Security.Cryptography.RNGCryptoServiceProvider]::new().GetBytes(32))

ENCRYPTION_KEY=your-generated-32-char-key-here
```

### Data Directory Structure

CONTROL automatically creates and manages:

```
data/
├── memory/
│   ├── contexts/           # Stored context files (JSON)
│   │   └── conversation/   # User conversation history
│   └── embeddings/         # Vector embeddings cache
├── executions/             # Execution records and state
└── backups/                # Automated backups (daily)
```

## Development & Testing

### Running CONTROL

**Development Mode** (with hot-reload and pretty logs):
```bash
npm run dev
```
- Watches `src/` for changes
- Auto-restarts on file modifications
- Pretty-printed console logs

**Production Mode** (optimized):
```bash
npm start
```
- Runs compiled JavaScript
- JSON logs to console and files
- Optimized performance

**Type Checking** (without building):
```bash
npm run typecheck
```

### Testing

```bash
# Run all tests once
npm test

# Watch mode for development
npm run test:watch

# Run tests for specific file
npm test -- contextStore.test.ts

# Generate coverage report
npm test -- --coverage
```

Test files are in `tests/` with fixtures in `tests/fixtures/`.


## Project Structure

```
src/
├── index.ts                    # Main entry point - initializes all subsystems
├── backup/
│   └── backupManager.ts        # Scheduled backup creation and management
├── cleanup/
│   └── memoryCleanup.ts        # Memory and context cleanup routines
├── concurrency/
│   └── rateLimiter.ts          # Rate limiting and concurrency control
├── config/
│   ├── config.ts               # Configuration loader and validation
│   └── secrets.ts              # Encrypted secret management
├── events/
│   └── eventBus.ts             # Central event pub/sub system
├── executor/
│   ├── executionStore.ts       # Tracks execution state and retries
│   └── skillExecutor.ts        # Executes skills with safety guardrails
├── heartbeat/
│   └── heartbeat.ts            # Health monitoring and status tracking
├── integrations/               # External platform integrations
│   ├── integrationLoader.ts    # Hot-loads integration modules
│   ├── integrationRunner.ts    # Runs integration polls
│   ├── discord.ts              # Discord bot integration
│   ├── telegram.ts             # Telegram bot integration
│   └── googleCalendar.ts       # Google Calendar API integration
├── llm/
│   ├── llmDecider.ts           # LLM decision engine (skill selection)
│   ├── promptBuilder.ts        # Constructs LLM prompts
│   └── sanitizer.ts            # Input/output sanitization
├── logging/
│   └── logger.ts               # Pino logger configuration
├── memory/
│   ├── contextStore.ts         # Persists and retrieves context files
│   ├── contextRetriever.ts     # Vector search for context
│   └── vectorStore.ts          # Manages embeddings
├── permissions/
│   └── rbac.ts                 # Role-based access control
├── server/
│   ├── server.ts               # Express.js HTTP server
│   └── approvalRoutes.ts       # Approval workflow endpoints
├── skills/                     # Executable skill modules
│   ├── skillLoader.ts          # Hot-loads skill modules
│   ├── skillRegistry.ts        # Manages skill definitions
│   ├── sendMessage.ts          # Send messages to integrations
│   ├── queryCalendar.ts        # Search calendar events
│   ├── createCalendarEvent.ts  # Create calendar events
│   ├── deleteCalendarEvent.ts  # Delete calendar events
│   └── updateContext.ts        # Store info in memory
├── tracing/
│   └── decisionTracer.ts       # Full decision trace logging
└── types/
    └── index.ts                # TypeScript type definitions

tests/                          # Test suite
├── contextStore.test.ts        # Context store tests
├── rateLimiter.test.ts         # Rate limiter tests
├── rbac.test.ts                # RBAC permission tests
├── sanitizer.test.ts           # Input sanitization tests
└── fixtures/                   # Test scenarios
    ├── calendar-conflict.test.ts
    └── llm-fallback.test.ts

data/                           # Runtime data (gitignored)
├── memory/                     # Stored contexts and embeddings
├── executions/                 # Execution records
├── backups/                    # Scheduled backups
logs/                           # Application logs (gitignored)
├── app.log                     # Main application log
├── traces/                     # Decision traces
└── decision-traces/            # Archived traces
```

### Module Responsibilities

| Module | Responsibility |
|--------|-----------------|
| **Config** | Load and validate environment variables and secrets |
| **Event Bus** | Pub/sub for internal events, coordinates all subsystems |
| **Integrations** | Poll/webhook adapters for external platforms |
| **Skills** | Executable actions with clear contracts and safety constraints |
| **LLM Decider** | Analyzes context + event → selects skill and parameters |
| **Memory** | Context persistence, vector search, conversation history |
| **Executor** | Runs skills safely with retries, timeouts, idempotency |
| **Permissions** | Validates user roles and skill access requirements |
| **Server** | HTTP API for monitoring, approval, and webhooks |
| **Logging** | Structured logging with traceability and audit trails |

## Core Concepts

### Events

All inputs to CONTROL are normalized into `NormalizedEvent` objects, creating a unified interface across all sources.

```typescript
interface NormalizedEvent {
  id: string;                        // Unique event ID (UUID)
  traceId: string;                   // Distributed trace ID
  source: string;                    // Origin: 'telegram', 'discord', 'calendar', etc
  type: string;                      // Event type: 'message', 'calendar-event', 'notification'
  payload: Record<string, unknown>;  // Event-specific data
  timestamp: string;                 // ISO 8601 timestamp
  userId?: string;                   // User identifier (for RBAC)
  role?: 'guest' | 'user' | 'admin'; // User role
  tenantId?: string;                 // Tenant ID (multi-tenancy)
}
```

**Example Events:**

```typescript
// Message from Telegram
{
  id: "evt_123",
  traceId: "trace_456",
  source: "telegram",
  type: "message",
  payload: {
    text: "Schedule meeting with Alice tomorrow",
    chatId: "789",
    sender: "John"
  },
  userId: "user_john",
  role: "user",
  timestamp: "2026-02-23T12:34:00Z"
}

// Calendar event from Google Calendar
{
  id: "evt_124",
  traceId: "trace_457",
  source: "googleCalendar",
  type: "calendar-event",
  payload: {
    eventId: "cal_xyz",
    summary: "Team Standup",
    startTime: "2026-02-24T09:00:00Z"
  },
  userId: "user_john",
  role: "user",
  timestamp: "2026-02-23T08:00:00Z"
}
```

### Skills

Skills are executable actions with defined capabilities and safety constraints. Each skill encapsulates business logic.

**Skill Definition:**

```typescript
interface SkillDefinition {
  name: string;                      // Unique skill identifier
  description: string;               // Human-readable description
  paramsSchema: ZodSchema;           // Parameter validation schema
  tags: string[];                    // Categorization tags
  minRole: 'guest' | 'user' | 'admin'; // Minimum role required
  requiresApproval: boolean;         // Requires manual approval
  dailyLimit: number;                // Max executions per user per day
  maxConcurrent: number;             // Max simultaneous executions
  priority: 'low' | 'normal' | 'high'; // Execution priority
  timeoutMs: number;                 // Execution timeout
}
```

**Example Skill:**

```typescript
// src/skills/sendMessage.ts
import { z } from 'zod';
import { SkillModule, NormalizedEvent } from '../types/index.js';

const definition = {
  name: 'sendMessage',
  description: 'Send a message via Telegram, Discord, or other integrations',
  paramsSchema: z.object({
    destination: z.enum(['telegram', 'discord']),
    recipientId: z.string(),
    message: z.string().max(4096),
  }),
  tags: ['communication', 'messaging'],
  minRole: 'user',
  requiresApproval: false,
  dailyLimit: 100,
  maxConcurrent: 10,
  priority: 'normal',
  timeoutMs: 15000,
};

const execute = async (
  params: z.infer<typeof definition.paramsSchema>,
  event: NormalizedEvent
) => {
  // Implementation
  return { success: true, messageId: '...' };
};

export default { definition, execute } as SkillModule;
```

**Built-in Skills:**

| Skill | Purpose | Requires Approval |
|-------|---------|-------------------|
| `sendMessage` | Send messages to users via integrations | No |
| `queryCalendar` | Search and retrieve calendar events | No |
| `createCalendarEvent` | Create new calendar events | Yes (recommended) |
| `deleteCalendarEvent` | Delete calendar events | Yes |
| `updateContext` | Store information in memory system | No |

### Integrations

Integrations connect CONTROL to external platforms. Each integration provides three main methods:

```typescript
interface IntegrationModule {
  name: string;                    // Unique integration name
  
  poll?: () => Promise<NormalizedEvent[] | null>;
  // Optional: Called periodically to fetch events (polling mode)
  
  onEvent?: (callback: (event: NormalizedEvent) => void) => void;
  // Optional: Register for push notifications (webhook/event mode)
  
  send: (payload: Record<string, unknown>) => Promise<void>;
  // Send data back to the platform
}
```

**Built-in Integrations:**

| Integration | Method | Features |
|-------------|--------|----------|
| **Telegram** | Push (webhooks) | Bot API, inline buttons, message editing |
| **Discord** | Push (gateway) | Bot commands, reactions, thread support |
| **Google Calendar** | Poll/Push | OAuth2, event creation, queries |

**Custom Integration Example:**

```typescript
// src/integrations/email.ts
import { IntegrationModule, NormalizedEvent } from '../types/index.js';

const name = 'email';

const send = async (payload: { to: string; subject: string; body: string }) => {
  // Send email using your provider
};

export default { name, send, poll: null, onEvent: null } as IntegrationModule;
```

### Context & Memory

CONTROL maintains context to make intelligent decisions. Context is stored as documents and indexed for semantic search.

**Context File Structure:**

```json
{
  "id": "ctx_user_preferences",
  "label": "User Preferences",
  "category": "personal",
  "tags": ["preferences", "settings", "user"],
  "content": "John prefers meetings after 2pm. Uses calendar for work only...",
  "embedding": [0.234, -0.19, ...],
  "createdAt": "2026-02-20T10:00:00Z",
  "updatedAt": "2026-02-23T14:30:00Z"
}
```

**Context Categories:**

- `personal` - User preferences, habits, settings
- `conversation` - Chat history and interactions
- `temporary` - Transient data, expires after time
- `event-log` - Historical events, audit trail

**Memory Operations:**

```typescript
// Store context
await contextStore.save({
  label: "Meeting with Alice",
  category: "conversation",
  content: "Discussed Q1 roadmap...",
});

// Retrieve relevant contexts
const contexts = await contextRetriever.retrieve(
  userId,
  eventType,
  keywords: ["Alice", "meeting", "roadmap"],
  limit: 3 // Top 3 matches
);
```

### Decision Flow (In Detail)

```
1. EVENT RECEIVED
   └─ Normalize to NormalizedEvent
   └─ Assign trace ID for tracing
   └─ Log metadata

2. CONTEXT RETRIEVAL
   └─ Extract keywords from event
   └─ Vector search across stored contexts
   └─ Return top-K relevant matches with similarity scores

3. LLM DECISION
   └─ Build prompt with:
      - Event details
      - Retrieved contexts
      - Available skills
      - User role/permissions
   └─ Call LLM (with fallbacks)
   └─ Parse response: skill name + parameters

4. VALIDATION & CHECKING
   └─ Validate parameters against skill schema
   └─ Check user permissions against minRole
   └─ Check rate limits (daily, concurrent)
   └─ Determine approval requirements

5. APPROVAL GATE (if needed)
   └─ Store pending execution
   └─ Send approval request
   └─ Wait for admin approval
   └─ (Or proceed if auto-approved)

6. EXECUTION
   └─ Acquire concurrency token
   └─ Execute skill with timeout
   └─ Implement idempotency (prevent duplicates)
   └─ Retry on failure (exponential backoff)
   └─ Release concurrency token

7. RESPONSE
   └─ Format response for integration
   └─ Send via integration.send()
   └─ Log execution record

8. TRACING
   └─ Write full trace to logs/traces/
   └─ Include timing, model, decision path
   └─ Enable debugging and auditing
```

### RBAC (Role-Based Access Control)

Three roles with increasing permissions:

```typescript
interface Role {
  name: 'guest' | 'user' | 'admin';
  permissions: Set<string>;
}
```

**Permission Hierarchy:**
- **Guest**: Read-only access to non-sensitive information
- **User**: Execute standard skills, manage own contexts
- **Admin**: All skills, user management, system config, approval authority

Skills specify their `minRole` requirement:

```typescript
const definition = {
  name: 'deleteCalendarEvent',
  minRole: 'admin',  // Only admins can delete events
  // ...
};
```

### Approval Workflows

Skills can require manual approval by setting `requiresApproval: true`. This creates a two-stage execution flow:

```
Stage 1: PENDING
├─ User triggers execution
├─ System validates permissions & rate limits
├─ Stores execution request with status: "pending"
└─ Sends approval request to admin

Stage 2: APPROVAL
├─ Admin reviews request
├─ Can approve or reject
└─ If approved, transitions to "approved"

Stage 3: EXECUTION
├─ Skill executor fetches approved requests
├─ Executes with idempotency key
├─ Updates status: "completed" or "failed"
└─ Sends response to user

```

REST API for approvals:
```bash
# List pending approvals
GET /approvals

# Approve execution
POST /approvals/:executionId/approve

# Reject execution
POST /approvals/:executionId/reject
```

### Rate Limiting

Prevents abuse by enforcing limits per user per skill:

```typescript
// Skill definition
{
  dailyLimit: 10,      // Max 10 executions per user per day
  maxConcurrent: 3,    // Max 3 simultaneous executions
}
```

Rate limiter tracks:
- Daily execution count per (user, skill) pair
- Current concurrent execution count
- Resets daily at UTC midnight

Exceeding limits returns: `RateLimitExceeded error`

## Development Guide

### Adding a New Skill

Skills are modular, hot-reloadable actions. Here's how to create one:

**Step 1: Create the skill file**

```typescript
// src/skills/summarizeMessages.ts
import { z } from 'zod';
import { SkillModule, NormalizedEvent } from '../types/index.js';
import { logger } from '../logging/logger.js';

const definition = {
  name: 'summarizeMessages',
  description: 'Summarize recent messages into a brief recap',
  paramsSchema: z.object({
    userId: z.string(),
    numMessages: z.number().min(1).max(50),
    format: z.enum(['bullet-points', 'paragraph']).optional(),
  }),
  tags: ['communication', 'summarization', 'analysis'],
  minRole: 'user' as const,
  requiresApproval: false,
  dailyLimit: 50,
  maxConcurrent: 5,
  priority: 'normal' as const,
  timeoutMs: 20000,
};

const execute = async (
  params: z.infer<typeof definition.paramsSchema>,
  event: NormalizedEvent
) => {
  logger.info({ userId: params.userId }, 'Starting message summarization');
  
  try {
    // Your implementation here
    const summary = await summarizeUserMessages(
      params.userId,
      params.numMessages,
      params.format || 'paragraph'
    );
    
    logger.info({ userId: params.userId }, 'Summarization complete');
    return { success: true, summary };
  } catch (error) {
    logger.error({ error, userId: params.userId }, 'Summarization failed');
    throw error;
  }
};

async function summarizeUserMessages(
  userId: string,
  count: number,
  format: 'bullet-points' | 'paragraph'
): Promise<string> {
  // Fetch messages from context store
  // Call LLM to summarize
  // Return formatted result
  return 'Summary here...';
}

export default { definition, execute } as SkillModule;
```

**Step 2: Register the skill**

The skill will be automatically hot-loaded if placed in `src/skills/`. To manually register:

```typescript
// In src/index.ts
import summarizeMessagesSkill from './skills/summarizeMessages.js';

// Register
skillRegistry.register(summarizeMessagesSkill);
```

**Step 3: Test the skill**

```typescript
// tests/summarizeMessages.test.ts
import { describe, it, expect, beforeEach } from 'vitest';
import summarizeMessagesSkill from '../src/skills/summarizeMessages';
import { NormalizedEvent } from '../src/types/index';

describe('summarizeMessages', () => {
  const mockEvent: NormalizedEvent = {
    id: 'evt_123',
    traceId: 'trace_123',
    source: 'test',
    type: 'test',
    payload: {},
    timestamp: new Date().toISOString(),
    userId: 'user_123',
  };

  it('should summarize messages correctly', async () => {
    const result = await summarizeMessagesSkill.execute(
      {
        userId: 'user_123',
        numMessages: 10,
        format: 'bullet-points',
      },
      mockEvent
    );

    expect(result.success).toBe(true);
    expect(result.summary).toBeDefined();
  });

  it('should respect parameter constraints', async () => {
    expect(() => {
      summarizeMessagesSkill.definition.paramsSchema.parse({
        userId: 'user_123',
        numMessages: 100, // Exceeds max of 50
      });
    }).toThrow();
  });
});
```

### Adding a New Integration

Integrations connect external platforms to CONTROL.

**Step 1: Create the integration**

```typescript
// src/integrations/slack.ts
import axios from 'axios';
import { IntegrationModule, NormalizedEvent } from '../types/index.js';
import { logger } from '../logging/logger.js';
import { config } from '../config/config.js';

const name = 'slack';

const poll = async (): Promise<NormalizedEvent[] | null> => {
  // Slack doesn't typically support polling for messages
  // Return null to skip polling
  return null;
};

const onEvent = (callback: (event: NormalizedEvent) => void) => {
  // Set up webhook to receive Slack events
  // For example, listen to incoming slash commands or events
  logger.info('Slack event listener registered');
};

const send = async (payload: {
  channel: string;
  text: string;
  blocks?: Record<string, unknown>[];
}) => {
  try {
    await axios.post(
      `https://slack.com/api/chat.postMessage`,
      {
        channel: payload.channel,
        text: payload.text,
        blocks: payload.blocks,
      },
      {
        headers: {
          Authorization: `Bearer ${config.SLACK_BOT_TOKEN}`,
          'Content-Type': 'application/json',
        },
      }
    );
    logger.info({ channel: payload.channel }, 'Slack message sent');
  } catch (error) {
    logger.error({ error, channel: payload.channel }, 'Failed to send Slack message');
    throw error;
  }
};

export default { name, poll, onEvent, send } as IntegrationModule;
```

**Step 2: Add environment variables**

```env
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_WEBHOOK_SECRET=your-signing-secret
```

**Step 3: Update HTTP routes** (if webhook needed)

```typescript
// src/server/server.ts
app.post('/webhooks/slack', (req, res) => {
  const signature = req.headers['x-slack-signature'];
  const timestamp = req.headers['x-slack-request-timestamp'];
  
  // Verify signature
  if (!verifySlackSignature(signature, timestamp, req.body)) {
    return res.status(401).json({ error: 'Unauthorized' });
  }
  
  // Process event
  eventBus.emit('slack-event', req.body);
  res.status(200).json({ ok: true });
});
```

### Structured Logging

Use Pino for all logging:

```typescript
import { logger } from './logging/logger.js';

// Info level
logger.info(
  { userId: 'user_123', skill: 'sendMessage', duration: 150 },
  'Skill executed successfully'
);

// Warning level  
logger.warn(
  { skill: 'deleteCalendarEvent', approved: false },
  'Skill requires approval'
);

// Error level with error object
logger.error(
  { error: someErr, userId: 'user_123', skill: 'queryCalendar' },
  'Skill execution failed'
);

// Debug level (not in production by default)
logger.debug(
  { context: { retrieved: 3, scores: [0.92, 0.85, 0.71] } },
  'Context retrieval complete'
);
```

Logs are automatically:
- **Console**: Pretty-printed in dev, JSON in production
- **File**: Written to `logs/app.log` in JSON format
- **Traces**: Decision traces written to `logs/traces/YYYY-MM-DD.jsonl`

### Testing Guidelines

**Test Categories:**

1. **Unit Tests**: Test individual functions
```typescript
describe('RateLimiter', () => {
  it('should enforce daily limits', async () => {
    // Test implementation
  });
});
```

2. **Integration Tests**: Test component communication
```typescript
describe('Event Flow', () => {
  it('should process event through full pipeline', async () => {
    // Test from event → decision → execution
  });
});
```

3. **Fixture Tests**: Test specific scenarios
```typescript
// tests/fixtures/calendar-conflict.test.ts
// Test how system handles calendar conflicts
```

**Running Tests:**

```bash
# All tests
npm test

# Specific file
npm test -- contextStore.test.ts

# Watch mode
npm run test:watch

# With coverage
npm test -- --coverage
```

## API Reference

CONTROL exposes a REST API for monitoring, control, and integration.

### Health & Status Endpoints

#### GET /health
Check system health status.

**Request:**
```
GET /health
```

**Response (200):**
```json
{
  "status": "healthy",
  "timestamp": "2026-02-23T12:34:56.000Z",
  "uptime": 3600000,
  "components": {
    "eventBus": "ok",
    "ollama": "ok",
    "contextStore": "ok",
    "integrations": {
      "telegram": "ok",
      "discord": "ok",
      "googleCalendar": "ok"
    }
  }
}
```

### Approval Management Endpoints

#### GET /approvals
List pending approval requests.

**Query Parameters:**
| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `status` | string | Filter by status: `pending`, `approved`, `rejected` | All |
| `limit` | number | Max results to return | 50 |
| `offset` | number | Pagination offset | 0 |

**Request:**
```bash
GET /approvals?status=pending&limit=10
```

**Response (200):**
```json
{
  "approvals": [
    {
      "id": "exec_abc123",
      "status": "pending",
      "skill": "deleteCalendarEvent",
      "userId": "user_john",
      "parameters": {
        "eventId": "evt_xyz"
      },
      "createdAt": "2026-02-23T12:00:00Z",
      "expiresAt": "2026-02-23T18:00:00Z"
    }
  ],
  "total": 5,
  "limit": 10,
  "offset": 0
}
```

#### POST /approvals/:executionId/approve
Approve a pending execution.

**Authentication:** Requires `role: admin`

**Request:**
```bash
POST /approvals/exec_abc123/approve
Content-Type: application/json

{
  "comment": "Looks good, proceeding"
}
```

**Response (200):**
```json
{
  "id": "exec_abc123",
  "status": "approved",
  "approvedBy": "user_admin",
  "approvedAt": "2026-02-23T12:05:00Z",
  "executionStarted": true
}
```

#### POST /approvals/:executionId/reject
Reject a pending execution.

**Authentication:** Requires `role: admin`

**Request:**
```bash
POST /approvals/exec_abc123/reject
Content-Type: application/json

{
  "reason": "Event is already in the past"
}
```

**Response (200):**
```json
{
  "id": "exec_abc123",
  "status": "rejected",
  "rejectedBy": "user_admin",
  "rejectedAt": "2026-02-23T12:05:00Z"
}
```

### Context Management Endpoints

#### GET /context
Retrieve stored contexts.

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `label` | string | Filter by label |
| `category` | string | Filter by category |
| `tags` | string | Filter by comma-separated tags |

**Request:**
```bash
GET /context?category=conversation&tags=meeting,alice
```

**Response (200):**
```json
{
  "contexts": [
    {
      "id": "ctx_123",
      "label": "Meeting with Alice",
      "category": "conversation",
      "tags": ["meeting", "alice", "discussion"],
      "content": "Discussed...",
      "createdAt": "2026-02-20T10:00:00Z",
      "updatedAt": "2026-02-23T14:30:00Z"
    }
  ],
  "total": 1
}
```

#### POST /context
Create or update a context.

**Request:**
```bash
POST /context
Content-Type: application/json

{
  "label": "Q1 Roadmap Discussion",
  "category": "conversation",
  "tags": ["roadmap", "planning", "q1"],
  "content": "Met with team to discuss Q1 priorities..."
}
```

**Response (201):**
```json
{
  "id": "ctx_124",
  "label": "Q1 Roadmap Discussion",
  "category": "conversation",
  "createdAt": "2026-02-23T14:35:00Z"
}
```

#### DELETE /context/:contextId
Delete a context document.

**Request:**
```bash
DELETE /context/ctx_123
```

**Response (204):** No content

### Execution History Endpoints

#### GET /executions
Retrieve execution history.

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | string | Filter by: `pending`, `executing`, `completed`, `failed` |
| `skill` | string | Filter by skill name |
| `userId` | string | Filter by user |
| `limit` | number | Max results (default: 50) |

**Request:**
```bash
GET /executions?status=completed&skill=sendMessage&limit=20
```

**Response (200):**
```json
{
  "executions": [
    {
      "id": "exec_001",
      "traceId": "trace_001",
      "skill": "sendMessage",
      "userId": "user_john",
      "status": "completed",
      "parameters": { "destination": "telegram", "message": "Hello" },
      "result": { "success": true, "messageId": "msg_123" },
      "startedAt": "2026-02-23T12:00:00Z",
      "completedAt": "2026-02-23T12:00:05Z",
      "durationMs": 5000
    }
  ],
  "total": 100,
  "limit": 20
}
```

#### GET /executions/:executionId
Get details of a specific execution.

**Request:**
```bash
GET /executions/exec_001
```

**Response (200):**
```json
{
  "id": "exec_001",
  "traceId": "trace_001",
  "skill": "sendMessage",
  "status": "completed",
  "parameters": { ... },
  "result": { ... },
  "trace": {
    "event": { ... },
    "retrievedContexts": [ ... ],
    "llmPrompt": "...",
    "llmResponse": "...",
    "llmModel": "qwen3:4b",
    "decision": { ... }
  },
  "timestamps": {
    "received": "2026-02-23T12:00:00Z",
    "started": "2026-02-23T12:00:01Z",
    "completed": "2026-02-23T12:00:05Z"
  },
  "metrics": {
    "durationMs": 5000,
    "retrialCount": 0
  }
}
```

### Skill Registry Endpoint

#### GET /skills
List available skills and their definitions.

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `tag` | string | Filter by tag |
| `minRole` | string | Filter by minimum role |

**Request:**
```bash
GET /skills?tag=communication&minRole=user
```

**Response (200):**
```json
{
  "skills": [
    {
      "name": "sendMessage",
      "description": "Send a message via integrations",
      "parameters": {
        "destination": { "type": "string", "enum": ["telegram", "discord"] },
        "recipientId": { "type": "string" },
        "message": { "type": "string", "maxLength": 4096 }
      },
      "tags": ["communication"],
      "minRole": "user",
      "requiresApproval": false,
      "limits": {
        "dailyLimit": 100,
        "maxConcurrent": 10
      },
      "performance": {
        "timeoutMs": 15000,
        "priority": "normal"
      }
    }
  ],
  "total": 1
}
```

### Error Responses

All error responses follow this format:

```json
{
  "error": "SkillExecutionError",
  "message": "Skill execution failed",
  "code": "SKILL_EXECUTION_FAILED",
  "details": {
    "skill": "sendMessage",
    "reason": "Rate limit exceeded",
    "retryAfter": 3600
  },
  "traceId": "trace_123"
}
```

**Common HTTP Status Codes:**

| Status | Meaning |
|--------|---------|
| 200 | Success |
| 201 | Created |
| 204 | No Content |
| 400 | Bad Request |
| 401 | Unauthorized |
| 403 | Forbidden |
| 404 | Not Found |
| 429 | Too Many Requests (Rate Limited) |
| 500 | Internal Server Error |

### Webhooks

External systems can send webhooks to CONTROL:

**Webhook Endpoints:**

```
POST /webhooks/telegram
POST /webhooks/discord
POST /webhooks/custom
```

**Example Webhook Payload:**

```json
{
  "type": "message",
  "source": "telegram",
  "userId": "user_123",
  "payload": {
    "text": "Hello CONTROL",
    "chatId": "789"
  }
}
```

## Monitoring & Observability

### Logging System

CONTROL uses **Pino** for structured, high-performance logging with automatic indexing.

**Log Levels (Priority Order):**
1. **fatal** - System cannot continue
2. **error** - Operations failed, manual intervention may be needed
3. **warn** - Unusual condition, system recovering
4. **info** - Normal operational events
5. **debug** - Detailed diagnostic information (dev only)
6. **trace** - Most verbose, per-operation details

**Log Outputs:**

```
┌─────────────────────────────────────────────────────────────────┐
│                    LOGGING OUTPUT STREAMS                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Console                        │ Files                         │
│ ├─ Pretty (dev)                │ ├─ logs/app.log (JSON)       │
│ └─ JSON (prod)                 │ ├─ logs/traces/*.jsonl       │
│                                │ └─ logs/errors/*.log         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Example Log Entry:**

```json
{
  "level": 30,
  "time": "2026-02-23T12:34:56.789Z",
  "userId": "user_john",
  "skill": "sendMessage",
  "duration": 234,
  "traceId": "trace_123",
  "msg": "Skill executed successfully",
  "v": 1
}
```

### Decision Traces

Every event processing is fully traced and recorded for debugging and auditing.

**Trace Location:** `logs/traces/YYYY-MM-DD.jsonl` (one JSON object per line)

**Trace Structure:**

```json
{
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-02-23T12:34:00.000Z",
  "event": {
    "id": "evt_abc123",
    "source": "telegram",
    "type": "message",
    "payload": {
      "text": "Schedule a meeting tomorrow"
    },
    "userId": "user_john",
    "role": "user"
  },
  "retrievedContexts": [
    {
      "label": "User preferences",
      "score": 0.92,
      "category": "personal"
    },
    {
      "label": "Calendar habits",
      "score": 0.87,
      "category": "personal"
    }
  ],
  "llmPrompt": "Given the event and contexts, select a skill...",
  "llmResponse": {
    "skill": "createCalendarEvent",
    "parameters": {
      "title": "Meeting",
      "time": "2026-02-24T10:00:00Z"
    },
    "reasoning": "User requested calendar event creation"
  },
  "decision": {
    "skill": "createCalendarEvent",
    "parameters": {
      "title": "Meeting",
      "time": "2026-02-24T10:00:00Z"
    },
    "approvalRequired": true,
    "rateLimitStatus": "ok"
  },
  "llmModel": "qwen3:4b",
  "executionTime": {
    "contextRetrieval": 45,
    "llmDecision": 189,
    "skillExecution": 0,
    "total": 234
  },
  "status": "pending_approval",
  "errors": null
}
```

### Health Monitoring

The heartbeat module runs periodic checks and exposes system health:

```bash
# Check system health
curl http://localhost:3000/health

# Returns
{
  "status": "healthy",
  "timestamp": "2026-02-23T12:35:00Z",
  "uptime": 3600000,
  "components": {
    "eventBus": "ok",
    "ollama": "ok",
    "contextStore": "ok"
  },
  "metrics": {
    "pendingExecutions": 2,
    "failureRate": 0.001,
    "averageResponseTime": 234
  }
}
```

### Metrics to Monitor

**System Level:**
- Uptime (should be very high)
- Error rate (should be < 1%)
- Average response time (target < 500ms)
- Memory usage (should be stable)

**Operational:**
- Pending approvals count
- Rate limit violations (shouldn't happen with good limits)
- Failed skill executions
- Ollama connection availability

**Business:**
- Skills executed per day
- Popular skills
- User engagement
- Decision reversal rate

### Viewing Logs

**Recent logs:**
```bash
# Last 10 entries
tail -n 10 logs/app.log

# Stream logs as they arrive
tail -f logs/app.log

# Pretty print JSON logs
tail -f logs/app.log | jq '.'

# Filter by user
tail -f logs/app.log | jq 'select(.userId == "user_john")'

# Filter by skill
cat logs/app.log | jq 'select(.skill == "sendMessage")'
```

**Decision traces:**
```bash
# View today's traces
cat logs/traces/$(date +%Y-%m-%d).jsonl | jq '.'

# Find slow decisions (> 500ms)
cat logs/traces/$(date +%Y-%m-%d).jsonl | jq 'select(.executionTime.total > 500)'

# Find errors
cat logs/traces/$(date +%Y-%m-%d).jsonl | jq 'select(.errors != null)'
```

### Alerting & Monitoring Integration

For production monitoring, integrate with external systems:

**Example: Datadog Integration**
```typescript
// src/logging/logger.ts
import StatsD from 'node-dogstatsd';

const client = new StatsD.StatsD();

logger.on('error', (log) => {
  client.increment('control.errors', 1, { skill: log.skill });
  client.gauge('control.error_rate', calculateErrorRate());
});
```

**Example: Sentry Integration**
```typescript
import * as Sentry from "@sentry/node";

Sentry.init({
  dsn: process.env.SENTRY_DSN,
  environment: process.env.NODE_ENV,
});

logger.on('error', (log) => {
  Sentry.captureException(log.error, {
    tags: { skill: log.skill, userId: log.userId }
  });
});
```

### Performance Profiling

Use Node.js built-in tools:

```bash
# Generate CPU profile
node --prof src/index.ts
# Stop after 60 seconds, then:
node --prof-process isolate-*.log > profile.txt

# Use Chrome DevTools
node --inspect src/index.ts
# Visit chrome://inspect
```

## Troubleshooting

### Starting CONTROL

**Problem: "Cannot find module" error**
```
Error: Cannot find module './config/config.js'
```

**Solution:**
Make sure to build TypeScript first:
```bash
npm run build
npm start
```

---

### Ollama & LLM Issues

**Problem: Connection refused at `http://localhost:11434`**

**Solution:**
1. Ensure Ollama is running:
   ```bash
   ollama serve
   ```
   You should see: `Listening on 127.0.0.1:11434`

2. Verify connectivity:
   ```bash
   curl http://localhost:11434/api/status
   ```

3. Check logs for Ollama errors

**Problem: `Model not found: qwen3:4b`**

**Solution:**
Pull the required models:
```bash
ollama pull qwen3:4b
ollama pull mistral:7b
ollama pull phi:2.7b
```

Verify:
```bash
ollama list
```

**Problem: LLM responses are slow or timeout**

**Solution:**
1. Reduce model size:
   ```env
   LLM_PRIMARY_MODEL=phi:2.7b  # Smaller, faster
   ```

2. Check Ollama logs for resource issues

3. Increase timeout:
   ```env
   LLM_TIMEOUT_MS=60000  # Increase from 30000
   ```

4. Monitor Ollama resource usage:
   ```bash
   # macOS/Linux
   top -p $(pgrep ollama)
   
   # Windows
   tasklist /FI "IMAGENAME eq ollama.exe"
   ```

---

### Integration Issues

**Problem: Telegram webhook not receiving events**

**Solution:**
1. Verify `WEBHOOK_BASE_URL` is publicly accessible:
   ```bash
   curl https://yourdomain.com/health
   ```

2. For local development, use ngrok:
   ```bash
   ngrok http 3000
   # Output: https://abc123.ngrok.io
   ```
   
   Update `.env`:
   ```env
   WEBHOOK_BASE_URL=https://abc123.ngrok.io
   ```

3. Verify Telegram webhook is registered:
   ```bash
   curl https://api.telegram.org/bot{TOKEN}/getWebhookInfo
   ```

4. Check webhook logs:
   ```bash
   tail -f logs/app.log | jq 'select(.source == "telegram")'
   ```

**Problem: Discord bot not responding to commands**

**Solution:**
1. Verify bot token is correct:
   ```bash
   # Check in Discord Developer Portal
   # Settings → Bot → Copy Token
   ```

2. Verify bot has required permissions:
   ```
   - Send Messages
   - Read Messages/View Channels
   - Read Message History
   - Mention @everyone, @here, and All Roles
   ```

3. Verify bot is added to server with correct scope:
   - OAuth2 URL: `https://discord.com/api/oauth2/authorize?client_id=YOUR_ID&scope=bot&permissions=2048`

4. Check Discord events:
   ```bash
   tail -f logs/app.log | jq 'select(.source == "discord")'
   ```

**Problem: Google Calendar authentication failures**

**Solution:**
1. Verify OAuth2 credentials are valid

2. Check redirect URI matches:
   ```env
   GOOGLE_REDIRECT_URI=http://localhost:3000/auth/google/callback
   # Must match exactly in Google Console
   ```

3. Refresh access token:
   ```bash
   # Delete the cached token
   rm data/google-auth-token.json
   # Restart and re-authenticate
   npm start
   ```

4. Check token expiration:
   ```bash
   cat data/google-auth-token.json | jq '.expiry_date'
   ```

---

### Permissions & RBAC Issues

**Problem: `Permission denied` error executing skill**

**Diagnosis:**
```bash
# Check execution logs
tail -f logs/app.log | jq 'select(.skill == "deleteCalendarEvent")'
```

**Solution:**
1. Verify user role is high enough:
   ```typescript
   // Skill definition requires specific role
   minRole: 'admin'  // User needs admin role
   ```

2. Verify event includes user role:
   ```json
   {
     "userId": "user_john",
     "role": "admin",  // Must match skill requirement
   }
   ```

3. Grant user permission:
   - Add user to admin group
   - Or lower skill's `minRole` requirement

---

### Rate Limiting Issues

**Problem: `RateLimitExceeded` errors**

**Diagnosis:**
```bash
# Check which limits are being hit
tail -f logs/app.log | jq 'select(.error == "RateLimitExceeded")'
```

**Solution:**
1. Increase skill limits:
   ```typescript
   const definition = {
     name: 'sendMessage',
     dailyLimit: 100,      // Increase from 50
     maxConcurrent: 10,    // Increase from 5
   };
   ```

2. Check if legitimate traffic is being blocked:
   - Review use patterns in execution logs
   - Adjust limits based on actual needs

3. Implement tiered limits:
   ```typescript
   const limits = {
     'admin': { dailyLimit: 1000, maxConcurrent: 50 },
     'user': { dailyLimit: 100, maxConcurrent: 10 },
     'guest': { dailyLimit: 10, maxConcurrent: 1 },
   };
   ```

---

### Memory & Performance Issues

**Problem: High memory usage**

**Solution:**
1. Check scheduled cleanup tasks:
   ```bash
   tail -f logs/app.log | grep -i cleanup
   ```

2. Adjust cleanup interval:
   ```env
   CLEANUP_INTERVAL_MS=1800000  # 30 minutes
   ```

3. Reduce context retention:
   ```typescript
   // src/cleanup/memoryCleanup.ts
   RETENTION_DAYS: 7  // Keep only 7 days of history
   ```

4. Monitor memory:
   ```bash
   # Node.js process
   node -e "setInterval(() => console.log(process.memoryUsage()), 5000)" src/index.ts
   ```

**Problem: Slow context retrieval**

**Solution:**
1. Check vector embeddings:
   ```bash
   # Monitor embedding generation
   tail -f logs/app.log | jq 'select(.action == "embedding")'
   ```

2. Reduce context size:
   ```typescript
   // src/memory/contextRetriever.ts
   const limit = 3;  // Retrieve fewer contexts
   ```

3. Use exact search instead of semantic:
   ```typescript
   // src/memory/contextRetriever.ts
   useSemanticSearch: false  // Use simple keyword matching
   ```

---

### Database & Storage Issues

**Problem: `data/` directory permission errors**

**Solution:**
```bash
# Ensure data directory is writable
chmod -R 755 data/

# Check permissions
ls -la data/
```

**Problem: Backup failures**

**Diagnosis:**
```bash
tail -f logs/app.log | jq 'select(.action == "backup")'
```

**Solution:**
1. Check disk space:
   ```bash
   df -h  # Check available space
   ```

2. Verify backup schedule:
   ```env
   BACKUP_SCHEDULE_CRON=0 2 * * *  # Daily at 2 AM
   ```

3. Manually trigger backup:
   ```typescript
   // In src/index.ts
   import { scheduleBackups } from './backup/backupManager.js';
   await scheduleBackups();
   ```

---

### Application Crashes

**Problem: Application exits unexpectedly**

**Diagnosis:**
```bash
# Check exit code
echo $?  # 0 = success, non-zero = error
```

**Solution:**
1. Check error logs:
   ```bash
   cat logs/app.log | jq 'select(.level >= 40)'  # Errors and above
   ```

2. Run with detailed logging:
   ```bash
   LOG_LEVEL=debug npm start
   ```

3. Check for uncaught exceptions:
   ```bash
   tail -f logs/app.log | jq 'select(.error)'
   ```

4. Enable core dumps for deep analysis:
   ```bash
   ulimit -c unlimited
   npm start
   ```

---

### Testing & Development Issues

**Problem: Tests failing with async timeouts**

**Solution:**
```bash
# Increase timeout in vitest.config.ts
testTimeout: 30000  // 30 seconds
```

**Problem: Hot-reload not working**

**Solution:**
```bash
# Make sure you're using watch mode
npm run dev  # Not npm start

# Check file watchers limit
echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

---

### Getting Help

1. **Check logs first:**
   ```bash
   tail -f logs/app.log | jq '.' | less
   ```

2. **Search decision traces:**
   ```bash
   grep "error" logs/traces/$(date +%Y-%m-%d).jsonl | jq '.'
   ```

3. **Enable debug logging:**
   ```env
   LOG_LEVEL=debug
   ```

4. **Check GitHub issues:** https://github.com/SCWPretorius/CONTROL/issues

## Contributing

### Development Setup

1. **Clone and install:**
   ```bash
   git clone https://github.com/SCWPretorius/CONTROL.git
   cd CONTROL
   npm install
   ```

2. **Create feature branch:**
   ```bash
   git checkout -b feature/my-feature
   ```

3. **Make changes and test:**
   ```bash
   npm run dev           # Start in dev mode
   npm run typecheck     # Check types
   npm test:watch       # Run tests with watch
   ```

4. **Commit with clear messages:**
   ```bash
   git commit -m "feat: Add new skill for X"
   ```

   **Commit format:** `type(scope): description`
   - Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`
   - Scopes: `skills`, `integrations`, `memory`, `llm`, etc.

5. **Push and open PR:**
   ```bash
   git push origin feature/my-feature
   # Open PR on GitHub
   ```

### Code Standards

**TypeScript:**
- Strict mode enabled
- No `any` types without justification
- Export interfaces and types
- Use meaningful variable names

**Testing:**
- Unit tests for utility functions
- Integration tests for features
- 70%+ coverage for critical paths
- Use `describe`/`it` structure

**Git:**
- One feature per PR
- Keep commits atomic and logical
- Write descriptive commit messages
- Link related issues

### Areas for Contribution

Good opportunities to contribute:

1. **New Integrations:**
   - Slack, email, SMS
   - Product integrations (Jira, Monday, etc)

2. **New Skills:**
   - Data analysis
   - File operations
   - Advanced scheduling

3. **Documentation:**
   - Better examples
   - Video tutorials
   - Architecture diagrams

4. **Features:**
   - Web UI for approvals
   - Multi-tenant support
   - Plugin marketplace

5. **Infrastructure:**
   - Docker/Kubernetes
   - CI/CD improvements
   - Performance optimizations

---

## License

ISC License - See [LICENSE](LICENSE) file for details

---

## Roadmap

### Near Term (Next 2 Months)
- [ ] Web UI for approval workflows
- [ ] Advanced scheduling with recurrence
- [ ] Email integration
- [ ] Slack integration
- [ ] Performance optimizations (caching, indexing)

### Mid Term (2-6 Months)
- [ ] Voice interface support (speech recognition)
- [ ] Plugin marketplace
- [ ] Multi-tenant isolation
- [ ] Database backend (vs file-based)
- [ ] Advanced analytics dashboard

### Long Term (6+ Months)
- [ ] Cloud deployment guides (AWS, GCP, Azure)
- [ ] Kubernetes operators
- [ ] Mobile app
- [ ] Federated learning for personalization
- [ ] Commercial hosting option

---

## Support & Community

- **Issues:** [GitHub Issues](https://github.com/SCWPretorius/CONTROL/issues)
- **Discussions:** [GitHub Discussions](https://github.com/SCWPretorius/CONTROL/discussions)
- **Email:** contact@example.com

---

**Built with ❤️ using TypeScript, Node.js, and LLMs**

[⬆ Back to top](#table-of-contents)