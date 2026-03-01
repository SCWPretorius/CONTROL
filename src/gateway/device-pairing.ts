import { randomBytes, createHmac } from 'crypto';
import { logger } from '../logging/logger.js';

interface Device {
  id: string;
  approved: boolean;
  registeredAt: Date;
  label?: string;
}

interface Challenge {
  deviceId: string;
  nonce: string;
  expiresAt: Date;
}

class DevicePairingService {
  private devices = new Map<string, Device>();
  private challenges = new Map<string, Challenge>();

  generateChallenge(deviceId: string): string {
    const nonce = randomBytes(32).toString('hex');
    this.challenges.set(deviceId, {
      deviceId,
      nonce,
      expiresAt: new Date(Date.now() + 5 * 60 * 1000),
    });
    logger.info({ deviceId }, '[PAIRING] Challenge generated');
    return nonce;
  }

  verifyChallenge(deviceId: string, response: string, secret: string): boolean {
    const challenge = this.challenges.get(deviceId);
    if (!challenge || challenge.expiresAt < new Date()) {
      this.challenges.delete(deviceId);
      return false;
    }
    const expected = createHmac('sha256', secret).update(challenge.nonce).digest('hex');
    const valid = response === expected;
    if (valid) this.challenges.delete(deviceId);
    return valid;
  }

  approveDevice(deviceId: string, label?: string): void {
    this.devices.set(deviceId, {
      id: deviceId,
      approved: true,
      registeredAt: new Date(),
      label,
    });
    logger.info({ deviceId, label }, '[PAIRING] Device approved');
  }

  isApproved(deviceId: string): boolean {
    return this.devices.get(deviceId)?.approved ?? false;
  }

  listDevices(): Device[] {
    return Array.from(this.devices.values());
  }
}

export const devicePairing = new DevicePairingService();
