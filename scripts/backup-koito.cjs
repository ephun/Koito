// Run with Koito's config mounted read-only in a Node environment with node:sqlite.
// node backup-koito.cjs /koito /backups/new-snapshot
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { DatabaseSync, backup } = require('node:sqlite');

async function main() {
  if (process.argv.length !== 4) throw new Error('Provide the Koito config folder and a new backup folder.');
  const source = fs.realpathSync(process.argv[2]);
  const destination = path.resolve(process.argv[3]);
  if (destination === source || destination.startsWith(source + path.sep)) throw new Error('Choose a backup folder outside Koito config.');
  fs.mkdirSync(destination, { mode: 0o700 });
  const original = new DatabaseSync(path.join(source, 'koito.db'), { readOnly: true });
  try { await backup(original, path.join(destination, 'koito.db')); }
  finally { original.close(); }
  const saved = new DatabaseSync(path.join(destination, 'koito.db'), { readOnly: true });
  const tables = {};
  try {
    const checks = saved.prepare('PRAGMA quick_check').all();
    if (checks.length !== 1 || checks[0].quick_check !== 'ok') throw new Error('Database integrity check failed.');
    for (const { name } of saved.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").all()) {
      tables[name] = saved.prepare('SELECT COUNT(*) AS count FROM "' + name.replaceAll('"', '""') + '"').get().count;
    }
  } finally { saved.close(); }
  if (!Object.hasOwn(tables, 'listens')) throw new Error('The snapshot does not contain Koito listening history.');
  const files = [];
  for (const entry of fs.readdirSync(source, { withFileTypes: true })) {
    if (entry.isFile() && (entry.name === 'docker-compose.yml' || entry.name === 'docker-compose.yaml' || entry.name === '.env' || entry.name === 'config.json')) {
      fs.copyFileSync(path.join(source, entry.name), path.join(destination, entry.name), fs.constants.COPYFILE_EXCL);
    }
  }
  for (const name of fs.readdirSync(destination)) {
    const filename = path.join(destination, name);
    fs.chmodSync(filename, 0o600);
    const hash = crypto.createHash('sha256');
    for await (const block of fs.createReadStream(filename)) hash.update(block);
    files.push({ path: name, bytes: fs.statSync(filename).size, sha256: hash.digest('hex') });
  }
  const manifest = { completed_utc: new Date().toISOString(), database_consistency: 'SQLite online backup', other_files_consistency: 'Copied while the service remains running', database_tables: tables, files };
  fs.writeFileSync(path.join(destination, 'manifest.json'), JSON.stringify(manifest, null, 2), { flag: 'wx', mode: 0o600 });
  console.log('Koito backup verified. Files: ' + files.length + '. Listens: ' + (tables.listens ?? 'table absent'));
}

main().catch(error => { console.error('Koito backup failed: ' + error.message); process.exitCode = 1; });
