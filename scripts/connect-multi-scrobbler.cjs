// Run in Multi-Scrobbler's Node environment. Read the new client JSON on stdin.
// node connect-multi-scrobbler.cjs /config/config.json /config/verified-backup
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { isDeepStrictEqual } = require('node:util');

function addDestination(original, client, existingName = 'Koito') {
  if (!Array.isArray(original.clients) || !Array.isArray(original.sources)) throw new Error('Expected sources and clients arrays.');
  if (client.type !== 'koito' || !client.name || !client.id || client.enable !== true || !client.data?.token || !client.data?.username) throw new Error('The development client is incomplete.');
  const url = new URL(client.data.url);
  if (!['http:', 'https:'].includes(url.protocol)) throw new Error('Expected an HTTP destination.');
  if (!original.clients.some(item => item.name === existingName && item.enable !== false)) throw new Error('The original Koito destination is missing or disabled.');
  const next = structuredClone(original);
  const existing = next.clients.find(item => item.name === client.name || item.id === client.id || item.data?.url === client.data.url);
  if (existing && !isDeepStrictEqual(existing, client)) throw new Error('A different destination already uses this name, identifier, or address.');
  if (!existing) next.clients.push(structuredClone(client));
  for (const source of next.sources) {
    if (source.clients === undefined) continue; // Unrestricted sources already include all clients.
    if (!Array.isArray(source.clients)) throw new Error('Expected a source destination list.');
    if (source.clients.includes(existingName) && !source.clients.includes(client.name)) source.clients.push(client.name);
  }
  return next;
}

async function main() {
  if (process.argv.length !== 4) throw new Error('Provide the configuration file and verified backup folder.');
  const filename = fs.realpathSync(process.argv[2]);
  const backup = fs.realpathSync(process.argv[3]);
  const manifest = JSON.parse(fs.readFileSync(path.join(backup, 'manifest.json'), 'utf8'));
  if (!manifest.completed_utc || manifest.database_consistency !== 'SQLite online backup') throw new Error('A completed online backup is required.');
  const digest = data => crypto.createHash('sha256').update(data).digest('hex');
  const backupRoot = backup + path.sep;
  for (const entry of manifest.files) {
    const saved = path.resolve(backup, entry.path);
    if (!saved.startsWith(backupRoot)) throw new Error('Invalid backup file path.');
    const raw = fs.readFileSync(saved);
    if (raw.length !== entry.bytes || digest(raw) !== entry.sha256) throw new Error('A backup file failed verification.');
  }
  if (!manifest.files.some(item => item.path === 'ms.db') || !manifest.files.some(item => item.path === 'config.json')) throw new Error('The backup needs its database and configuration.');
  const client = JSON.parse(fs.readFileSync(0, 'utf8'));
  const raw = fs.readFileSync(filename);
  const current = JSON.parse(raw);
  const next = addDestination(current, client);
  const validation = await fetch(client.data.url.replace(/\/$/, '') + '/apis/listenbrainz/1/validate-token', {
    headers: { Authorization: 'Token ' + client.data.token }, signal: AbortSignal.timeout(15000),
  });
  if (!validation.ok) throw new Error('The development receiver could not validate its token.');
  const account = await validation.json();
  if (!account.valid || account.user_name !== client.data.username) throw new Error('The development receiver returned a different account.');
  if (isDeepStrictEqual(next, current)) {
    console.log('The development destination is already configured.');
    return;
  }
  const savedConfig = manifest.files.find(item => item.path === 'config.json');
  if (digest(raw) !== savedConfig.sha256) throw new Error('The current configuration changed after its backup. Create a fresh backup.');
  const info = fs.statSync(filename);
  const temporary = filename + '.koito-' + crypto.randomUUID();
  try {
    const fd = fs.openSync(temporary, 'wx', info.mode & 0o777);
    try {
      fs.writeFileSync(fd, JSON.stringify(next, null, 2) + '\n');
      fs.fsyncSync(fd);
    } finally { fs.closeSync(fd); }
    if (process.platform !== 'win32') fs.chownSync(temporary, info.uid, info.gid);
    if (digest(fs.readFileSync(filename)) !== digest(raw)) throw new Error('Configuration changed during preparation. It was left unchanged.');
    fs.renameSync(temporary, filename);
    if (process.platform !== 'win32') {
      const directory = fs.openSync(path.dirname(filename), 'r');
      try { fs.fsyncSync(directory); } finally { fs.closeSync(directory); }
    }
  } finally {
    if (fs.existsSync(temporary)) fs.unlinkSync(temporary);
  }
  const routed = next.sources.filter(source => source.clients?.includes(client.name)).map(source => source.name);
  console.log('Added development destination. Explicit source routes: ' + routed.join(', '));
  console.log('Existing clients and source settings are preserved. Restart Multi-Scrobbler to load the addition.');
}

module.exports = { addDestination };
if (require.main === module) main().catch(error => { console.error('Connection setup failed: ' + error.message); process.exitCode = 1; });
