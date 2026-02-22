import {
  readFileSync,
  writeFileSync,
  existsSync,
  mkdirSync,
} from 'fs';
import { join } from 'path';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

const EMBEDDINGS_DIR = join(config.memoryDir, 'embeddings');
const VECTOR_INDEX_FILE = join(EMBEDDINGS_DIR, 'vectors.json');

interface VectorEntry {
  id: string;
  label: string;
  vector: number[];
  metadata: Record<string, unknown>;
}

interface VectorIndex {
  entries: VectorEntry[];
}

function ensureDir(): void {
  mkdirSync(EMBEDDINGS_DIR, { recursive: true });
}

function loadVectorIndex(): VectorIndex {
  ensureDir();
  if (!existsSync(VECTOR_INDEX_FILE)) return { entries: [] };
  try {
    return JSON.parse(readFileSync(VECTOR_INDEX_FILE, 'utf-8')) as VectorIndex;
  } catch {
    return { entries: [] };
  }
}

function saveVectorIndex(index: VectorIndex): void {
  ensureDir();
  writeFileSync(VECTOR_INDEX_FILE, JSON.stringify(index), 'utf-8');
}

function cosineSimilarity(a: number[], b: number[]): number {
  if (a.length !== b.length || a.length === 0) return 0;
  let dot = 0;
  let normA = 0;
  let normB = 0;
  for (let i = 0; i < a.length; i++) {
    dot += (a[i] ?? 0) * (b[i] ?? 0);
    normA += (a[i] ?? 0) ** 2;
    normB += (b[i] ?? 0) ** 2;
  }
  const denom = Math.sqrt(normA) * Math.sqrt(normB);
  return denom === 0 ? 0 : dot / denom;
}

export function upsertVector(id: string, label: string, vector: number[], metadata: Record<string, unknown>): void {
  const index = loadVectorIndex();
  const existing = index.entries.findIndex(e => e.id === id);
  if (existing >= 0) {
    index.entries[existing] = { id, label, vector, metadata };
  } else {
    index.entries.push({ id, label, vector, metadata });
  }
  saveVectorIndex(index);
}

export function searchVectors(queryVector: number[], topK = 5): Array<{ id: string; label: string; score: number; metadata: Record<string, unknown> }> {
  const index = loadVectorIndex();
  const scored = index.entries.map(e => ({
    id: e.id,
    label: e.label,
    score: cosineSimilarity(queryVector, e.vector),
    metadata: e.metadata,
  }));
  return scored
    .sort((a, b) => b.score - a.score)
    .slice(0, topK)
    .filter(e => e.score >= 0.3);
}

export function deleteVector(id: string): void {
  const index = loadVectorIndex();
  index.entries = index.entries.filter(e => e.id !== id);
  saveVectorIndex(index);
}

export function pruneWeakVectors(_minScore = 0.3): number {
  logger.debug('[VECTOR-STORE] Prune called (no-op without query vectors)');
  return 0;
}

export function getVectorCount(): number {
  return loadVectorIndex().entries.length;
}
