import { randomBytes, createHmac } from 'crypto';
import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'fs';
import { resolve } from 'path';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

const DEVICES_FILE = resolve(config.dataDir, 'devices.json');

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

  constructor() {
    this.loadFromDisk();
  }

  private loadFromDisk(): void {
    try {
      if (existsSync(DEVICES_FILE)) {
        const raw = readFileSync(DEVICES_FILE, 'utf8');
        const entries = JSON.parse(raw) as Array<[string, Device]>;
        for (const [id, device] of entries) {
          this.devices.set(id, { ...device, registeredAt: new Date(device.registeredAt) });
        }
        logger.debug({ count: this.devices.size }, '[PAIRING] Loaded devices from disk');
      }
    } catch (err) {
      logger.warn({ err }, '[PAIRING] Failed to load devices from disk — starting fresh');
    }
  }

  private saveToDisk(): void {
    try {
      mkdirSync(config.dataDir, { recursive: true });
      writeFileSync(DEVICES_FILE, JSON.stringify(Array.from(this.devices.entries())), 'utf8');
    } catch (err) {
      logger.error({ err }, '[PAIRING] Failed to save devices to disk');
    }
  }

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
    this.saveToDisk();
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
