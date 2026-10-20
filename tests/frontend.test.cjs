const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

// Exercise the application's registered event handlers without a browser or
// third-party DOM library. Only the DOM operations used by the import form
// are modeled; no catalog or image files are written.
function app() {
  class Element {
    constructor() {
      this.value = ''; this.files = []; this.open = false;
      this.children = []; this.listeners = new Map(); this.classes = new Set();
      this.classList = {add: name => this.classes.add(name), remove: name => this.classes.delete(name), toggle: (name, enabled) => enabled ? this.classes.add(name) : this.classes.delete(name)};
    }
    addEventListener(name, handler) {
      if (!this.listeners.has(name)) this.listeners.set(name, []);
      this.listeners.get(name).push(handler);
    }
    dispatch(name, event = {}) { for (const handler of this.listeners.get(name) || []) handler(event); }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    setAttribute() {}
    showModal() { this.open = true; }
    close() { this.open = false; }
    focus() {}
  }
  const elements = new Map();
  const get = id => { if (!elements.has(id)) elements.set(id, new Element()); return elements.get(id); };
  const document = new Element();
  document.getElementById = get; document.createElement = () => new Element(); document.querySelectorAll = () => [];
  const context = vm.createContext({document, window: new Element(), localStorage: {getItem: () => null},
    fetch: () => new Promise(() => {}), Option: class {}, console, setTimeout, clearTimeout, URLSearchParams, Intl});
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../static/app.js'), 'utf8'), context);
  vm.runInContext('canEdit = true; state = {folders: []};', context);
  for (const [id, value] of Object.entries({albumName: 'Album', albumMode: 'paged', importPages: '2', importRows: '2', importColumns: '2', startPage: '1', startRow: '1', startColumn: '1', importOrder: 'name'})) get(id).value = value;
  const read = code => JSON.parse(vm.runInContext(`JSON.stringify(${code})`, context));
  const drop = (files, items = []) => {
    const event = {prevented: false, preventDefault() { this.prevented = true; }, dataTransfer: {types: ['Files'], files, items}};
    document.dispatch('drop', event); return event;
  };
  return {get, context, read, drop, document};
}
const photo = name => ({name, size: 100, lastModified: 1});

test('drops append photos, open import, and follow natural filename order', () => {
  const a = app();
  assert.equal(a.drop([photo('photo10.png'), photo('photo2.png')]).prevented, true);
  assert.equal(a.get('importDialog').open, true);
  a.drop([photo('photo1.png')]);
  assert.deepEqual(a.read('orderedFiles().map(file => file.name)'), ['photo1.png', 'photo2.png', 'photo10.png']);
  assert.equal(a.get('fileSelectionCount').textContent, '3 photos selected');
  a.get('importOrder').value = 'selection';
  assert.deepEqual(a.read('orderedFiles().map(file => file.name)'), ['photo10.png', 'photo2.png', 'photo1.png']);
});
test('file picker appends to dropped files and clear removes both', () => {
  const a = app(); a.drop([photo('first.png')]);
  a.get('importFiles').files = [photo('second.png')]; a.get('importFiles').dispatch('change');
  assert.equal(a.read('importPhotos.length'), 2);
  a.get('clearImportFiles').onclick();
  assert.equal(a.read('importPhotos.length'), 0);
  assert.equal(a.get('fileSelectionCount').textContent, 'No photos selected');
  assert.equal(a.get('reviewButton').disabled, true);
});
test('adding dropped photos invalidates earlier conflict decisions', () => {
  const a = app(); a.drop([photo('first.png')]);
  vm.runInContext("importPlan = {unresolved: 0}; importActions = {0: 'replace'};", a.context);
  a.drop([photo('second.png')]);
  assert.equal(a.read('importPlan'), null); assert.deepEqual(a.read('importActions'), {});
  assert.equal(a.get('importButton').disabled, true);
});
test('signed-out drops open sign-in and do not select files', () => {
  const a = app(); vm.runInContext('canEdit = false;', a.context);
  assert.equal(a.drop([photo('private.png')]).prevented, true);
  assert.equal(a.read('importPhotos.length'), 0); assert.equal(a.get('loginDialog').open, true);
});
test('drops cannot change an active upload', () => {
  const a = app(); a.drop([photo('first.png')]);
  vm.runInContext('importing = true;', a.context); a.drop([photo('second.png')]);
  assert.deepEqual(a.read('importPhotos.map(file => file.name)'), ['first.png']);
});
test('folder drops are rejected without losing an existing selection', () => {
  const a = app(); a.drop([photo('first.png')]);
  a.drop([photo('second.png')], [{webkitGetAsEntry: () => ({isDirectory: true})}]);
  assert.equal(a.read('importPhotos.length'), 1);
  assert.match(a.get('importError').textContent, /individual photo files/);
});
test('unsupported dropped files fail the same validation as picked files', () => {
  const a = app(); a.drop([photo('notes.txt')]);
  assert.equal(a.get('reviewButton').disabled, true);
  assert.match(a.get('importError').textContent, /unsupported image/);
});

test('public cards show a saved price and omit an absent price', () => {
  const a = app(); vm.runInContext('canEdit = false;', a.context);
  const item = {path:'Box/photo.png', folder:'Box', filename:'photo.png', page:null, row:null, col:null, description:'A stamp', price:'25.00 SEK'};
  vm.runInContext(`var pricedCard = card(${JSON.stringify(item)});`, a.context);
  const children = a.read('pricedCard.children[1].children.map(node => ({text:node.textContent, className:node.className}))');
  assert.ok(children.some(child => child.className === 'price' && child.text === '25.00 SEK'));
  vm.runInContext(`var unpricedCard = card(${JSON.stringify({...item, price:''})});`, a.context);
  assert.equal(a.read("unpricedCard.children[1].children.some(node => node.className === 'price')"), false);
});
test('price edits participate in unsaved-change protection', () => {
  const a = app();
  vm.runInContext("selected = {description: 'Note', price: '10 SEK'};", a.context);
  a.get('description').value = 'Note'; a.get('price').value = '10 SEK';
  assert.equal(a.read('dirty()'), false);
  a.get('price').value = '20 SEK'; assert.equal(a.read('dirty()'), true);
  vm.runInContext('canEdit = false;', a.context); assert.equal(a.read('dirty()'), false);
});

test('typing a search resets album, physical page, and result page to all', () => {
  const a = app();
  vm.runInContext("folder = 'Album'; sheet = '2'; page = 4;", a.context);
  a.get('folders').value = 'Album'; a.get('sheets').value = '2';
  a.get('search').value = 'stamp'; a.get('search').oninput();
  assert.equal(a.read('folder'), ''); assert.equal(a.read('sheet'), '');
  assert.equal(a.get('folders').value, ''); assert.equal(a.get('sheets').value, '');
  // Trigger the debounced action directly to avoid real timer-dependent assertions.
  vm.runInContext('clearTimeout(timer); filtersChanged();', a.context);
  assert.equal(a.read('page'), 1);
});
test('CSV album selector defaults to all albums independently of current filters', () => {
  const a = app();
  vm.runInContext("state = {folders:[{path:'Album', count:65}, {path:'Box', count:2}]}; folder='Box'; sheet='none';", a.context);
  a.get('exportOpen').onclick();
  assert.deepEqual(a.read('[...exportSelection]'), ['Album', 'Box']);
  assert.equal(a.get('exportAll').checked, true);
  assert.equal(a.get('exportSummary').textContent, '2 albums selected · 67 cards');
  a.get('exportAll').checked = false; a.get('exportAll').onchange();
  assert.equal(a.get('exportDownload').disabled, true);
  const boxLabel = a.get('exportAlbums').children[1];
  boxLabel.children[0].checked = true; boxLabel.children[0].onchange();
  assert.deepEqual(a.read('[...exportSelection]'), ['Box']);
});
