import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

test('Test workflow reruns PR release gate when release metadata changes', () => {
  const workflow = fs.readFileSync('.github/workflows/test.yml', 'utf8');

  assert.match(workflow, /pull_request:\n\s+branches: \[main\]\n\s+types: \[([^\]]+)\]/);

  const [, rawTypes] = workflow.match(/pull_request:\n\s+branches: \[main\]\n\s+types: \[([^\]]+)\]/) ?? [];
  const types = rawTypes.split(',').map((type) => type.trim());

  for (const requiredType of ['opened', 'synchronize', 'reopened', 'edited', 'labeled', 'unlabeled']) {
    assert.ok(types.includes(requiredType), `missing pull_request type: ${requiredType}`);
  }
});
