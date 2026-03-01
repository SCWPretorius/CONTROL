import { WebSocket } from 'ws';

export async function runAgent(message: string, url: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(url);
    const timeout = setTimeout(() => {
      ws.close();
      reject(new Error(`Could not connect to ${url}`));
    }, 60000);

    ws.once('open', () => {
      ws.send(JSON.stringify({ id: '1', method: 'chat', params: { message } }));
    });

    ws.on('message', (raw) => {
      try {
        const msg = JSON.parse(raw.toString());

        // Print streaming agent blocks as they arrive
        if (msg.event === 'agent:block') {
          const block = msg.data;
          if (block.type === 'text') {
            process.stdout.write(block.content + '\n');
          } else if (block.type === 'tool-call') {
            process.stdout.write(`[tool] ${block.name}(${JSON.stringify(block.params)})\n`);
          } else if (block.type === 'error') {
            process.stderr.write(`[error] ${block.message}\n`);
          } else if (block.type === 'done') {
            process.stdout.write(`[done] ${block.stepsExecuted} step(s) via ${block.model}\n`);
          }
          return;
        }

        // Final RPC response
        clearTimeout(timeout);
        if (msg.error) {
          console.error('Error:', msg.error.message);
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

