const { test } = require('node:test');
const assert = require('node:assert/strict');
const { addDestination } = require('./connect-multi-scrobbler.cjs');

const client = { type: 'koito', name: 'Development', id: 'development', enable: true, data: { url: 'http://127.0.0.1:4110', token: 'test', username: 'test' } };
const original = {
  sources: [{ name: 'Spotify', clients: ['Koito', 'Last.fm'], data: { existing: true } }, { name: 'Last.fm only', clients: ['Last.fm'] }, { name: 'All clients' }],
  clients: [{ type: 'koito', name: 'Koito', enable: true, data: { token: 'original' } }, { type: 'lastfm', name: 'Last.fm' }],
  sourceDefaults: { unchanged: true },
};

test('add a separate destination and update explicit source routes without changing existing settings', () => {
  const before = structuredClone(original);
  const next = addDestination(original, client);
  assert.deepEqual(original, before);
  assert.deepEqual(next.clients, [...original.clients, client]);
  assert.deepEqual(next.sources[0], { ...original.sources[0], clients: ['Koito', 'Last.fm', 'Development'] });
  assert.deepEqual(next.sources.slice(1), original.sources.slice(1));
  assert.deepEqual(next.sourceDefaults, original.sourceDefaults);
  assert.deepEqual(addDestination(next, client), next);
});

test('reject conflicting clients and a missing original destination', () => {
  assert.throws(() => addDestination(original, { ...client, name: 'Koito' }), /different destination/);
  assert.throws(() => addDestination({ ...original, clients: [] }, client), /original Koito/);
  assert.throws(() => addDestination(original, { ...client, enable: false }), /incomplete/);
});
