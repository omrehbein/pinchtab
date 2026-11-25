import test from 'node:test';
import assert from 'node:assert/strict';
import { validateVersion } from './release-version.mjs';
test('matches persisted stable version', () => {
  assert.equal(validateVersion('v0.15.3', '0.15.3'), '0.15.3');
  assert.equal(validateVersion('0.16.0', '0.16.0'), '0.16.0');
});
test('rejects mismatch and malformed versions', () => {
  for (const input of ['0.15.2', '0.15.2.b', '0.15.3-b', '01.15.3', '0.15.3; echo bad', '']) assert.throws(() => validateVersion(input, '0.15.3'));
});
