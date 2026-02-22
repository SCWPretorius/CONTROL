import { Router, Request, Response } from 'express';
import rateLimit from 'express-rate-limit';
import {
  readFileSync,
  writeFileSync,
  readdirSync,
  existsSync,
  mkdirSync,
} from 'fs';
import { join } from 'path';
import { config } from '../config/config.js';
import { ApprovalRequest } from '../types/index.js';
import { logger } from '../logging/logger.js';

export const approvalRouter = Router();

const historyLimiter = rateLimit({
  windowMs: 60_000,
  max: 30,
  standardHeaders: true,
  legacyHeaders: false,
});

function ensureDir(): void {
  mkdirSync(config.approval.auditLogPath, { recursive: true });
}

function loadApproval(id: string): ApprovalRequest | null {
  const filePath = join(config.approval.auditLogPath, `${id}.json`);
  if (!existsSync(filePath)) return null;
  try {
    return JSON.parse(readFileSync(filePath, 'utf-8')) as ApprovalRequest;
  } catch {
    return null;
  }
}

function saveApproval(approval: ApprovalRequest): void {
  ensureDir();
  writeFileSync(
    join(config.approval.auditLogPath, `${approval.id}.json`),
    JSON.stringify(approval, null, 2),
    'utf-8'
  );
}

approvalRouter.get('/:approvalId/code', (req: Request, res: Response) => {
  const approvalId = req.params['approvalId'];
  if (!approvalId || typeof approvalId !== 'string') {
    res.status(400).json({ error: 'Missing approvalId' });
    return;
  }
  const approval = loadApproval(approvalId);
  if (!approval) {
    res.status(404).json({ error: 'Approval not found' });
    return;
  }
  res.json({ code: approval.generatedCodePreview });
});

approvalRouter.post('/:approvalId/decide', (req: Request, res: Response) => {
  const approvalId = req.params['approvalId'];
  if (!approvalId || typeof approvalId !== 'string') {
    res.status(400).json({ error: 'Missing approvalId' });
    return;
  }
  const approval = loadApproval(approvalId);
  if (!approval) {
    res.status(404).json({ error: 'Approval not found' });
    return;
  }

  if (approval.status !== 'pending') {
    res.status(400).json({ error: 'Approval already decided' });
    return;
  }

  const { action, userId } = req.body as { action: 'approve' | 'reject'; userId?: string };
  if (!['approve', 'reject'].includes(action)) {
    res.status(400).json({ error: 'Invalid action' });
    return;
  }

  approval.status = action === 'approve' ? 'approved' : 'rejected';
  approval.decidedAt = new Date().toISOString();
  approval.decidedBy = userId ?? 'unknown';
  saveApproval(approval);

  logger.info({ approvalId: approval.id, action }, `[APPROVAL-${action.toUpperCase()}]`);
  res.json({ success: true, status: approval.status });
});

approvalRouter.get('/history', historyLimiter, (req: Request, res: Response) => {
  ensureDir();
  const limit = parseInt((req.query['limit'] as string) ?? '20', 10);
  try {
    const files = readdirSync(config.approval.auditLogPath)
      .filter(f => f.endsWith('.json'))
      .slice(-limit);
    const approvals = files
      .map(f => {
        try {
          return JSON.parse(readFileSync(join(config.approval.auditLogPath, f), 'utf-8')) as ApprovalRequest;
        } catch {
          return null;
        }
      })
      .filter(Boolean);
    res.json({ approvals, total: approvals.length });
  } catch {
    res.json({ approvals: [], total: 0 });
  }
});
