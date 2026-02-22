import { createCipheriv, createDecipheriv, randomBytes, createHash } from 'crypto';
import { readFileSync, writeFileSync, existsSync, mkdirSync, appendFileSync } from 'fs';
import { resolve } from 'path';
import { config } from './config.js';
import { logger } from '../logging/logger.js';

const ALGO = 'aes-256-gcm';
const SECRETS_FILE = resolve(config.dataDir, 'secrets.enc');
const AUDIT_LOG = resolve(config.logsDir, 'secret-access.jsonl');

function getKey(): Buffer {
  const key = config.encryptionKey;
  if (!key || key.length < 32) {
    throw new Error('ENCRYPTION_KEY must be at least 32 characters');
  }
  return Buffer.from(key.slice(0, 32));
}

function encrypt(plaintext: string): string {
  const iv = randomBytes(12);
  const key = getKey();
  const cipher = createCipheriv(ALGO, key, iv);
  const encrypted = Buffer.concat([cipher.update(plaintext, 'utf8'), cipher.final()]);
  const authTag = cipher.getAuthTag();
  return Buffer.concat([iv, authTag, encrypted]).toString('base64');
}

function decrypt(ciphertext: string): string {
  const buf = Buffer.from(ciphertext, 'base64');
  const iv = buf.subarray(0, 12);
  const authTag = buf.subarray(12, 28);
  const encrypted = buf.subarray(28);
  const key = getKey();
  const decipher = createDecipheriv(ALGO, key, iv);
  decipher.setAuthTag(authTag);
  return Buffer.concat([decipher.update(encrypted), decipher.final()]).toString('utf8');
}

function auditLog(action: string, secretName: string): void {
  try {
    mkdirSync(config.logsDir, { recursive: true });
    const entry = JSON.stringify({
      timestamp: new Date().toISOString(),
      action,
      secretName,
    });
    appendFileSync(AUDIT_LOG, entry + '\n');
  } catch {
    // Non-fatal
  }
}

export function saveSecrets(secrets: Record<string, string>): void {
  mkdirSync(config.dataDir, { recursive: true });
  const encrypted = encrypt(JSON.stringify(secrets));
  writeFileSync(SECRETS_FILE, encrypted, 'utf8');
  logger.info('[SECRETS] Saved encrypted secrets');
}

export function loadSecrets(): Record<string, string> {
  if (!existsSync(SECRETS_FILE)) return {};
  try {
    const encrypted = readFileSync(SECRETS_FILE, 'utf8');
    const decrypted = decrypt(encrypted);
    return JSON.parse(decrypted) as Record<string, string>;
  } catch (err) {
    logger.error({ err }, '[SECRETS] Failed to decrypt secrets file');
    return {};
  }
}

export function getSecret(name: string): string | undefined {
  auditLog('read', name);
  const envVal = process.env[name];
  if (envVal) return envVal;
  const secrets = loadSecrets();
  return secrets[name];
}

export function setSecret(name: string, value: string): void {
  auditLog('write', name);
  const secrets = loadSecrets();
  secrets[name] = value;
  saveSecrets(secrets);
}

export function rotateSecret(name: string, newValue: string): void {
  auditLog('rotate', name);
  setSecret(name, newValue);
  logger.info({ secretName: name }, '[SECRETS] Secret rotated');
}

export const ROTATION_POLICIES: Record<string, number> = {
  TELEGRAM: 90,
  GOOGLE: 180,
  CUSTOM: 30,
};

export function checkRotationDue(secretName: string, lastRotated: Date): boolean {
  const prefix = Object.keys(ROTATION_POLICIES).find(k => secretName.toUpperCase().startsWith(k));
  if (!prefix) return false;
  const days = ROTATION_POLICIES[prefix]!;
  const dueDate = new Date(lastRotated.getTime() + days * 24 * 60 * 60 * 1000);
  return new Date() >= dueDate;
}

export function hashIdempotencyKey(skillName: string, params: Record<string, unknown>, userId: string): string {
  const data = JSON.stringify({ skillName, params, userId });
  return createHash('sha256').update(data).digest('hex');
}
