import test from 'node:test';
import assert from 'node:assert/strict';
import { verifyHelpLinkBoundaries } from './check-help-links.mjs';
const links = { help: 'https://example.test/help' };
test('requires JSON boundaries on every occurrence and all catalog URLs', () => {
  const valid = Buffer.from(JSON.stringify(links));
  assert.doesNotThrow(() => verifyHelpLinkBoundaries(valid, links));
  assert.throws(() => verifyHelpLinkBoundaries(Buffer.from(links.help + 'SharePoint'), links));
  assert.throws(() => verifyHelpLinkBoundaries(Buffer.concat([valid, Buffer.from(links.help)]), links));
  assert.throws(() => verifyHelpLinkBoundaries(Buffer.from('{}'), links));
});
