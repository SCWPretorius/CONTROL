import { createHmac, timingSafeEqual } from 'crypto';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

const TOKEN_TTL_MS = 60 * 60 * 1000; // 1 hour

function getSigningKey(): string {
  return config.encryptionKey || 'default-media-signing-key';
}

/**
 * Generate an HMAC-signed, TTL-bounded token for a media path.
 * Format (base64url-encoded): `${mediaPath}:${expiry}:${hmac}`
 */
export function generateMediaToken(mediaPath: string): string {
  const expiry = Date.now() + TOKEN_TTL_MS;
  const payload = `${mediaPath}:${expiry}`;
  const sig = createHmac('sha256', getSigningKey()).update(payload).digest('hex');
  return Buffer.from(`${payload}:${sig}`).toString('base64url');
}

/**
 * Verify a media token. Returns `{ valid: true, mediaPath }` or `{ valid: false, reason }`.
 * The HMAC is verified with timing-safe comparison; expiry is checked before returning.
 */
export function verifyMediaToken(token: string): { valid: boolean; mediaPath?: string; reason?: string } {
  try {
    const decoded = Buffer.from(token, 'base64url').toString('utf8');

    // Split off the last colon-delimited segment (sig), then the second-to-last (expiry)
    const lastColon = decoded.lastIndexOf(':');
    if (lastColon === -1) return { valid: false, reason: 'Malformed token' };

    const sig = decoded.slice(lastColon + 1);
    const withoutSig = decoded.slice(0, lastColon);

    const expiryColon = withoutSig.lastIndexOf(':');
    if (expiryColon === -1) return { valid: false, reason: 'Malformed token' };

    const expiryStr = withoutSig.slice(expiryColon + 1);
    const mediaPath = withoutSig.slice(0, expiryColon);
    const expiry = parseInt(expiryStr, 10);

    // Verify HMAC before checking expiry (prevent timing oracle on expiry)
    const expectedSig = createHmac('sha256', getSigningKey()).update(withoutSig).digest('hex');
    const sigBuf = Buffer.from(sig.padEnd(expectedSig.length, '0'), 'hex');
    const expectedBuf = Buffer.from(expectedSig, 'hex');
    if (sigBuf.length !== expectedBuf.length || !timingSafeEqual(sigBuf, expectedBuf)) {
      return { valid: false, reason: 'Invalid signature' };
    }

    if (isNaN(expiry) || Date.now() > expiry) {
      return { valid: false, reason: 'Token expired' };
    }

    return { valid: true, mediaPath };
  } catch (err) {
    logger.debug({ err }, '[MEDIA] Token verification error');
    return { valid: false, reason: 'Token parse error' };
  }
}
