import { WebSocket } from 'ws';

export async function runAgent(message: string, url: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(url);
    const timeout = setTimeout(() => {
      ws.close();
      reject(new Error(`Could not connect to ${url}`));
    }, 10000);

    ws.once('open', () => {
      // 'chat' method is added in Phase 2. This sends the request and handles a "not found" response gracefully.
      ws.send(JSON.stringify({ id: '1', method: 'chat', params: { message } }));
    });

    ws.on('message', (raw) => {
      try {
        const msg = JSON.parse(raw.toString());
        if (msg.event) return; // skip server-push events
        clearTimeout(timeout);
        if (msg.error) {
          if (msg.error.code === -32601) {
            console.error('Agent chat is not yet available (requires Phase 2). Use `openclaw status` to verify the gateway is running.');
          } else {
            console.error('Error:', msg.error.message);
          }
        } else {
          console.log(typeof msg.result === 'string' ? msg.result : JSON.stringify(msg.result, null, 2));
        }
        ws.close();
        resolve();
      } catch {
        // ignore parse errors
      }
    });

    ws.on('error', (err) => {
      clearTimeout(timeout);
      reject(err);
    });
  });
}
