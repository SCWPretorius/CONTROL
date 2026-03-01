export interface ChannelCapabilities {
  maxMessageLength: number;
  supportsMarkdown: boolean;
}

export interface Channel {
  readonly name: string;
  readonly capabilities: ChannelCapabilities;
  send(chatId: string, text: string): Promise<void>;
}

/**
 * Split text into parts no longer than maxLen.
 * Prefers splitting at newline boundaries.
 */
export function chunkMessage(text: string, maxLen: number): string[] {
  if (text.length <= maxLen) return [text];
  const parts: string[] = [];
  let remaining = text;
  while (remaining.length > 0) {
    if (remaining.length <= maxLen) {
      parts.push(remaining);
      break;
    }
    let cut = remaining.lastIndexOf('\n', maxLen);
    if (cut <= 0) cut = maxLen;
    parts.push(remaining.slice(0, cut));
    remaining = remaining.slice(cut).replace(/^\n/, '');
  }
  return parts;
}
