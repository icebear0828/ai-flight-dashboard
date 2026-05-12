import assert from 'node:assert/strict';
import test from 'node:test';

import { buildReleasePlan, formatReleaseNotes } from './release_notes.mjs';

test('skips documentation-only changes', () => {
  const plan = buildReleasePlan({
    changedFiles: ['README.md', 'docs/RELEASE.md', '.github/pull_request_template.md'],
    prs: [
      {
        number: 41,
        title: 'docs: refresh README for current dashboard',
        body: '',
        labels: [],
        url: 'https://github.com/example/repo/pull/41',
      },
    ],
  });

  assert.equal(plan.shouldRelease, false);
  assert.equal(plan.skipReason, 'non-runtime changes');
});

test('fails when a release-worthy pull request omits bilingual notes', () => {
  assert.throws(
    () =>
      buildReleasePlan({
        changedFiles: ['internal/calculator/calculator.go'],
        prs: [
          {
            number: 50,
            title: 'fix: recalculate usage costs after pricing updates',
            body: 'No release notes here.',
            labels: [],
            url: 'https://github.com/example/repo/pull/50',
          },
        ],
      }),
    /missing release notes zh-CN, release notes en-US for #50/,
  );
});

test('builds patch release notes from bilingual pull request sections', () => {
  const plan = buildReleasePlan({
    changedFiles: ['internal/calculator/calculator.go'],
    prs: [
      {
        number: 50,
        title: 'fix: recalculate usage costs after pricing updates',
        body: [
          '## Release notes zh-CN',
          '- 修复价格更新后历史用量成本不会重新计算的问题。',
          '',
          '## Release notes en-US',
          '- Fixed historical usage costs not being recalculated after pricing updates.',
        ].join('\n'),
        labels: [],
        url: 'https://github.com/example/repo/pull/50',
      },
    ],
  });

  assert.equal(plan.shouldRelease, true);
  assert.equal(plan.bump, 'patch');

  const notes = formatReleaseNotes(plan, 'v1.2.26');
  assert.match(notes, /^# v1\.2\.26/);
  assert.match(notes, /## 中文/);
  assert.match(notes, /### 修复/);
  assert.match(notes, /修复价格更新后历史用量成本不会重新计算的问题。 #50/);
  assert.match(notes, /## English/);
  assert.match(notes, /### Fixed/);
  assert.match(notes, /Fixed historical usage costs not being recalculated after pricing updates\. #50/);
});

test('uses feature pull requests for minor releases', () => {
  const plan = buildReleasePlan({
    changedFiles: ['frontend/src/App.tsx'],
    prs: [
      {
        number: 51,
        title: 'feat: add release automation status panel',
        body: [
          '## Release notes zh-CN',
          '- 新增发布自动化状态面板。',
          '',
          '## Release notes en-US',
          '- Added a release automation status panel.',
        ].join('\n'),
        labels: [],
        url: 'https://github.com/example/repo/pull/51',
      },
    ],
  });

  assert.equal(plan.shouldRelease, true);
  assert.equal(plan.bump, 'minor');
});

test('allows an explicit skip label on product changes', () => {
  const plan = buildReleasePlan({
    changedFiles: ['internal/dashboard/stats.go'],
    prs: [
      {
        number: 52,
        title: 'test: cover dashboard stats cache',
        body: '',
        labels: ['release:skip'],
        url: 'https://github.com/example/repo/pull/52',
      },
    ],
  });

  assert.equal(plan.shouldRelease, false);
  assert.equal(plan.skipReason, 'all product changes are marked release:skip');
});

test('fails product changes when no runtime pull request is associated', () => {
  assert.throws(
    () =>
      buildReleasePlan({
        changedFiles: ['internal/dashboard/stats.go'],
        prs: [
          {
            number: 53,
            title: 'docs: refresh release docs',
            body: '',
            labels: [],
            files: ['docs/RELEASE.md'],
            url: 'https://github.com/example/repo/pull/53',
          },
        ],
      }),
    /Product changes require at least one release-worthy pull request with runtime files/,
  );
});
