import { Command } from 'commander';

export function createProgram(): Command {
  const program = new Command();

  program
    .name('openclaw')
    .description('CONTROL gateway CLI')
    .version('0.1.0');

  const gateway = program.command('gateway');

  gateway
    .command('run')
    .description('Start the WebSocket gateway server')
    .option('-p, --port <number>', 'Port to listen on (overrides PORT env)')
    .option('--host <string>', 'Host to bind to (overrides HOST env)')
    .action(async (opts) => {
      if (opts.port) process.env['PORT'] = String(opts.port);
      if (opts.host) process.env['HOST'] = String(opts.host);
      const { runGateway } = await import('./commands/gateway.js');
      await runGateway();
    });

  program
    .command('status')
    .description('Show gateway status')
    .option('--url <url>', 'Gateway WebSocket URL', 'ws://localhost:3000')
    .action(async (opts) => {
      const { runStatus } = await import('./commands/status.js');
      await runStatus(String(opts.url));
    });

  program
    .command('agent')
    .description('Send a message to the agent')
    .requiredOption('-m, --message <text>', 'Message to send')
    .option('--url <url>', 'Gateway WebSocket URL', 'ws://localhost:3000')
    .action(async (opts) => {
      const { runAgent } = await import('./commands/agent.js');
      await runAgent(String(opts.message), String(opts.url));
    });

  return program;
}
