// The tour points at real controls. Only the admin performs the actions.
(() => {
  const steps = [
    {title:'Welcome', target:'#adminTourOpen', text:'We’ll look around your collection together. Follow the highlighted control, then choose Next. Skip any step you do not need. You can stop and start again from this menu.'},
    {title:'Connect your photo folder', target:()=>$('folderDialog').open ? '#rootInput' : state?.root ? '#rootLabel' : '#connect', text:()=>state?.root ? 'Your photo folder is already connected. Use the Admin guide if you need help understanding how folders are imported.' : 'Click Connect folder, enter the full location of your main photo folder on the computer running Card Atlas, then choose Connect & scan. The Admin guide explains how to prepare it.', done:()=>!!state?.root},
    {title:'Browse your albums', target:'#allImages', text:'Click Browse albums to see your collection. Find an album filters this list by name.', done:()=>browsingAlbums},
    {title:'Open an album or box', target:'.album-tile, .album-link', text:'Click an album or box to see its photos. A book icon means a paged album. A box icon means photos without fixed positions. If your collection is empty, skip ahead to Add photos.', done:()=>!!folder && !browsingAlbums},
    {title:'Try Grid view', target:'[data-view="grid"]', text:'Click Grid to see the photos as cards.', done:()=>!browsingAlbums && view==='grid'},
    {title:'Try Table view', target:'[data-view="table"]', text:'Click Table to compare names, prices and other details in a list.', done:()=>!browsingAlbums && view==='table'},
    {title:'Try Organized view', target:'[data-view="organized"]', text:'Click Organized to see page, row and column positions. Admins can also see empty cells.', done:()=>!browsingAlbums && view==='organized'},
    {title:'Search the collection', target:'#search', text:'Type a name, tag or description here. Search looks across the collection and shows up to 240 photos per page. Use the album and page filters to narrow the results.', done:()=>!!$('search').value.trim()},
    {title:'Filter by album', target:'#folders', text:'Choose an album or box here to narrow your search. Choose All albums & folders to search everywhere again.'},
    {title:'Filter by physical page', target:'#sheets', text:'In a paged album, choose a page to see that part of the album. Without positions / boxes shows photos without a saved position.'},
    {title:'Choose a sort order', target:'#sort', text:'In Grid or Table view, choose filename order or position order. Organized view always uses saved positions.'},
    {title:'See more results', target:'#pagination', text:'Use Next and Previous below the photos when the results span more than one screen of results. The page count shows where you are.'},
    {title:'Show photo previews', target:'#showPreviews', text:'Show thumbnails turns the small photo previews on or off. You can keep them off while looking through a large collection.'},
    {title:'Open a photo', target:'.card, .table-image', text:'Click a photo to open its details. If you are still on the album list, open an album first.', done:()=>$('detailDialog').open},
    {title:'Edit the details', target:'#descriptionForm', text:'You can edit Card name, Tags, Price and Description. Separate tags with commas, and include a currency with the price. Choose Save details to keep any changes. You can skip this step without changing anything.'},
    {title:'Look at nearby photos', target:'.detail-navigation', text:'Use the arrows to open the previous or next photo. If you have unsaved edits, the app asks before discarding them.'},
    {title:'Move one photo within its album', target:'#pickMove', text:'Move photo lets you choose another page in the same album, then click a destination cell. Clicking another photo swaps their positions. Use Cancel move if you change your mind. For a different album or box, use Select images after closing this window.'},
    {title:'Return to the photos', target:'#closeDetail', text:'Click the close button to return to the collection. Save any edits you want to keep first.', done:()=>!$('detailDialog').open},
    {title:'Select several photos', target:'#selectImages', text:'Click Select images. Then click the photos you want to move. Select all shown selects the current result page; you can also select photos on other pages.', done:()=>selectingImages},
    {title:'Delete photos', target:'#selectImages', text:'Open an album or box and click Select images. Click the photos you want to remove, then Delete selected photos. Check the list before confirming. To delete one photo, open its details and choose Delete photo. Cancel keeps everything. Deleted files stay in the recovery folder.'},
    {title:'Delete several albums or boxes', target:'#allImages', text:'Click Browse albums, then Select albums / boxes. Click each album or box you want to remove and choose Delete selected albums / boxes. Review the list carefully: nested folders and their photos are included. You can cancel before confirming.'},
    {title:'Choose photos to move', target:'#library', text:'Click at least one photo. Selected photos get a green outline. Clear selection lets you start over.', done:()=>selectedImages.size>0},
    {title:'Choose a destination', target:'#transferOpen', text:'Click Move selected to choose another album or box.', done:()=>$('transferDialog').open},
    {title:'Check before moving', target:'#transferForm', text:'Choose the destination and check the photo list. Confirm move moves the files and their saved details. A paged album uses free cells; a box removes position labels. Cancel lets you leave the photos where they are.'},
    {title:'Leave the move window', target:'#transferCancel', text:'Choose Cancel to practise without moving anything, or finish your move and wait for the completion message.', done:()=>!$('transferDialog').open},
    {title:'Finish selecting', target:'#selectImages', text:'Click Done selecting when you have finished. You can then open photos normally again.', done:()=>!selectingImages},
    {title:'Arrange a paged album', target:'[data-view="organized"]', text:'Open a paged album and choose Organized. Drag a photo into an empty cell, or onto another photo to swap positions. These changes save immediately. In a box, dragging one photo onto another swaps their display order.'},
    {title:'Change the album layout', target:'#layoutOpen', text:'In a paged album, click Album layout to change the page, row and column counts. Existing photos must still fit. Boxes do not have this control.', done:()=>$('layoutDialog').open},
    {title:'Review the dimensions', target:'#layoutForm', text:'Increase Pages if you need more room. Save layout applies your changes. Choose Cancel to leave the layout as it is.'},
    {title:'Return from layout', target:'#cancelLayout', text:'Choose Cancel, or save your layout and wait for it to close.', done:()=>!$('layoutDialog').open},
    {title:'Add photos', target:'#importOpen', text:'Click Import photos to add copies of individual image files. The originals stay in place. Use Connect folder instead when you want to use a folder where the photos already live.', done:()=>$('importDialog').open},
    {title:'Choose the import settings', target:'.import-settings', text:'Enter a new name or choose an existing album or box. For new imports, plain filenames select Box automatically; numbered filenames select Paged album. A saved type cannot be changed.'},
    {title:'Choose your files', target:'#importFiles', text:'Choose your photos, or drag individual image files into this window. You can add more with another selection. Check the preview and import order.', done:()=>importPhotos.length>0},
    {title:'Review the import', target:'#reviewButton', text:'Click Review import. If a filename or position is already used, Replace uses the new photo and keeps the saved details. Rename keeps both. Read the preview before deciding.', done:()=>!!importPlan},
    {title:'Finish or cancel the import', target:'#importButton', text:'After every conflict is resolved, Import photos copies the files into the collection. Wait for the completion message. To practise without adding files, use the close button. During an upload, wait for it to finish.'},
    {title:'Return from importing', target:'#cancelImport', text:'Close the import window when you are ready to continue.', done:()=>!$('importDialog').open},
    {title:'Export a list', target:'#exportOpen', text:'Click Export CSV to create a list you can open in a spreadsheet app.', done:()=>$('exportDialog').open},
    {title:'Choose what to export', target:'#exportForm', text:'All albums are selected at first. Uncheck any you do not want, then choose Download CSV. The list includes all photos in those albums, regardless of the current search. Image files are not included.'},
    {title:'Return from export', target:'#cancelExport', text:'Choose Cancel, or finish downloading your list.', done:()=>!$('exportDialog').open},
    {title:'Delete an album', target:'#deleteAlbumOpen', text:'Open an album or box, then click Delete album / box to see the confirmation. The main collection folder cannot be deleted.', done:()=>$('deleteAlbumDialog').open},
    {title:'Read the deletion confirmation', target:'#deleteAlbumForm', text:'Check the name and photo count. Nested albums are included too. Confirm deletion removes the folder from the library and keeps its files in a hidden recovery folder. There is no undo button. Choose Cancel to practise safely.'},
    {title:'Return without deleting', target:'#deleteAlbumCancel', text:'Choose Cancel to keep the album, or finish a deletion you intended to make.', done:()=>!$('deleteAlbumDialog').open},
    {title:'Reload changes from your folder', target:'#rescan', text:'If you add or remove files outside Card Atlas, click Reload from folder to update the collection. Avoid moving or renaming existing files outside the app, because their saved details may not follow them.'},
    {title:'Back up and finish', target:'#signOut', text:'Back up the whole collection folder, including catalog.json. That file keeps your saved details. If you need help with an error, keep the message; technical details are in .log. Sign out when you finish on a shared computer. You can restart this tour from the menu any time.'}
  ];
  let active=false, index=0, highlighted=null, lastStep=-1, queued=false;
  const panel=el('section',undefined,'admin-tour'); panel.hidden=true; panel.setAttribute('aria-label','Getting started tour');
  const progress=el('p',undefined,'tour-progress'),title=el('h2'),body=el('p'),hint=el('p',undefined,'tour-hint');
  const jump=el('select'); jump.setAttribute('aria-label','Jump to a tour step');
  steps.forEach((step,i)=>jump.append(new Option(`${i+1}. ${step.title}`,String(i))));
  const actions=el('div',undefined,'tour-actions');
  const back=el('button','Back','secondary'),skip=el('button','Skip','secondary'),next=el('button','Next'),stop=el('button','End tour','secondary');
  for(const button of [back,skip,next,stop]) button.type='button';
  actions.append(back,skip,next,stop);panel.append(progress,title,body,hint,jump,actions);document.body.append(panel);
  const resolve=value=>typeof value==='function'?value():value;
  function refresh(){
    if(!active)return;
    if(!canEdit){end();return;}
    const step=steps[index],modal=document.querySelector('dialog[open]');
    const target=[...document.querySelectorAll(resolve(step.target))].find(node=>node.getClientRects().length && (!modal || modal.contains(node)));
    const host=modal || document.body;
    if(panel.parentElement!==host)host.append(panel);
    if(highlighted!==target){highlighted?.classList.remove('tour-focus');highlighted=target;highlighted?.classList.add('tour-focus');}
    const changed=lastStep!==index;lastStep=index;
    if(changed && target){const rect=target.getBoundingClientRect();if(rect.top<0 || rect.bottom>window.innerHeight)target.scrollIntoView({block:'center',behavior:'instant'});}
    title.textContent=step.title;body.textContent=resolve(step.text);progress.textContent=`Getting started · ${index+1} of ${steps.length}`;
    const done=!step.done || step.done();
    next.disabled=!done;next.textContent=index===steps.length-1?'Finish':'Next';back.disabled=index===0;skip.hidden=index===steps.length-1;jump.value=String(index);
    hint.textContent=step.done && done?'That’s done. Choose Next when you are ready.':!target?'This control is not available in the current view. Close the open window or open an album, or use Skip to continue.':done?'Ready when you are.':'Follow the highlighted control to continue, or choose Skip.';
    const width=Math.min(360,window.innerWidth-24);panel.style.width=width+'px';
    const rect=target?.getBoundingClientRect(),height=panel.offsetHeight;
    let left=rect?Math.min(window.innerWidth-width-12,Math.max(12,rect.left)):window.innerWidth-width-12;
    let top=rect?rect.bottom+12:window.innerHeight-height-12;
    if(rect && rect.right+width+24<=window.innerWidth){left=rect.right+12;top=rect.top;}
    else if(rect && rect.left>=width+24){left=rect.left-width-12;top=rect.top;}
    if(top+height>window.innerHeight-12)top=rect?rect.top-height-12:12;
    panel.style.left=Math.max(12,left)+'px';panel.style.top=Math.max(12,Math.min(top,window.innerHeight-height-12))+'px';
  }
  function schedule(){if(!active || queued)return;queued=true;requestAnimationFrame(()=>{queued=false;refresh();});}
  function start(){if(!canEdit)return;active=true;index=0;lastStep=-1;panel.hidden=false;refresh();}
  function end(){active=false;panel.hidden=true;highlighted?.classList.remove('tour-focus');highlighted=null;lastStep=-1;}
  function advance(){if(index===steps.length-1){end();return;}index++;refresh();}
  next.onclick=()=>{if(!next.disabled)advance();};skip.onclick=advance;back.onclick=()=>{if(index>0){index--;refresh();}};
  stop.onclick=end;jump.onchange=()=>{index=Number(jump.value);refresh();};
  $('adminTourOpen').onclick=start;
  document.addEventListener('keydown',event=>{if(active && event.key==='Escape'){event.preventDefault();end();}});
  document.addEventListener('click',schedule);document.addEventListener('input',schedule);document.addEventListener('change',schedule);document.addEventListener('close',schedule,true);
  window.addEventListener('resize',schedule);window.addEventListener('scroll',schedule,true);
  // Watch app changes, not the tour's own updates, to avoid observer loops.
  if(typeof MutationObserver!=='undefined'){
    const observer=new MutationObserver(schedule);
    observer.observe($('library'),{childList:true,subtree:true});
    for(const dialog of document.querySelectorAll('dialog'))observer.observe(dialog,{attributes:true,attributeFilter:['open']});
    observer.observe($('accessMode'),{childList:true});
  }
  window.adminTour={start,stop:end};
  if(new URLSearchParams(window.location.search).get('tour')==='1'){
    refreshAccess().then(()=>{if(canEdit)start();else openLogin();}).catch(error=>notify(userErrorMessage(error),true));
  }
})();
