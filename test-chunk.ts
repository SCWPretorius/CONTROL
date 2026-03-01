import { chunkMessage } from './src/channels/channel-interface.js';

function test(name: string, input: string, maxLen: number, expectedParts: number) {
  const parts = chunkMessage(input, maxLen);
  console.log(`Test '${name}':`, parts.length === expectedParts ? 'PASS' : `FAIL (got ${parts.length}, expected ${expectedParts})`);
  if (parts.length !== expectedParts) console.log(parts);
}

test('simple', 'hello world', 5, 3); // 'hello', ' worl', 'd' -> wait, space?
test('newline', 'hello\nworld', 10, 2);
test('newline at start', '\nhello', 5, 2);
test('long line', 'abcdefghij', 5, 2);
