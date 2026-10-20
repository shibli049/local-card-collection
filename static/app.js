const $ = id => document.getElementById(id);
const el = (tag, text, cls) => {
  const node = document.createElement(tag);
  if (text !== undefined) node.textContent = text;
  if (cls) node.className = cls;
  return node;
};
let canEdit = false;
let deletingAlbum = false, albumToDelete = '';
async function openDeleteAlbum(path) {
  if (!canEdit || !path || path === '.' || deletingAlbum) return;
  try {
    const directory = await api('/api/images?browse=albums&all=1');
    if (!canEdit) return;
    const affected = directory.folders.filter(album => album.path === path || album.path.startsWith(path + '/'));
    if (!affected.length) { notify('Album no longer exists. Reload the collection.', true); return; }
    albumToDelete = path;
    const count = affected.reduce((sum,album) => sum + album.count, 0);
    $('deleteAlbumSummary').textContent = `Delete “${folderName(path)}” with ${count} photo${count === 1 ? '' : 's'}${affected.length > 1 ? ` across ${affected.length} albums / boxes, including nested folders` : ''}?`;
    $('deleteAlbumError').textContent = ''; $('deleteAlbumDialog').showModal();
  } catch (error) { notify(error.message,true); }
}
$('deleteAlbumOpen').onclick = () => openDeleteAlbum(folder);
$('deleteAlbumCancel').onclick = () => { if (!deletingAlbum) $('deleteAlbumDialog').close(); };
$('deleteAlbumDialog').addEventListener('cancel', event => { if (deletingAlbum) event.preventDefault(); });
$('deleteAlbumForm').onsubmit = async event => {
  event.preventDefault(); if (!canEdit || !albumToDelete || deletingAlbum) return;
  deletingAlbum = true; $('deleteAlbumConfirm').disabled = true; $('deleteAlbumCancel').disabled = true;
  try {
    await api('/api/delete-album', {folder:albumToDelete});
    $('deleteAlbumDialog').close(); selectedImages.clear(); selectingImages=false; cancelMove();
    folder=''; sheet=''; page=1; browsingAlbums=true; $('albumSearch').value=''; $('search').value='';
    await load(); notify('Album deleted. Files are kept in the local recovery folder.');
  } catch (error) { $('deleteAlbumError').textContent=error.message; }
  finally { deletingAlbum=false; $('deleteAlbumConfirm').disabled=false; $('deleteAlbumCancel').disabled=false; }
};
let selectingImages = false, transferring = false;
const selectedImages = new Map();
function updateSelectionTools() {
  $('selectionTools').hidden = !canEdit || browsingAlbums;
  $('selectImages').textContent = selectingImages ? 'Done selecting' : 'Select images';
  $('selectImages').setAttribute('aria-pressed', String(selectingImages));
  for (const id of ['selectVisible', 'clearSelection', 'transferOpen']) $(id).hidden = !selectingImages;
  $('transferOpen').textContent = `Move selected (${selectedImages.size})…`;
  $('transferOpen').disabled = !selectedImages.size;
}
function selectImage(item) {
  if (selectedImages.has(item.path)) selectedImages.delete(item.path);
  else if (selectedImages.size < 1000) selectedImages.set(item.path, item);
  else notify('Select up to 1000 images per move.', true);
  updateSelectionTools(); renderLibrary();
}
function activateImage(item) {
  if (canEdit && selectingImages) selectImage(item);
  else if (movingCard && canEdit) moveTo(item.folder, slotOf(item), item.path);
  else openDetail(item);
}
$('selectImages').onclick = () => { selectingImages = !selectingImages; cancelMove(); if (!selectingImages) selectedImages.clear(); updateSelectionTools(); renderLibrary(); };
$('selectVisible').onclick = () => { for (const item of state.items) { if (selectedImages.size >= 1000) break; selectedImages.set(item.path,item); } updateSelectionTools(); renderLibrary(); };
$('clearSelection').onclick = () => { selectedImages.clear(); updateSelectionTools(); renderLibrary(); };
function updateTransferSummary() {
  const destination = $('transferDestination').value;
  $('transferSummary').textContent = `Move ${selectedImages.size} selected image${selectedImages.size === 1 ? '' : 's'} to ${destination ? folderName(destination) : 'the selected destination'}?`;
  $('transferConfirm').disabled = !destination || !selectedImages.size || transferring;
}
$('transferDestination').onchange = updateTransferSummary;
$('transferOpen').onclick = async () => {
  if (!canEdit || !selectedImages.size || transferring) return;
  try {
    const directory = await api('/api/images?browse=albums&all=1');
    if (!canEdit || !selectedImages.size) return;
    const sources = new Set([...selectedImages.values()].map(item => item.folder));
    const destinations = directory.folders.filter(album => !sources.has(album.path));
    $('transferDestination').replaceChildren(new Option('Choose a destination', ''), ...destinations.map(album => new Option((album.album?.mode === 'box' ? '📦 ' : '📖 ') + folderName(album.path), album.path)));
    $('transferDestination').value = '';
    $('transferImages').replaceChildren(...[...selectedImages.values()].map(item => el('li', `${item.name || item.filename} · ${folderName(item.folder)}`)));
    $('transferError').textContent = destinations.length ? '' : 'No other albums or boxes are available. Create a destination first.';
    updateTransferSummary(); $('transferDialog').showModal();
  } catch (error) { notify(error.message, true); }
};
$('transferCancel').onclick = () => { if (!transferring) $('transferDialog').close(); };
$('transferDialog').addEventListener('cancel', event => { if (transferring) event.preventDefault(); });
$('transferForm').onsubmit = async event => {
  event.preventDefault();
  if (!canEdit || !selectedImages.size || !$('transferDestination').value || transferring) return;
  const paths = [...selectedImages.keys()], destination = $('transferDestination').value;
  transferring = true; updateTransferSummary(); $('transferCancel').disabled = true; $('transferDestination').disabled = true; $('transferError').textContent = '';
  try {
    await api('/api/transfer', {paths, destination});
    selectedImages.clear(); selectingImages = false; $('transferDialog').close();
    folder = destination; sheet = ''; page = 1; browsingAlbums = false; $('search').value = '';
    await load(); notify(`Moved ${paths.length} images to ${folderName(destination)}.`);
  } catch (error) { $('transferError').textContent = error.message; }
  finally { transferring = false; $('transferCancel').disabled = false; $('transferDestination').disabled = false; updateTransferSummary(); updateSelectionTools(); }
};
let movingCard = null, arranging = false, previewPreference = null;
const cardDragType = 'application/x-card-atlas';
const slotOf = item => ({page:item.page || 0,row:item.row || 0,col:item.col || 0});
const currentAlbum = () => state?.folders.find(f => f.path === folder)?.album;
const slotView = () => canEdit && !browsingAlbums && !!folder && view === 'organized' && !$('search').value.trim() && sheet !== 'none';
let browsingAlbums = true, showPreviews = false, listController;
let page = 1, state, selected, request = 0, folder = '', sheet = '', view = 'grid', importing = false;
try { view = localStorage.getItem('libraryView') || view; } catch {}
if (!['grid', 'table', 'organized'].includes(view)) view = 'grid';
async function api(url, data) {
  const response = await fetch(url, data === undefined ? {} : {
    method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data)
  });
  const body = await response.json();
  if (!response.ok) { if (response.status === 401 && !['/api/login', '/api/session'].includes(url)) { canEdit = false; applyAccess(); } throw Error(body.error || 'Request failed'); }
  return body;
}
const imageUrl = (path, revision = '') => '/image?path=' + encodeURIComponent(path) + '&v=' + encodeURIComponent(revision);
const folderName = path => path === '.' ? 'Root folder' : path;
function automaticPreviews(data) { return data.mode !== 'albums' && (!!folder || (!!$('search').value.trim() && data.total < 100)); }
function applyAccess() {
  $('accessMode').textContent = canEdit ? 'Signed in as administrator' : 'Search-only access';
  for (const id of ['importOpen', 'rescan', 'exportOpen']) $(id).hidden = !canEdit;
  $('connect').hidden = !canEdit || !!state?.root;
  $('signIn').hidden = canEdit; $('signOut').hidden = !canEdit; $('viewerNotice').hidden = canEdit;
  $('emptyImport').hidden = !canEdit || !!state?.count;
  $('emptySignIn').hidden = canEdit || !!state?.count;
  $('nameField').hidden = !canEdit; $('cardName').disabled = !canEdit; $('tagsField').hidden = !canEdit; $('priceField').hidden = !canEdit; $('price').readOnly = !canEdit; $('detailPrice').hidden = canEdit || !selected?.price; $('description').readOnly = !canEdit; $('description').placeholder = canEdit ? 'What should you remember about this image?' : 'No description'; $('saveButton').hidden = !canEdit;
  $('pickMove').hidden = !canEdit;
  $('layoutOpen').hidden = !canEdit || !folder || browsingAlbums || currentAlbum()?.mode !== 'paged';
  $('deleteAlbumOpen').hidden = !canEdit || !folder || folder === '.' || browsingAlbums;
  $('moveTools').hidden = !canEdit || !movingCard;
  if (!canEdit) { selectingImages=false; selectedImages.clear(); movingCard=null; $('deleteAlbumDialog').close(); $('transferDialog').close(); $('layoutDialog').close(); $('importDialog').close(); $('folderDialog').close(); $('exportDialog').close(); }
  updateSelectionTools();
}
async function refreshAccess() { const session = await api('/api/session'); canEdit = session.authenticated; applyAccess(); }
function openLogin() { $('loginError').textContent = ''; $('loginDialog').showModal(); $('username').focus(); }
$('signIn').onclick = openLogin; $('emptySignIn').onclick = openLogin;
$('cancelLogin').onclick = () => $('loginDialog').close();
$('loginForm').onsubmit = async event => {
  event.preventDefault(); $('loginButton').disabled = true; $('loginError').textContent = '';
  try { await api('/api/login', {username: $('username').value, password: $('password').value}); $('password').value = ''; canEdit = true; applyAccess(); $('loginDialog').close(); await load(); notify('Signed in. You can manage this collection.'); }
  catch (e) { $('loginError').textContent = e.message; }
  finally { $('loginButton').disabled = false; }
};
$('signOut').onclick = async () => {
  if ($('detailDialog').open && dirty() && !confirm('Discard your unsaved changes and sign out?')) return;
  try { await api('/api/logout', {}); canEdit = false; $('detailDialog').close(); applyAccess(); await load(); notify('Signed out. Search-only access is active.'); }
  catch (e) { notify(e.message, true); }
};

function notify(message, isError = false) {
  $('notice').textContent = message;
  $('notice').classList.toggle('error', isError);
}
function filtersChanged() { page = 1; load(); }
function chooseFolder(value) { previewPreference=null; if (folder !== value) movingCard=null; browsingAlbums = false; folder = value; sheet = ''; filtersChanged(); }
function clearFilters() { previewPreference=null; movingCard=null; browsingAlbums = true; $('albumSearch').value = ''; folder = ''; sheet = ''; $('search').value = ''; filtersChanged(); }
const previewQueue = [];
let activePreviews = 0;
function drainPreviews() {
  while (activePreviews < 4 && previewQueue.length) {
    const {image, url} = previewQueue.shift();
    if (!image.isConnected) continue;
    activePreviews++;
    const done = () => { activePreviews--; drainPreviews(); };
    image.onload = done;
    image.onerror = () => { image.alt = 'Open image to view original'; if (image.dataset.albumCover) image.hidden=true; done(); };
    image.src = url;
  }
}
const previewObserver = typeof IntersectionObserver === 'undefined' ? null : new IntersectionObserver(entries => {
  for (const entry of entries) if (entry.isIntersecting) {
    const image = entry.target; previewObserver.unobserve(image);
    previewQueue.push({image, url:image.dataset.preview}); drainPreviews();
  }
}, {rootMargin:'150px'});
function thumbnail(item, albumCover = false) {
  const container = el('div', undefined, 'thumb');
  if (!showPreviews && !albumCover) { container.append(el('span', '▧ View image', 'preview-placeholder')); return container; }
  if (albumCover) { container.classList.add('album-cover'); container.append(el('span','▱','album-tile-icon')); }
  const image = el('img'); if (albumCover) image.dataset.albumCover='true'; image.draggable=false; image.alt = item.name || item.filename; image.loading = 'lazy'; image.decoding = 'async';
  const url = '/thumbnail?' + new URLSearchParams({path:item.path, v:item.revision || ''});
  container.append(image);
  if (previewObserver) { image.dataset.preview = url; previewObserver.observe(image); }
  else image.src = url;
  return container;
}
function card(item) {
  const button = el('button', undefined, 'card');
  button.type = 'button'; button.onclick = () => activateImage(item);
  if (canEdit && selectingImages) { button.setAttribute('aria-pressed', String(selectedImages.has(item.path))); button.classList.toggle('selected-image',selectedImages.has(item.path)); button.append(el('span',selectedImages.has(item.path) ? '✓ Selected' : 'Select image','selection-label')); }
  wireCardDrag(button,item);
  const body = el('div', undefined, 'card-body');
  body.append(el('div', '▱ ' + folderName(item.folder), 'folder'), el('h3', item.name || item.filename));
  const badges = el('div', undefined, 'badges');
  for (const [label, key] of [['Page', 'page'], ['Row', 'row'], ['Col', 'col']]) {
    if (item[key] !== null) badges.append(el('span', `${label} ${item[key]}`, 'badge'));
  }
  if (item.page === null) badges.append(el('span', 'Unpositioned', 'badge'));
  body.append(badges);
  const tags = el('div', undefined, 'badges card-tags'); for (const tag of item.tags || []) tags.append(el('span', tag, 'tag')); body.append(tags);
  if (item.price) body.append(el('div', item.price, 'price'));
  body.append(el('p', item.description || (canEdit ? 'Add a description…' : 'No description')));
  button.append(thumbnail(item), body);
  return button;
}
function renderTable(items) {
  const wrap = el('div', undefined, 'table-wrap'), table = el('table');
  const head = el('thead'), row = el('tr');
  for (const title of ['Image', 'Name', 'Filename', 'Album / folder', 'Page', 'Row', 'Column', 'Price', 'Description', 'Tags']) row.append(el('th', title));
  head.append(row); table.append(head);
  const body = el('tbody');
  for (const item of items) {
    const row = el('tr'), imageCell = el('td');
    const imageButton = el('button', undefined, 'table-image');
    imageButton.append(thumbnail(item)); imageButton.onclick = () => activateImage(item); imageButton.setAttribute('aria-label', (selectingImages && canEdit ? 'Select ' : 'View ') + item.filename); imageCell.append(imageButton);
    if (canEdit && selectingImages) { row.classList.toggle('selected-image',selectedImages.has(item.path)); imageButton.append(el('span',selectedImages.has(item.path) ? '✓ Selected' : 'Select image')); imageButton.setAttribute('aria-pressed',String(selectedImages.has(item.path))); }
    const nameCell = el('td'), nameButton = el('button', item.filename, 'text-button');
    nameButton.onclick = () => activateImage(item); nameCell.append(nameButton);
    row.append(imageCell, el('td', item.name || item.filename), nameCell, el('td', folderName(item.folder)), el('td', item.page ?? '—'), el('td', item.row ?? '—'), el('td', item.col ?? '—'), el('td', item.price || '—', 'price'), el('td', item.description || 'No description', 'table-description'), el('td', (item.tags || []).join(', ')));
    body.append(row);
  }
  table.append(body); wrap.append(table); return wrap;
}
function wireCardDrag(node,item) {
  node.draggable = canEdit && !selectingImages;
  if (!canEdit || selectingImages) return;
  node.title = 'Drag to an empty cell to move, or onto a photo to swap';
  node.addEventListener('dragstart',event => {
    if (arranging) { event.preventDefault(); return; }
    movingCard=item; event.dataTransfer.setData(cardDragType,item.path); event.dataTransfer.effectAllowed='move';
    $('moveTools').hidden=false; $('moveHint').textContent=`Moving ${item.name || item.filename}. Drop onto a cell, or navigate to another page and click a destination.`;
  });
  wireDrop(node,item.folder,slotOf(item),item.path);
}
function wireDrop(node,path,slot,target='') {
  if (!canEdit) return;
  node.addEventListener('dragover',event => {
    if (!movingCard || movingCard.folder!==path || ![...(event.dataTransfer?.types || [])].includes(cardDragType)) return;
    event.preventDefault(); event.stopPropagation(); event.dataTransfer.dropEffect='move'; node.classList.add('move-over');
  });
  node.addEventListener('dragleave',()=>node.classList.remove('move-over'));
  node.addEventListener('drop',event => {
    if (![...(event.dataTransfer?.types || [])].includes(cardDragType)) return;
    event.preventDefault(); event.stopPropagation(); node.classList.remove('move-over');
    moveTo(path,slot,target);
  });
}
async function moveTo(path,destination,target='') {
  if (!canEdit || !movingCard || arranging) return;
  if (movingCard.folder!==path) { notify('Choose a cell in the same album.',true); return; }
  if (target===movingCard.path) { cancelMove(); return; }
  arranging=true;
  try {
    await api('/api/arrange',{path:movingCard.path,target,destination,expected:slotOf(movingCard)});
    cancelMove(); await load(); notify(target ? 'Photos swapped. Saved locally.' : 'Photo moved. Saved locally.');
  } catch(e) { cancelMove(); notify(e.message,true); await load(); }
  finally { arranging=false; }
}
function cancelMove() { movingCard=null; $('moveTools').hidden=true; }
$('cancelMove').onclick=cancelMove;
$('pickMove').onclick=()=> {
  if (!canEdit || !selected || !canLeaveDetail()) return;
  movingCard=selected; $('detailDialog').close(); $('preview').removeAttribute('src');
  $('moveTools').hidden=false; $('moveHint').textContent=`Moving ${selected.name || selected.filename}. Choose a page, then click an empty cell or photo to swap.`;
};
$('layoutOpen').onclick=()=> {
  if (!canEdit || currentAlbum()?.mode !== 'paged') return;
  const a=currentAlbum(); $('layoutPages').value=a?.pages || 1; $('layoutRows').value=a?.rows || 3; $('layoutColumns').value=a?.columns || 3;
  $('layoutError').textContent=''; $('layoutDialog').showModal();
};
$('cancelLayout').onclick=()=>$('layoutDialog').close();
$('layoutForm').onsubmit=async event=> {
  event.preventDefault(); if (!canEdit) return; $('saveLayout').disabled=true;
  try { await api('/api/layout',{folder,album:{mode:'paged',pages:Number($('layoutPages').value),rows:Number($('layoutRows').value),columns:Number($('layoutColumns').value)}}); $('layoutDialog').close(); sheet=''; page=1; await load(); notify('Album layout saved. Empty positions are ready to use.'); }
  catch(e) { $('layoutError').textContent=e.message; }
  finally { $('saveLayout').disabled=false; }
};
function renderSlots(library) {
  const pages=new Map();
  const occupied=new Map();
  const key=s=>`${s.page}/${s.row}/${s.col}`;
  for (const item of state.items) { const k=key(slotOf(item)); if (!occupied.has(k)) occupied.set(k,[]); occupied.get(k).push(item); }
  for (const slot of state.slots || []) {
    if (!pages.has(slot.page)) pages.set(slot.page,new Map());
    const rows=pages.get(slot.page); if (!rows.has(slot.row)) rows.set(slot.row,[]); rows.get(slot.row).push(slot);
  }
  for (const [number,rows] of pages) {
    const section=el('section',undefined,'page-group'), heading=el('div',undefined,'page-heading');
    heading.append(el('h3','Page '+number),el('span','Empty positions are shown')); section.append(heading);
    for (const [number,slots] of rows) {
      const row=el('div',undefined,'position-row'),grid=el('div',undefined,'row-images'); row.append(el('div','ROW '+number,'row-label'));
      for (const slot of slots) {
        const cell=el('div',undefined,'position-cell'); cell.append(el('div','COLUMN '+slot.col,'column-label'));
        const items=occupied.get(key(slot)) || [];
        if (items.length) items.forEach(item=>cell.append(card(item)));
        else {
          const empty=el('button',canEdit ? 'Empty cell · drop or move photo here' : 'Empty cell','empty-cell'); empty.type='button';
          empty.setAttribute('aria-label',`Empty cell: page ${slot.page}, row ${slot.row}, column ${slot.col}`);
          empty.onclick=()=>moveTo(folder,slot); wireDrop(empty,folder,slot); cell.append(empty);
        }
        grid.append(cell);
      }
      row.append(grid); section.append(row);
    }
    library.append(section);
  }
}
function renderLibrary() {
  const library = $('library'); if (previewObserver) previewObserver.disconnect(); previewQueue.length = 0; library.replaceChildren();
  document.querySelectorAll('[data-view]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.view === view)));
  $('sort').disabled = view === 'organized';
  if (!state) return;
  if (state.mode === 'albums') {
    const grid = el('div', undefined, 'album-grid');
    for (const album of state.folders) {
      const button = el('button', undefined, 'album-tile'); button.type = 'button';
      button.append(album.cover ? thumbnail(album.cover,true) : el('div', album.album?.mode === 'box' ? '□' : '▱', 'album-tile-icon'), el('h2',(album.album?.mode === 'box' ? '📦 ' : '📖 ') + folderName(album.path)),el('p',`${album.count} cards`));
      if (album.album?.mode === 'paged') button.append(el('span',`${album.album.pages} pages · ${album.album.rows} × ${album.album.columns}`, 'badge'));
      button.onclick = () => chooseFolder(album.path); grid.append(button);
    }
    library.append(grid); return;
  }
  if (state.mode === 'slots') { renderSlots(library); return; }
  if (view === 'table') { library.append(renderTable(state.items)); return; }
  if (view === 'grid') {
    const grid = el('div', undefined, 'grid'); state.items.forEach(item => grid.append(card(item))); library.append(grid); return;
  }
  const groups = new Map();
  for (const item of state.items) {
    if (!groups.has(item.folder)) groups.set(item.folder, new Map());
    const pages = groups.get(item.folder), number = item.page ?? 'none';
    if (!pages.has(number)) pages.set(number, []);
    pages.get(number).push(item);
  }
  for (const [path, pages] of groups) {
    const group = el('section', undefined, 'album-group');
    const heading = el('div', undefined, 'album-heading');
    const title = el('button', '▱ ' + folderName(path), 'text-button'); title.onclick = () => chooseFolder(path);
    const summary = state.folders.find(f => f.path === path);
    const album = summary?.album;
    const kind = album?.mode === 'box' ? 'Box · no positions' : album ? `${album.pages} pages · ${album.rows} rows × ${album.columns} columns` : `${summary?.count ?? 0} images`;
    heading.append(title, el('span', kind)); group.append(heading);
    for (const [number, items] of pages) {
      const section = el('section', undefined, 'page-group');
      const heading = el('div', undefined, 'page-heading');
      heading.append(el('h3', number === 'none' ? (album?.mode === 'box' ? 'Box contents' : 'Without position numbers') : 'Page ' + number), el('span', `${items.length} shown`));
      section.append(heading);
      if (number === 'none') {
        const grid = el('div', undefined, 'grid'); items.forEach(item => grid.append(card(item))); section.append(grid);
      } else {
        // Render each row separately so physical positions remain visible.
        const rows = new Map();
        for (const item of items) { const key = item.row ?? 0; if (!rows.has(key)) rows.set(key, []); rows.get(key).push(item); }
        for (const [number, images] of rows) {
          const row = el('div', undefined, 'position-row');
          row.append(el('div', 'ROW ' + number, 'row-label'));
          const grid = el('div', undefined, 'row-images');
          for (const item of images) { const cell = el('div', undefined, 'position-cell'); cell.append(el('div', 'COLUMN ' + item.col, 'column-label'), card(item)); grid.append(cell); }
          row.append(grid); section.append(row);
        }
      }
      group.append(section);
    }
    library.append(group);
  }
}
function renderNavigation() {
  const nav = $('albumNav'); nav.replaceChildren();
  for (const item of state.folders.slice(0, 40)) {
    const button = el('button', undefined, 'album-link');
    button.classList.toggle('active', folder === item.path); button.title = folderName(item.path);
    button.setAttribute('aria-current', folder === item.path ? 'true' : 'false');
    button.append(el('span', (item.album?.mode === 'box' ? '📦 ' : '📖 ') + folderName(item.path)), el('span', String(item.count), 'album-count'));
    button.onclick = () => chooseFolder(item.path); nav.append(button);
  }
  if ((state.albumCount || 0) > 40) { const more = el('button', 'Browse all albums →', 'album-link'); more.onclick = clearFilters; nav.append(more); }
  if (!state.folders.length) nav.append(el('p', 'Your albums will appear here.', 'nav-empty'));
  $('allImages').classList.toggle('active', !folder);
}
async function load() {
  const id = ++request;
  if (listController) listController.abort();
  listController = typeof AbortController === 'undefined' ? null : new AbortController();
  try {
    const params = new URLSearchParams({q:browsingAlbums ? $('albumSearch').value : $('search').value, folder, sheet, sort:view === 'organized' ? 'position' : $('sort').value, page, summary:'0'});
    if (browsingAlbums) params.set('browse','albums');
    else if (slotView()) params.set('layout','auto');
    const response = await fetch('/api/images?' + params, listController ? {signal:listController.signal} : {});
    if (!response.ok) throw Error('Could not load the collection.');
    const data = await response.json();
    if (id !== request) return;
    state = data; page = data.page;
    showPreviews = previewPreference ?? automaticPreviews(data);
    $('showPreviews').checked=showPreviews;
    $('sideCount').textContent = data.count;
    $('rootLabel').textContent = canEdit ? data.root || 'Import photos to create your library, or connect an existing folder.' : 'Local image collection · browse and search';
    $('connect').hidden = !!data.root; $('rescan').disabled = !data.root;
    $('folders').replaceChildren(new Option('All albums & folders', ''), ...data.folders.map(f => new Option(folderName(f.path) + ` (${f.count})`, f.path)));
    if (folder && !data.folders.some(f => f.path === folder)) $('folders').append(new Option(folderName(folder),folder));
    $('folders').value = folder;
    $('existingAlbums').replaceChildren(...data.folders.map(f => new Option(folderName(f.path), f.path)));
    $('sheets').replaceChildren(new Option('All pages', ''), ...data.pages.map(p => new Option('Page ' + p, p)));
    if (data.unpositioned || sheet === 'none') $('sheets').append(new Option('Without positions / boxes', 'none'));
    if (sheet && ![...$('sheets').options].some(o => o.value === sheet)) $('sheets').append(new Option('Page ' + sheet, sheet));
    $('sheets').value = sheet;
    const albumMode = data.mode === 'albums';
    $('albumTools').hidden = !albumMode; $('viewControls').hidden = albumMode; $('sheets').hidden = albumMode; $('showPreviews').disabled = albumMode; $('sheets').disabled = albumMode;
    $('heading').textContent = albumMode ? 'Your albums.' : folder ? folderName(folder) : 'Card collection.';
    const album = data.folders.find(f => f.path === folder)?.album;
    $('subtitle').textContent = album?.mode === 'box' ? 'Box contents, with no page or position numbers.' : album ? `${album.pages} pages · ${album.rows} rows × ${album.columns} columns per page` : 'Your collection, organized your way.';
    $('breadcrumb').textContent = (folder ? folderName(folder) : 'All cards') + (sheet ? ` / ${sheet === 'none' ? 'Without positions' : 'Page ' + sheet}` : '');
    $('resultCount').textContent = `${data.total} image${data.total === 1 ? '' : 's'}${data.total !== data.count ? ' found' : ' in your collection'}`;
    $('resultHint').textContent = view === 'organized' ? 'Grouped by folder → page → row → column' : canEdit ? 'Select an image to view and edit its details' : 'Select an image to view its details';
    if (canEdit && folder && view === 'organized') $('resultHint').textContent = 'Drag a photo to move it; drop onto another photo to swap. Use Album layout to include empty pages.';
    if (albumMode) { $('resultCount').textContent = `${data.total} albums · ${data.count} cards in your collection`; $('resultHint').textContent = 'Choose an album to load its cards'; }
    $('empty').hidden = data.mode === 'slots' || data.total !== 0;
    $('empty').querySelector('h2').textContent = data.count ? 'No matching images' : 'Your collection starts here';
    $('empty').querySelector('p').textContent = data.count ? 'Try another search or clear your filters.' : canEdit ? 'Import photos into an album or connect an existing image folder.' : 'There are no photos yet. An administrator can import photos or connect a folder.';
    $('clearFilters').hidden = !data.count; $('emptyImport').hidden = !!data.count;
    $('pageLabel').textContent = data.mode === 'slots' ? `Positions ${data.page} of ${data.pageCount} · up to 60 cells per view` : albumMode ? `Albums ${data.page} of ${data.pageCount} · up to 50 albums per view` : `Cards ${data.page} of ${data.pageCount} · up to 60 cards per view`;
    $('prev').disabled = data.page === 1; $('next').disabled = data.page >= data.pageCount;
    $('pagination').hidden = data.mode !== 'slots' && !data.total;
    applyAccess(); renderNavigation(); renderLibrary();
  } catch (e) { if (e.name !== 'AbortError' && id === request) notify(e.message, true); }
}
$('connect').onclick = () => { $('folderDialog').showModal(); $('rootInput').focus(); };
$('cancelFolder').onclick = () => $('folderDialog').close();
$('folderForm').onsubmit = async event => {
  event.preventDefault(); $('scanButton').disabled = true; $('folderError').textContent = '';
  try { await api('/api/scan', {root: $('rootInput').value.trim()}); $('folderDialog').close(); clearFilters(); notify('Folder connected.'); }
  catch (e) { $('folderError').textContent = e.message; }
  finally { $('scanButton').disabled = false; }
};
$('rescan').onclick = async () => {
  $('rescan').disabled = true; notify('Reloading files from folder…');
  try { const result = await api('/api/scan', {}); await load(); notify(`Folder reloaded. ${result.count} images in your collection.`); }
  catch (e) { notify(e.message, true); }
  finally { $('rescan').disabled = false; }
};
let timer;
$('search').oninput = () => { cancelMove(); previewPreference=null; browsingAlbums = !$('search').value.trim(); folder = ''; sheet = ''; $('folders').value = ''; $('sheets').value = ''; clearTimeout(timer); timer = setTimeout(filtersChanged, 200); };
$('albumSearch').oninput = () => { browsingAlbums = true; clearTimeout(timer); timer = setTimeout(filtersChanged,200); };
$('showPreviews').onchange = () => { showPreviews = $('showPreviews').checked; previewPreference=showPreviews; renderLibrary(); };
$('folders').onchange = () => chooseFolder($('folders').value);
$('sheets').onchange = () => { sheet = $('sheets').value; filtersChanged(); };
$('sort').onchange = filtersChanged;
$('allImages').onclick = clearFilters; $('clearFilters').onclick = clearFilters;
$('prev').onclick = () => { page--; load(); }; $('next').onclick = () => { page++; load(); };
document.querySelectorAll('[data-view]').forEach(button => button.onclick = () => {
  view = button.dataset.view; try { localStorage.setItem('libraryView', view); } catch {} filtersChanged();
});
function selectedTags() { const seen = new Set(); return $('tagsInput').value.split(',').map(tag => tag.trim()).filter(tag => { const key = tag.toLowerCase(); if (!tag || seen.has(key)) return false; seen.add(key); return true; }); }
function dirty() { return canEdit && selected && ($('description').value !== selected.description || $('price').value !== (selected.price || '') || $('cardName').value !== (selected.name || '') || JSON.stringify(selectedTags()) !== JSON.stringify(selected.tags || [])); }
function canLeaveDetail() { return !dirty() || confirm('Discard your unsaved changes?'); }
function openDetail(item) {
  selected = item; $('detailName').textContent = item.name || item.filename; $('cardName').value = item.name || ''; $('tagsInput').value = (item.tags || []).join(', '); $('detailTags').replaceChildren(...(item.tags || []).map(tag => el('span',tag,'tag')));
  $('preview').src = imageUrl(item.path,item.revision); $('preview').alt = item.filename; $('original').href = imageUrl(item.path,item.revision);
  $('price').value = item.price || ''; $('nameField').hidden = !canEdit; $('cardName').disabled = !canEdit; $('tagsField').hidden = !canEdit; $('priceField').hidden = !canEdit; $('detailPrice').textContent = item.price || ''; $('detailPrice').hidden = canEdit || !item.price; $('description').value = item.description; $('saveStatus').textContent = ''; $('metadata').replaceChildren();
  $('pickMove').hidden = !canEdit;
  const album = state.folders.find(f => f.path === item.folder)?.album;
  for (const [label, value] of [['Filename', item.filename], ['Folder', folderName(item.folder)], ['Type', album?.mode === 'box' ? 'Box' : item.page !== null ? 'Paged image' : 'Unpositioned image'], ['Page', item.page ?? '—'], ['Row', item.row ?? '—'], ['Column', item.col ?? '—'], ['Path', item.path]]) $('metadata').append(el('dt', label), el('dd', String(value)));
  const index = state.items.findIndex(i => i.path === item.path);
  $('detailPrev').disabled = index <= 0; $('detailNext').disabled = index >= state.items.length - 1;
  if (!$('detailDialog').open) $('detailDialog').showModal();
}
$('closeDetail').onclick = () => { if (canLeaveDetail()) { $('detailDialog').close(); $('preview').removeAttribute('src'); } };
$('detailDialog').addEventListener('cancel', event => { event.preventDefault(); $('closeDetail').click(); });
for (const [id, delta] of [['detailPrev', -1], ['detailNext', 1]]) $(id).onclick = () => {
  if (!canLeaveDetail()) return;
  const index = state.items.findIndex(i => i.path === selected.path), item = state.items[index + delta];
  if (item) openDetail(item);
};
$('descriptionForm').onsubmit = async event => {
  event.preventDefault(); if (!canEdit) return; $('saveButton').disabled = true; $('saveStatus').textContent = 'Saving…';
  try { const description = $('description').value, price = $('price').value.trim(), name = $('cardName').value.trim(), tags = selectedTags(); await api('/api/description', {path: selected.path, description, price, name, tags}); selected.name = name; selected.tags = tags; $('cardName').value = name; $('detailName').textContent = name; $('detailTags').replaceChildren(...tags.map(tag => el('span', tag, 'tag'))); selected.description = description; selected.price = price; $('price').value = price; $('saveStatus').textContent = 'Saved locally'; await load(); }
  catch (e) { $('saveStatus').textContent = e.message; }
  finally { $('saveButton').disabled = false; }
};

// The reviewed order is exactly the order sent to the server.
let importPhotos = [];
let importPlan = null, importActions = {}, conflictSelection = new Set(), conflictPage = 1, reviewRequest = 0, reviewing = false, lastAlbumName = null;
function orderedFiles() {
  const files = [...importPhotos];
  if ($('importOrder').value !== 'selection') {
    const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: 'base'});
    files.sort((a, b) => collator.compare(a.name, b.name));
    if ($('importOrder').value === 'reverse') files.reverse();
  }
  return files;
}
function importSettings() {
  return {album: $('albumName').value.trim(), mode: $('albumMode').value, pages: Number($('importPages').value), rows: Number($('importRows').value), columns: Number($('importColumns').value), startPage: Number($('startPage').value), startRow: Number($('startRow').value), startColumn: Number($('startColumn').value)};
}
function assignedPosition(index, s) {
  index += ((s.startPage - 1) * s.rows + s.startRow - 1) * s.columns + s.startColumn - 1;
  return {page: Math.floor(index / (s.rows * s.columns)) + 1, row: Math.floor(index / s.columns) % s.rows + 1, col: index % s.columns + 1};
}
function importValidation(files, s) {
  if (!s.album) return 'Enter or choose an album or box name.';
  if (s.album !== '.' && s.album.split('/').some(part => !part || part.length > 120 || /[<>:"\\|?*\x00-\x1f]/.test(part) || /^[ .]|[ .]$/.test(part) || /^(CON|PRN|AUX|NUL|COM[0-9]|LPT[0-9])(?:\.|$)/i.test(part) || part.startsWith('.soft-inventory-'))) return 'Use folder names without special characters or leading/trailing dots or spaces.';
  if (!files.length) return 'Select photos to import.';
  if (files.length > 10000) return 'Import up to 10,000 images at a time.';
  if (files.some(f => !/\.(jpe?g|png|gif|webp|bmp|avif|ico)$/i.test(f.name))) return 'Some files have unsupported image extensions.';
  if (files.some(f => !f.size || f.size > 50 * 1024 * 1024)) return 'Each photo must be nonempty and no larger than 50 MiB.';
  if (files.reduce((sum, f) => sum + f.size, 0) > 1024 * 1024 * 1024 - 2000000) return 'This import exceeds 1 GiB. Select fewer photos.';
  if (s.mode === 'paged') {
    if (![s.pages, s.rows, s.columns, s.startPage, s.startRow, s.startColumn].every(Number.isInteger) || s.pages < 1 || s.pages > 1000 || s.rows < 1 || s.rows > 100 || s.columns < 1 || s.columns > 100) return 'Enter 1–1000 pages and 1–100 rows and columns.';
    if (s.startPage < 1 || s.startPage > s.pages || s.startRow < 1 || s.startRow > s.rows || s.startColumn < 1 || s.startColumn > s.columns) return 'The starting position must be inside the album.';
    const start = ((s.startPage - 1) * s.rows + s.startRow - 1) * s.columns + s.startColumn - 1;
    if (start + files.length > s.pages * s.rows * s.columns) return 'Not enough positions from the starting point. Increase Pages or select fewer photos.';
  }
  return '';
}
function syncAlbumSettings() {
  const name = $('albumName').value.trim(), existing = state?.folders.find(f => f.path.toLowerCase() === name.toLowerCase())?.album;
  if (name !== lastAlbumName && existing) {
    $('albumMode').value = existing.mode;
    if (existing.mode === 'paged') { $('importPages').value = existing.pages; $('importRows').value = existing.rows; $('importColumns').value = existing.columns; }
  }
  lastAlbumName = name;
  $('albumMode').disabled = !!existing;
  $('importPages').min = existing?.mode === 'paged' ? existing.pages : 1;
  $('importRows').disabled = !!existing || $('albumMode').value === 'box';
  $('importColumns').disabled = !!existing || $('albumMode').value === 'box';
  $('existingAlbumHint').hidden = !existing || existing.mode === 'box';
}
function renderImportPreview() {
  $('fileSelectionCount').textContent = importPhotos.length ? `${importPhotos.length} photo${importPhotos.length === 1 ? '' : 's'} selected` : 'No photos selected';
  $('clearImportFiles').disabled = !importPhotos.length || importing;
  const s = importSettings(), files = orderedFiles(), box = s.mode === 'box';
  $('dimensions').hidden = box; $('startPosition').hidden = box;
  for (const id of ['importPages', 'startPage', 'startRow', 'startColumn']) $(id).disabled = box;
  $('orderHint').textContent = box ? 'Photos keep their filenames unless you choose Rename for a conflict. Boxes have no position numbers.' : 'Fill left to right, then the next row, then the next page, from the starting position. Review occupied positions before importing.';
  const message = importValidation(files, s);
  $('reviewButton').disabled = !!message || reviewing || importing;
  $('importButton').disabled = !!message || !importPlan || importPlan.unresolved > 0 || reviewing || importing;
  $('importSummary').textContent = !files.length ? 'Select photos to preview their positions.' : `${files.length} photo${files.length === 1 ? '' : 's'} → ${s.album || 'Album'} · ${box ? 'no position numbers' : `${s.pages} pages × ${s.rows} rows × ${s.columns} columns`}${importPlan ? ` · ${importPlan.unresolved ? importPlan.unresolved + ' conflicts need a choice' : 'ready to import'}` : ' · review before importing'}`;
  $('importPreview').replaceChildren();
  if (!files.length) return;
  const table = el('table'), head = el('thead'), header = el('tr');
  for (const title of ['#', 'Original filename', ...(importPlan ? ['Saved filename'] : []), ...(box ? [] : ['Page', 'Row', 'Column'])]) header.append(el('th', title));
  head.append(header); table.append(head); const body = el('tbody');
  files.slice(0, 100).forEach((file, index) => {
    const planned = importPlan?.entries[index], row = el('tr'); row.append(el('td', index + 1), el('td', file.name));
    if (importPlan) row.append(el('td', planned.filename));
    if (!box) { const p = planned || assignedPosition(index, s); row.append(el('td', p.page), el('td', p.row), el('td', p.col)); }
    body.append(row);
  });
  table.append(body); $('importPreview').append(table);
  if (files.length > 100) $('importPreview').append(el('p', `Showing the first 100 of ${files.length} photos. All photos follow this order.`, 'hint'));
}
function updateImportPreview() {
  if (importing) return;
  ++reviewRequest; reviewing = false; importPlan = null; importActions = {}; conflictSelection.clear(); conflictPage = 1;
  $('conflictSection').hidden = true; $('importStatus').textContent = ''; syncAlbumSettings(); renderImportPreview();
  const message = importValidation(orderedFiles(), importSettings()); $('importError').textContent = message && orderedFiles().length ? message : '';
}
function renderConflicts() {
  const conflicts = importPlan?.entries.filter(e => e.conflicts.length) || [];
  $('conflictSection').hidden = !conflicts.length;
  if (!conflicts.length) return;
  $('conflictTitle').textContent = `${conflicts.length} conflict${conflicts.length === 1 ? '' : 's'} · ${importPlan.unresolved} need a choice`;
  $('selectAllConflicts').checked = conflicts.every(e => conflictSelection.has(e.index));
  $('selectAllConflicts').indeterminate = conflictSelection.size > 0 && !$('selectAllConflicts').checked;
  $('replaceSelected').disabled = !conflictSelection.size || reviewing;
  $('renameSelected').disabled = !conflictSelection.size || reviewing;
  const table = el('table'), head = el('thead'), header = el('tr');
  ['Select', 'Incoming photo / conflict', 'Choice', 'Saved as'].forEach(text => header.append(el('th', text))); head.append(header); table.append(head);
  const body = el('tbody');
  conflictPage = Math.max(1, Math.min(conflictPage, Math.ceil(conflicts.length / 50)));
  for (const entry of conflicts.slice((conflictPage - 1) * 50, conflictPage * 50)) {
    const row = el('tr'), selectCell = el('td'), checkbox = el('input'); checkbox.type = 'checkbox'; checkbox.checked = conflictSelection.has(entry.index); checkbox.setAttribute('aria-label', 'Select conflict ' + (entry.index + 1));
    checkbox.onchange = () => { if (checkbox.checked) conflictSelection.add(entry.index); else conflictSelection.delete(entry.index); renderConflicts(); }; selectCell.append(checkbox);
    const fileCell = el('td'); fileCell.append(el('strong', entry.original), el('p', entry.conflicts.join(', '), 'hint'));
    const choiceCell = el('td'), select = el('select'); select.setAttribute('aria-label', 'Conflict action for photo ' + (entry.index + 1));
    select.append(new Option('Choose…', ''), new Option('Replace', 'replace'), new Option('Rename', 'rename'));
    select.options[1].disabled = !entry.canReplace; select.value = importActions[entry.index] || ''; select.disabled = reviewing;
    select.onchange = () => { importActions[entry.index] = select.value; reviewImport(true); }; choiceCell.append(select);
    const output = el('td', entry.filename); if (entry.page !== null) output.append(el('p', `Page ${entry.page} · row ${entry.row} · col ${entry.col}`, 'hint'));
    row.append(selectCell, fileCell, choiceCell, output); body.append(row);
  }
  table.append(body); $('conflictList').replaceChildren(table);
  $('conflictPageLabel').textContent = `${conflictPage} of ${Math.ceil(conflicts.length / 50)}`;
  $('conflictPrev').disabled = conflictPage === 1; $('conflictNext').disabled = conflictPage * 50 >= conflicts.length;
}
async function reviewImport(keepActions = false) {
  const files = orderedFiles(), options = importSettings(), message = importValidation(files, options);
  if (message) { $('importError').textContent = message; return; }
  if (!keepActions) { importActions = {}; conflictSelection.clear(); conflictPage = 1; }
  const id = ++reviewRequest; reviewing = true; renderImportPreview(); renderConflicts(); $('importError').textContent = ''; $('importStatus').textContent = 'Checking destination files…';
  try {
    const plan = await api('/api/import-plan', {options, files: files.map(f => f.name), actions: importActions});
    if (id !== reviewRequest) return; importPlan = plan;
  } catch (e) { if (id === reviewRequest) { $('importError').textContent = e.message; importPlan = importPlan ? {...importPlan, unresolved: Math.max(1, importPlan.unresolved)} : null; } }
  finally { if (id === reviewRequest) { reviewing = false; $('importStatus').textContent = ''; renderImportPreview(); renderConflicts(); } }
}
function openImport() {
  if (!canEdit) { openLogin(); return; }
  if (folder) $('albumName').value = folder;
  $('importDialog').showModal(); updateImportPreview();
}
$('importOpen').onclick = openImport; $('emptyImport').onclick = openImport;
$('cancelImport').onclick = () => { if (!importing) $('importDialog').close(); };
$('importDialog').addEventListener('cancel', event => { if (importing) event.preventDefault(); });
$('importFiles').addEventListener('change', () => {
  if (!canEdit || importing) return;
  importPhotos.push(...$('importFiles').files);
  $('importFiles').value = '';
});
$('clearImportFiles').onclick = () => { if (!importing) { importPhotos = []; $('importFiles').value = ''; updateImportPreview(); } };
let dragDepth = 0;
const draggingFiles = event => [...(event.dataTransfer?.types || [])].includes('Files');
$('dropZone').addEventListener('dragenter', event => {
  if (!draggingFiles(event) || !canEdit || importing) return;
  event.preventDefault(); dragDepth++; $('dropZone').classList.add('drag-over');
});
$('dropZone').addEventListener('dragleave', () => {
  dragDepth = Math.max(0, dragDepth - 1);
  if (!dragDepth) $('dropZone').classList.remove('drag-over');
});
document.addEventListener('dragover', event => {
  if (!draggingFiles(event)) return;
  event.preventDefault(); event.dataTransfer.dropEffect = canEdit && !importing ? 'copy' : 'none';
});
document.addEventListener('drop', event => {
  if (!draggingFiles(event)) return;
  event.preventDefault(); dragDepth = 0; $('dropZone').classList.remove('drag-over');
  if (!canEdit) { notify('Sign in as administrator to import photos.', true); openLogin(); return; }
  if (importing) { notify('Wait for the current import to finish before adding photos.', true); return; }
  const items = [...(event.dataTransfer.items || [])];
  if (items.some(item => item.webkitGetAsEntry?.()?.isDirectory)) {
    if (!$('importDialog').open) openImport();
    $('importError').textContent = 'Drop individual photo files, not folders. Select the photos inside the folder and drop them together.';
    return;
  }
  const photos = [...event.dataTransfer.files];
  if (!photos.length) return;
  if (!$('importDialog').open) openImport();
  importPhotos.push(...photos); updateImportPreview();
});
for (const id of ['albumName', 'albumMode', 'importPages', 'importRows', 'importColumns', 'startPage', 'startRow', 'startColumn', 'importFiles', 'importOrder']) $(id).addEventListener(['albumMode', 'importFiles', 'importOrder'].includes(id) ? 'change' : 'input', updateImportPreview);
$('reviewButton').onclick = () => reviewImport();
$('selectAllConflicts').onchange = () => { conflictSelection.clear(); if ($('selectAllConflicts').checked) importPlan.entries.filter(e => e.conflicts.length).forEach(e => conflictSelection.add(e.index)); renderConflicts(); };
for (const [id, action] of [['replaceSelected', 'replace'], ['renameSelected', 'rename']]) $(id).onclick = () => {
  for (const index of conflictSelection) importActions[index] = action;
  reviewImport(true);
};
$('conflictPrev').onclick = () => { conflictPage--; renderConflicts(); }; $('conflictNext').onclick = () => { conflictPage++; renderConflicts(); };
function upload(form, settings) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest(); xhr.open('POST', '/api/import?' + new URLSearchParams(settings));
    xhr.upload.onprogress = event => { if (event.lengthComputable) { const percent = Math.round(event.loaded / event.total * 100); $('importProgress').value = percent; $('importStatus').textContent = percent === 100 ? 'Organizing and saving photos…' : `Uploading ${percent}%…`; } };
    xhr.onerror = () => reject(Error('Connection interrupted. Reload the folder before retrying.'));
    xhr.onload = () => { try { const data = JSON.parse(xhr.responseText); if (xhr.status < 200 || xhr.status >= 300) { if (xhr.status === 401) { canEdit = false; applyAccess(); } reject(Error(data.error || 'Import failed')); } else resolve(data); } catch { reject(Error('Import failed. Check that the app is still running.')); } };
    xhr.send(form);
  });
}
$('importForm').onsubmit = async event => {
  event.preventDefault(); const files = orderedFiles(), settings = importSettings(), message = importValidation(files, settings);
  if (message || !importPlan || importPlan.unresolved) { $('importError').textContent = message || 'Review the import and resolve every conflict first.'; return; }
  const form = new FormData(); form.append('review', JSON.stringify({fingerprint: importPlan.fingerprint, actions: importActions})); files.forEach(file => form.append('images', file, file.name));
  let failure = ''; importing = true; $('importError').textContent = ''; $('importStatus').textContent = 'Starting import…'; $('importProgress').hidden = false; $('importProgress').value = 0;
  const controls = [...$('importForm').querySelectorAll('input, select, button')]; controls.forEach(control => control.disabled = true);
  try {
    const result = await upload(form, settings); $('importDialog').close(); $('importForm').reset(); importPhotos = []; lastAlbumName = null; browsingAlbums = false; folder = result.album; sheet = ''; page = 1; $('search').value = ''; await load(); notify(`Imported ${result.count} photo${result.count === 1 ? '' : 's'} into ${result.album}.`);
  } catch (e) { failure = e.message; }
  finally { importing = false; controls.forEach(control => control.disabled = false); $('importProgress').hidden = true; $('importStatus').textContent = ''; updateImportPreview(); if (failure) $('importError').textContent = failure; }
};
window.addEventListener('beforeunload', event => { if (importing || ($('detailDialog').open && dirty())) { event.preventDefault(); event.returnValue = ''; } });
refreshAccess().then(load).catch(error => notify(error.message, true));

// CSV exports always include whole selected albums, not just visible results.
let exportSelection = new Set(), exportDirectory = null, exportPage = 1;
const exportAlbumsAvailable = () => exportDirectory || state?.folders || [];
function renderExportSelection() {
  const albums = exportAlbumsAvailable();
  const visible = albums.filter(a => a.path.toLowerCase().includes($('exportAlbumSearch').value.toLowerCase()));
  exportPage = Math.max(1, Math.min(exportPage, Math.ceil(visible.length / 50)));
  $('exportAlbums').replaceChildren();
  for (const album of visible.slice((exportPage-1)*50, exportPage*50)) {
    const label = el('label', undefined, 'export-album');
    const checkbox = el('input'); checkbox.type = 'checkbox'; checkbox.checked = exportSelection.has(album.path);
    checkbox.onchange = () => { if (checkbox.checked) exportSelection.add(album.path); else exportSelection.delete(album.path); renderExportSelection(); };
    label.append(checkbox, el('span', folderName(album.path)), el('span', `${album.count} cards`, 'album-count'));
    $('exportAlbums').append(label);
  }
  const count = albums.filter(a => exportSelection.has(a.path)).reduce((sum, a) => sum + a.count, 0);
  $('exportSummary').textContent = `${exportSelection.size} albums selected · ${count} cards`;
  $('exportDownload').disabled = !exportSelection.size;
  $('exportAll').checked = albums.length > 0 && albums.every(a => exportSelection.has(a.path));
  $('exportAll').indeterminate = exportSelection.size > 0 && !$('exportAll').checked;
  $('exportPageLabel').textContent = `${exportPage} of ${Math.max(1,Math.ceil(visible.length/50))}`; $('exportPrev').disabled = exportPage === 1; $('exportNext').disabled = exportPage*50 >= visible.length;
}
$('exportOpen').onclick = async () => {
  if (!canEdit) { openLogin(); return; }
  try {
    exportDirectory = null;
    if ((state?.albumCount || 0) > (state?.folders.length || 0)) { const directory = await api('/api/images?browse=albums&all=1'); exportDirectory = directory.folders; }
    exportPage = 1; $('exportAlbumSearch').value = ''; exportSelection = new Set(exportAlbumsAvailable().map(a => a.path)); renderExportSelection(); $('exportDialog').showModal();
  } catch (error) { notify(error.message,true); }
};
$('exportAll').onchange = () => { exportSelection = $('exportAll').checked ? new Set(exportAlbumsAvailable().map(a => a.path)) : new Set(); renderExportSelection(); };
$('exportAlbumSearch').oninput = () => { exportPage = 1; renderExportSelection(); };
$('exportPrev').onclick = () => { exportPage--; renderExportSelection(); }; $('exportNext').onclick = () => { exportPage++; renderExportSelection(); };
$('cancelExport').onclick = () => $('exportDialog').close();
$('exportForm').onsubmit = async event => {
  event.preventDefault(); if (!canEdit || !exportSelection.size) return;
  try {
    await refreshAccess(); if (!canEdit) { openLogin(); return; }
    const response = await fetch('/api/export', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({albums:[...exportSelection]})});
    if (!response.ok) { if (response.status === 401) await refreshAccess(); const body = await response.json(); throw Error(body.error || 'Export failed'); }
    const downloadUrl = URL.createObjectURL(await response.blob());
    const link = el('a'); link.href = downloadUrl; link.download = 'CardAtlas-export.csv';
    document.body.append(link); link.click(); link.remove(); setTimeout(() => URL.revokeObjectURL(downloadUrl), 1000); $('exportDialog').close();
  } catch (error) { notify(error.message, true); }
};
