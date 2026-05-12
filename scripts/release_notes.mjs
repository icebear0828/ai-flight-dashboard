#!/usr/bin/env node

import fs from 'node:fs';
import { fileURLToPath } from 'node:url';

const bumpRank = new Map([
  ['patch', 0],
  ['minor', 1],
  ['major', 2],
]);

const categoryOrder = ['features', 'fixes', 'performance', 'changes'];

const categoryNames = {
  features: { zh: '新增', en: 'Added' },
  fixes: { zh: '修复', en: 'Fixed' },
  performance: { zh: '性能', en: 'Performance' },
  changes: { zh: '变更', en: 'Changed' },
};

function normalizeLabels(labels = []) {
  return labels.map((label) => {
    if (typeof label === 'string') {
      return label.toLowerCase();
    }
    return String(label?.name ?? '').toLowerCase();
  });
}

function normalizePullRequest(pr) {
  return {
    number: pr.number,
    title: String(pr.title ?? ''),
    body: String(pr.body ?? ''),
    labels: normalizeLabels(pr.labels),
    url: String(pr.url ?? pr.html_url ?? ''),
    files: Array.isArray(pr.files)
      ? pr.files.map((file) => (typeof file === 'string' ? file : String(file?.path ?? ''))).filter(Boolean)
      : [],
  };
}

function conventionalType(title) {
  const match = title.match(/^([a-z]+)(?:\([^)]+\))?!?:/i);
  return match ? match[1].toLowerCase() : '';
}

function hasLabel(pr, names) {
  return pr.labels.some((label) => names.includes(label));
}

function isSkipPr(pr) {
  return hasLabel(pr, ['release:skip', 'release:none']);
}

function isDocumentationTitle(title) {
  return conventionalType(title) === 'docs';
}

function isRuntimeFile(file) {
  if (!file) {
    return false;
  }

  if (
    file === '.gitignore' ||
    file === 'TODO.md' ||
    file === 'README.md' ||
    file === 'README.en.md' ||
    file === 'usage.md' ||
    file.endsWith('.md') ||
    file.startsWith('.github/') ||
    file.startsWith('docs/') ||
    file.startsWith('frontend/tests/') ||
    file.startsWith('test/') ||
    file.includes('/testdata/') ||
    file.endsWith('_test.go') ||
    file.endsWith('.test.mjs')
  ) {
    return false;
  }

  return (
    file === 'go.mod' ||
    file === 'go.sum' ||
    file === 'embed.go' ||
    file === 'wails.json' ||
    file === 'frontend/index.html' ||
    file === 'frontend/package.json' ||
    file === 'frontend/package-lock.json' ||
    file.startsWith('cmd/') ||
    file.startsWith('internal/') ||
    file.startsWith('frontend/src/') ||
    file.startsWith('frontend/public/')
  );
}

function isRuntimePr(pr) {
  if (pr.files.length > 0) {
    return pr.files.some(isRuntimeFile);
  }
  return !isDocumentationTitle(pr.title);
}

function bumpForPullRequest(pr) {
  if (hasLabel(pr, ['release:major'])) {
    return 'major';
  }
  if (hasLabel(pr, ['release:minor']) || conventionalType(pr.title) === 'feat') {
    return 'minor';
  }
  return 'patch';
}

function maxBump(current, next) {
  return bumpRank.get(next) > bumpRank.get(current) ? next : current;
}

function categoryForPullRequest(pr) {
  switch (conventionalType(pr.title)) {
    case 'feat':
      return 'features';
    case 'fix':
      return 'fixes';
    case 'perf':
      return 'performance';
    default:
      return 'changes';
  }
}

function sectionPattern(locale) {
  const escaped = locale.replace('-', '[-_]');
  return new RegExp(`^##\\s+Release notes ${escaped}\\s*$`, 'i');
}

function cleanNoteLine(line) {
  let text = line.trim();
  if (!text || text.startsWith('<!--') || text.endsWith('-->')) {
    return null;
  }

  text = text.replace(/^[-*]\s+/, '').replace(/^\d+\.\s+/, '').trim();
  const normalized = text.toLowerCase().replace(/[._-]+$/g, '').trim();
  if (['n/a', 'na', 'none', 'not applicable', 'no release notes'].includes(normalized)) {
    return null;
  }
  return text || null;
}

function extractReleaseNotes(body, locale) {
  const lines = body.split(/\r?\n/);
  const header = sectionPattern(locale);
  const start = lines.findIndex((line) => header.test(line.trim()));
  if (start === -1) {
    return [];
  }

  const notes = [];
  for (const line of lines.slice(start + 1)) {
    if (/^##\s+/.test(line.trim())) {
      break;
    }
    const note = cleanNoteLine(line);
    if (note) {
      notes.push(note);
    }
  }
  return notes;
}

export function buildReleasePlan({ changedFiles, prs }) {
  const runtimeChanged = changedFiles.some(isRuntimeFile);
  if (!runtimeChanged) {
    return {
      shouldRelease: false,
      skipReason: 'non-runtime changes',
      bump: '',
      entries: [],
    };
  }

  const normalizedPrs = prs.map(normalizePullRequest);
  if (normalizedPrs.length === 0) {
    throw new Error('Product changes require pull request metadata and bilingual release notes.');
  }

  const releasePrs = normalizedPrs.filter((pr) => !isSkipPr(pr) && isRuntimePr(pr));
  if (releasePrs.length === 0) {
    if (!normalizedPrs.some(isSkipPr)) {
      throw new Error('Product changes require at least one release-worthy pull request with runtime files.');
    }
    return {
      shouldRelease: false,
      skipReason: 'all product changes are marked release:skip',
      bump: '',
      entries: [],
    };
  }

  let bump = 'patch';
  const missingNotes = [];
  const entries = [];

  for (const pr of releasePrs) {
    bump = maxBump(bump, bumpForPullRequest(pr));

    const zh = extractReleaseNotes(pr.body, 'zh-CN');
    const en = extractReleaseNotes(pr.body, 'en-US');
    const missing = [];
    if (zh.length === 0) {
      missing.push('release notes zh-CN');
    }
    if (en.length === 0) {
      missing.push('release notes en-US');
    }
    if (missing.length > 0) {
      missingNotes.push(`missing ${missing.join(', ')} for #${pr.number}`);
    }

    entries.push({
      number: pr.number,
      title: pr.title,
      url: pr.url,
      category: categoryForPullRequest(pr),
      zh,
      en,
    });
  }

  if (missingNotes.length > 0) {
    throw new Error(`Release-worthy pull requests are ${missingNotes.join('; ')}.`);
  }

  return {
    shouldRelease: true,
    skipReason: '',
    bump,
    entries,
  };
}

function groupedEntries(entries, locale) {
  const groups = new Map(categoryOrder.map((category) => [category, []]));
  for (const entry of entries) {
    const notes = entry[locale];
    for (const note of notes) {
      groups.get(entry.category).push({ note, number: entry.number });
    }
  }
  return groups;
}

function writeLocaleSection(lines, title, locale, entries) {
  lines.push(`## ${title}`, '');
  const groups = groupedEntries(entries, locale);
  let wroteGroup = false;

  for (const category of categoryOrder) {
    const items = groups.get(category);
    if (items.length === 0) {
      continue;
    }
    wroteGroup = true;
    lines.push(`### ${categoryNames[category][locale]}`, '');
    for (const item of items) {
      lines.push(`- ${item.note} #${item.number}`);
    }
    lines.push('');
  }

  if (!wroteGroup) {
    lines.push('- No release notes.', '');
  }
}

export function formatReleaseNotes(plan, version) {
  if (!plan.shouldRelease) {
    throw new Error('Cannot format release notes for a skipped release.');
  }
  if (!/^v\d+\.\d+\.\d+$/.test(version)) {
    throw new Error(`Invalid release version "${version}". Expected vX.Y.Z.`);
  }

  const lines = [`# ${version}`, ''];
  writeLocaleSection(lines, '中文', 'zh', plan.entries);
  writeLocaleSection(lines, 'English', 'en', plan.entries);
  lines.push('## Assets', '');
  lines.push('- macOS Apple Silicon');
  lines.push('- macOS Intel');
  lines.push('- Windows');
  lines.push('- Linux');
  lines.push('');
  return lines.join('\n');
}

function parseArgs(argv) {
  const args = {};
  for (let index = 0; index < argv.length; index += 1) {
    const key = argv[index];
    if (!key.startsWith('--')) {
      throw new Error(`Unexpected argument: ${key}`);
    }
    const name = key.slice(2);
    const value = argv[index + 1];
    if (!value || value.startsWith('--')) {
      throw new Error(`Missing value for ${key}`);
    }
    args[name] = value;
    index += 1;
  }
  return args;
}

function readChangedFiles(file) {
  return fs
    .readFileSync(file, 'utf8')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
}

function readPullRequests(file) {
  const parsed = JSON.parse(fs.readFileSync(file, 'utf8'));
  if (Array.isArray(parsed)) {
    return parsed;
  }
  if (Array.isArray(parsed.items)) {
    return parsed.items;
  }
  throw new Error(`Expected ${file} to contain a pull request array.`);
}

function writeGithubOutput(file, plan) {
  if (!file) {
    return;
  }
  const output = [
    `should_release=${plan.shouldRelease ? 'true' : 'false'}`,
    `bump=${plan.bump}`,
    `skip_reason=${plan.skipReason}`,
  ];
  fs.appendFileSync(file, `${output.join('\n')}\n`);
}

function writePlan(file, plan) {
  if (!file) {
    return;
  }
  fs.writeFileSync(file, `${JSON.stringify(plan, null, 2)}\n`);
}

function runCli() {
  const args = parseArgs(process.argv.slice(2));
  if (!args['changed-files'] || !args.prs) {
    throw new Error(
      'Usage: node scripts/release_notes.mjs --changed-files <file> --prs <file> [--version vX.Y.Z] [--notes-out <file>] [--plan-out <file>] [--github-output <file>]',
    );
  }

  const changedFiles = readChangedFiles(args['changed-files']);
  const prs = readPullRequests(args.prs);
  const plan = buildReleasePlan({ changedFiles, prs });

  writeGithubOutput(args['github-output'], plan);
  writePlan(args['plan-out'], plan);

  if (args['notes-out']) {
    if (!args.version) {
      throw new Error('--version is required when --notes-out is provided.');
    }
    const notes = formatReleaseNotes(plan, args.version);
    fs.writeFileSync(args['notes-out'], notes);
  }

  if (plan.shouldRelease) {
    console.log(`Release required: ${plan.bump}`);
  } else {
    console.log(`Release skipped: ${plan.skipReason}`);
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    runCli();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}
