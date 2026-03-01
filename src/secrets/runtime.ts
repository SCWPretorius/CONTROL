import { getSecret } from '../config/secrets.js';
import { logger } from '../logging/logger.js';

/**
 * Retrieve a secret, pass it to fn, then allow it to go out of scope.
 * Use this instead of calling getSecret() and storing the result in a variable,
 * to minimize the lifetime of decrypted secrets in the call stack.
 */
export async function useSecret<T>(
  name: string,
  fn: (value: string) => Promise<T> | T,
): Promise<T | undefined> {
  const value = getSecret(name);
  if (value === undefined) {
    logger.warn({ name }, '[SECRETS] useSecret: secret not found');
    return undefined;
  }
  try {
    return await fn(value);
  } finally {
    logger.debug({ name }, '[SECRETS] useSecret: access complete');
  }
}

/**
 * Retrieve a secret as a Buffer for crypto operations.
 * Zero the buffer after use: `buf.fill(0)`
 */
export function getSecretBuffer(name: string): Buffer | undefined {
  const value = getSecret(name);
  if (value === undefined) return undefined;
  return Buffer.from(value, 'utf8');
}
