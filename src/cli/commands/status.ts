import { WebSocket } from 'ws';

export async function runStatus(url: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(url);
    const timeout = setTimeout(() => {
      ws.close();
      reject(new Error(`Could not connect to ${url}`));
    }, 5000);

    ws.once('open', () => {
      ws.send(JSON.stringify({ id: '1', method: 'status', params: {} }));
    });

    ws.on('message', (raw) => {
      try {
        const msg = JSON.parse(raw.toString());
        if (msg.event) return; // skip server-push events
        clearTimeout(timeout);
        console.log(JSON.stringify(msg.result, null, 2));
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
