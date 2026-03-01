import { getLLMStatus } from '../llm/llmDecider.js';
import { config } from '../config/config.js';

export interface ModelInfo {
  id: string;
  provider: 'lmstudio';
  status: ReturnType<typeof getLLMStatus>;
}

export interface ModelAllowlist {
  allowedProviders?: string[];
  allowedModels?: string[];
}

export function resolveModel(preference?: string, allowlist?: ModelAllowlist): ModelInfo {
  const primaryModel = config.lmstudio.primaryModel;

  if (allowlist?.allowedProviders && !allowlist.allowedProviders.includes('lmstudio')) {
    throw new Error('Provider lmstudio is not in the allowed list');
  }

  const modelId = preference ?? primaryModel;
  if (allowlist?.allowedModels && !allowlist.allowedModels.includes(modelId)) {
    throw new Error(`Model ${modelId} is not in the allowed list`);
  }

  return { id: modelId, provider: 'lmstudio', status: getLLMStatus() };
}

export function isModelAvailable(): boolean {
  return getLLMStatus() !== 'unavailable';
}
