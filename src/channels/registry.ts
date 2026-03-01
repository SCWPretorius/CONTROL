import type { Channel } from './channel-interface.js';

class ChannelRegistry {
  private channels = new Map<string, Channel>();

  register(channel: Channel): void {
    this.channels.set(channel.name, channel);
  }

  get(name: string): Channel | undefined {
    return this.channels.get(name);
  }

  getAll(): Channel[] {
    return Array.from(this.channels.values());
  }

  has(name: string): boolean {
    return this.channels.has(name);
  }
}

export const channelRegistry = new ChannelRegistry();
