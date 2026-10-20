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
      this.dataset = {}; this.value = ''; this.files = []; this.open = false;
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

test('unnumbered file-picker imports select box before review and keep filenames', async () => {
  const a = app();
  a.get('importFiles').files = [photo('summer.jpg'), photo('portrait.png')];
  a.get('importFiles').dispatch('change');
  assert.equal(a.get('albumMode').value, 'box');
  assert.equal(a.get('dimensions').hidden, true);
  assert.equal(a.get('startPosition').hidden, true);
  assert.equal(a.read('importSettings().mode'), 'box');
  vm.runInContext('var reviewed; api=async(url,body)=>{reviewed=body;return {entries:body.files.map((name,index)=>({index,original:name,filename:name,page:null,row:null,col:null,conflicts:[]})),unresolved:0};};', a.context);
  await vm.runInContext('reviewImport()', a.context);
  assert.equal(a.read('reviewed.options.mode'), 'box');
  assert.deepEqual(a.read('importPlan.entries.map(entry=>entry.filename)'), ['portrait.png','summer.jpg']);
});

test('automatic type detection updates for dropped photos and respects manual choice', () => {
  const a = app();
  a.drop([photo('plain.jpg')]);
  assert.equal(a.get('albumMode').value, 'box');
  a.drop([photo('card-2-3-1.jpg')]);
  assert.equal(a.get('albumMode').value, 'paged');
  a.get('clearImportFiles').onclick();
  a.drop([photo('plain.jpg')]);
  assert.equal(a.get('albumMode').value, 'box');
  a.get('albumMode').value = 'paged'; a.get('albumMode').dispatch('change');
  a.drop([photo('another.jpg')]);
  assert.equal(a.get('albumMode').value, 'paged');
});

test('automatic type detection preserves an existing destination type', () => {
  const a = app();
  vm.runInContext('state.folders=[{path:"Album",album:{mode:"paged",pages:2,rows:2,columns:2}}];', a.context);
  a.drop([photo('plain.jpg')]);
  assert.equal(a.get('albumMode').value, 'paged');
  assert.equal(a.get('albumMode').disabled, true);
  a.get('albumName').value = 'New box'; a.get('albumName').dispatch('input');
  assert.equal(a.get('albumMode').value, 'box');
  assert.equal(a.get('albumMode').disabled, false);
});

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

test('multiple tags are trimmed and deduplicated without losing display case', () => {
  const a = app(); a.get('tagsInput').value = ' Rare, Japanese, rare, , Foil ';
  assert.deepEqual(a.read('selectedTags()'), ['Rare', 'Japanese', 'Foil']);
});
test('card names and multiple tags are visible on public cards', () => {
  const a = app(); vm.runInContext('canEdit = false;', a.context);
  const item = {path:'Cards/file.png', folder:'Cards', filename:'file.png', name:'Blue Eyes Dragon', tags:['Rare','Japanese'], page:null,row:null,col:null,description:'',price:''};
  vm.runInContext(`var namedCard = card(${JSON.stringify(item)});`, a.context);
  assert.equal(a.read('namedCard.children[1].children[1].textContent'), 'Blue Eyes Dragon');
  assert.deepEqual(a.read("namedCard.children[1].children.find(node => node.className === 'badges card-tags').children.map(node => node.textContent)"), ['Rare','Japanese']);
});

test('cards do not create image elements until previews are explicitly enabled', () => {
  const a = app();
  vm.runInContext("var deferred = thumbnail({path:'photo.png', filename:'photo.png'});", a.context);
  assert.equal(a.read('showPreviews'), false);
  assert.equal(a.read('deferred.children[0].textContent'), '▧ View image');
  assert.equal(a.read('browsingAlbums'), true);
});
test('album overview and navigation stay bounded for a large directory', () => {
  const a = app();
  // The album API provides at most 50 folders per page.
  vm.runInContext("state = {mode:'albums', albumCount:2000, folders:Array.from({length:50},(_,i)=>({path:'Album '+i,count:25})),items:[]}; renderNavigation(); renderLibrary();", a.context);
  assert.equal(a.get('albumNav').children.length, 41);
  assert.equal(a.get('library').children[0].children.length, 50);
});
test('export renders only 50 album choices even when selecting every album', () => {
  const a = app();
  vm.runInContext("state={folders:[]}; exportDirectory=Array.from({length:2000},(_,i)=>({path:'Album '+i,count:25})); exportSelection=new Set(exportDirectory.map(a=>a.path)); renderExportSelection();", a.context);
  assert.equal(a.get('exportAlbums').children.length, 50);
  assert.equal(a.get('exportSummary').textContent, '2000 albums selected · 50000 cards');
});

test('thumbnails default on for selected albums and searches below 100, with no album overview downloads', () => {
  const a=app();
  a.get('search').value='rare';
  assert.equal(a.read('automaticPreviews({mode:"images",total:99})'),true);
  assert.equal(a.read('automaticPreviews({mode:"images",total:100})'),false);
  vm.runInContext('folder="Album";',a.context);
  assert.equal(a.read('automaticPreviews({mode:"slots",total:50000})'),true);
  assert.equal(a.read('automaticPreviews({mode:"albums",total:50})'),false);
  vm.runInContext('state={mode:"albums",folders:[]};',a.context);
  a.get('showPreviews').checked=false; a.get('showPreviews').onchange();
  assert.equal(a.read('previewPreference'),false);
});
test('only admins can drag cards, and card drags are separate from file imports', () => {
  const a=app();
  vm.runInContext('var item={path:"Album/a.jpg",folder:"Album",filename:"a.jpg",page:1,row:1,col:1,description:""}; var dragCard=card(item);',a.context);
  const data=new Map();
  a.context.dragEvent={dataTransfer:{setData:(type,value)=>data.set(type,value)}};
  vm.runInContext('dragCard.dispatch("dragstart",dragEvent);',a.context);
  assert.equal(data.get('application/x-card-atlas'),'Album/a.jpg');
  assert.equal(a.read('movingCard.path'),'Album/a.jpg');
  assert.equal(a.get('moveTools').hidden,false);
  assert.equal(a.get('importDialog').open,false);
  a.get('cancelMove').onclick(); assert.equal(a.read('movingCard'),null);
  vm.runInContext('canEdit=false; var publicCard=card(item);',a.context);
  assert.equal(a.read('publicCard.draggable'),false);
});
test('only administrators request empty album pages and cells', () => {
  const a = app();
  vm.runInContext('browsingAlbums=false; folder="Album"; view="organized"; sheet="";', a.context);
  assert.equal(a.read('slotView()'), true);
  vm.runInContext('canEdit=false;', a.context);
  assert.equal(a.read('slotView()'), false);
  vm.runInContext('canEdit=true;', a.context);
  assert.equal(a.read('slotView()'), true);
});

test('boxes cannot open album layout and album tiles identify their type', () => {
  const a = app();
  vm.runInContext('folder="Box"; browsingAlbums=false; state={folders:[{path:"Box",count:1,album:{mode:"box"}}]}; applyAccess();', a.context);
  assert.equal(a.get('layoutOpen').hidden, true);
  a.get('layoutOpen').onclick();
  assert.equal(a.get('layoutDialog').open, false);
  vm.runInContext('state={mode:"albums",folders:[{path:"Box",count:1,album:{mode:"box"}},{path:"Album",count:1,album:{mode:"paged",pages:1,rows:1,columns:1}}]}; renderLibrary();', a.context);
  assert.equal(a.read('document.getElementById("library").children[0].children[0].children[1].textContent'), '📦 Box');
  assert.equal(a.read('document.getElementById("library").children[0].children[1].children[1].textContent'), '📖 Album');
});

test('admin selection persists across views and transfers only after confirmation', async () => {
  const a=app();
  vm.runInContext('browsingAlbums=false; state={items:[{path:"Source/a.jpg",folder:"Source",filename:"a.jpg",page:null,row:null,col:null,tags:[]}],folders:[]}; var calls=[]; api=async (url,data)=>{calls.push({url,data}); return {folders:[{path:"Source",album:{mode:"box"}},{path:"Destination",album:{mode:"paged"}}]};}; load=async()=>{};',a.context);
  a.get('selectImages').onclick();
  vm.runInContext('activateImage(state.items[0]);',a.context);
  assert.equal(a.read('selectedImages.size'),1);
  assert.equal(a.get('detailDialog').open,false);
  assert.equal(a.get('transferOpen').disabled,false);
  await a.get('transferOpen').onclick();
  assert.equal(a.get('transferDialog').open,true);
  assert.equal(a.read('calls.filter(call=>call.url==="/api/transfer").length'),0);
  a.get('transferDestination').value='Destination';
  a.get('transferDestination').onchange();
  assert.match(a.get('transferSummary').textContent,/Move 1 selected image to Destination/);
  a.get('transferCancel').onclick();
  assert.equal(a.read('calls.filter(call=>call.url==="/api/transfer").length'),0);
  await a.get('transferOpen').onclick();
  a.get('transferDestination').value='Destination';
  await a.get('transferForm').onsubmit({preventDefault(){}});
  assert.deepEqual(a.read('calls.filter(call=>call.url==="/api/transfer")[0].data'),{paths:['Source/a.jpg'],destination:'Destination'});
  assert.equal(a.read('selectedImages.size'),0);
  assert.equal(a.get('transferDialog').open,false);
});

test('signing out clears multi-selection and hides its controls', () => {
  const a=app();
  vm.runInContext('browsingAlbums=false; selectingImages=true; selectedImages.set("a",{path:"a"}); canEdit=false; applyAccess();',a.context);
  assert.equal(a.get('selectionTools').hidden,true);
  assert.equal(a.read('selectedImages.size'),0);
  assert.equal(a.read('selectingImages'),false);
});

test('album deletion requires confirmation and is hidden from non-admin users', async () => {
  const a=app();
  vm.runInContext('folder="Box"; browsingAlbums=false; state={folders:[{path:"Box",count:2,album:{mode:"box"}}]}; var calls=[]; api=async(url,data)=>{calls.push({url,data});return {folders:state.folders};}; load=async()=>{}; applyAccess();',a.context);
  assert.equal(a.get('deleteAlbumOpen').hidden,false);
  await a.get('deleteAlbumOpen').onclick();
  assert.equal(a.get('deleteAlbumDialog').open,true);
  assert.match(a.get('deleteAlbumSummary').textContent,/Box.*2 photos/);
  assert.equal(a.read('calls.filter(call=>call.url==="/api/delete-album").length'),0);
  a.get('deleteAlbumCancel').onclick();
  assert.equal(a.read('calls.filter(call=>call.url==="/api/delete-album").length'),0);
  await a.get('deleteAlbumOpen').onclick();
  await a.get('deleteAlbumForm').onsubmit({preventDefault(){}});
  assert.deepEqual(a.read('calls.filter(call=>call.url==="/api/delete-album")[0].data'),{folder:'Box'});
  assert.equal(a.get('deleteAlbumDialog').open,false);
  vm.runInContext('canEdit=false; applyAccess();',a.context);
  assert.equal(a.get('deleteAlbumOpen').hidden,true);
});

test('sparse pages render empty cells at their physical coordinates', () => {
  const a=app();
  vm.runInContext('state={mode:"slots",items:[],slots:[{page:2,row:3,col:1},{page:2,row:3,col:2}]}; var slotLibrary=document.createElement("section"); renderSlots(slotLibrary);',a.context);
  assert.equal(a.read('slotLibrary.children.length'),1);
  assert.equal(a.read('slotLibrary.children[0].children[0].children[0].textContent'),'Page 2');
  assert.equal(a.read('slotLibrary.children[0].children[1].children[0].textContent'),'ROW 3');
  assert.equal(a.read('slotLibrary.children[0].children[1].children[1].children.length'),2);
  assert.equal(a.read('slotLibrary.children[0].children[1].children[1].children[1].children[0].textContent'),'COLUMN 2');
});

test('album covers request thumbnails even when card previews are off', () => {
  const a=app();
  vm.runInContext('showPreviews=false; var cover=thumbnail({path:"Album/first.png",filename:"first.png",revision:"r1"},true);',a.context);
  assert.equal(a.read('cover.children.length'),2);
  assert.equal(a.read('cover.children[1].loading'),'lazy');
  assert.match(a.read('cover.children[1].src'),/^\/thumbnail\?/);
  assert.equal(a.read('showPreviews'),false);
});
