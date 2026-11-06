// Run inside Multi-Scrobbler's Node environment:
// node backup-multi-scrobbler.cjs /path/to/appdata /path/to/new-backup
// The SQLite online backup includes committed history and persisted queues.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { DatabaseSync, backup } = require('node:sqlite');

async function main() {
  if (process.argv.length !== 4) throw new Error('Provide the appdata folder and a new backup folder.');
  const source = fs.realpathSync(process.argv[2]);
  const destination = path.resolve(process.argv[3]);
  const logs = path.join(source, 'logs');
  if (destination === source || destination === logs || destination.startsWith(logs + path.sep)) {
    throw new Error('Choose a backup folder outside the source logs folder.');
  }
  for (const name of ['config.json', 'ms.db']) {
    if (!fs.statSync(path.join(source, name)).isFile()) throw new Error('The appdata folder must contain config.json and ms.db.');
  }
  fs.mkdirSync(destination, { mode: 0o700 });
  const manifest = { started_utc: new Date().toISOString(), files: [], database_tables: {} };
  const original = new DatabaseSync(path.join(source, 'ms.db'), { readOnly: true });
  try {
    await backup(original, path.join(destination, 'ms.db'));
  } finally {
    original.close();
  }
  const saved = new DatabaseSync(path.join(destination, 'ms.db'), { readOnly: true });
  try {
    const checks = saved.prepare('PRAGMA quick_check').all();
    if (checks.length !== 1 || checks[0].quick_check !== 'ok') throw new Error('The database backup failed its integrity check.');
    for (const { name } of saved.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").all()) {
      const identifier = '"' + name.replaceAll('"', '""') + '"';
      manifest.database_tables[name] = saved.prepare('SELECT COUNT(*) AS count FROM ' + identifier).get().count;
    }
  } finally {
    saved.close();
  }
  for (const entry of fs.readdirSync(source, { withFileTypes: true })) {
    if (entry.isFile() && (entry.name === 'config.json' || entry.name.startsWith('currentCreds-') && entry.name.endsWith('.json'))) {
      fs.copyFileSync(path.join(source, entry.name), path.join(destination, entry.name), fs.constants.COPYFILE_EXCL);
    }
  }
  if (fs.existsSync(logs)) fs.cpSync(logs, path.join(destination, 'logs'), { recursive: true, errorOnExist: true, force: false });
  async function inventory(directory) {
    for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
      const filename = path.join(directory, entry.name);
      if (entry.isDirectory()) await inventory(filename);
      else if (entry.isFile()) {
        fs.chmodSync(filename, 0o600);
        const digest = crypto.createHash('sha256');
        for await (const block of fs.createReadStream(filename)) digest.update(block);
        manifest.files.push({ path: path.relative(destination, filename), bytes: fs.statSync(filename).size, sha256: digest.digest('hex') });
      } else throw new Error('The backup contains an unsupported filesystem entry.');
    }
  }
  await inventory(destination);
  manifest.completed_utc = new Date().toISOString();
  manifest.database_consistency = 'SQLite online backup';
  manifest.other_files_consistency = 'Copied while the service remains running';
  fs.writeFileSync(path.join(destination, 'manifest.json'), JSON.stringify(manifest, null, 2), { flag: 'wx', mode: 0o600 });
  console.log(`Backup verified. ${manifest.files.length} files saved in ${destination}`);
}

main().catch(error => {
  console.error(`Backup failed: ${error.message}`);
  process.exitCode = 1;
});
