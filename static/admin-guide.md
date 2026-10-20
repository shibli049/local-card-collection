# Looking after your collection

This guide walks you through the jobs you can do as an admin. Keep it open while you read, then choose Close to return to your collection. You can find it again under Admin guide after signing in.

## Get started

Your collection stays on this computer. Keep the Card Atlas window that opened with the app running while you work. Closing it stops the app.

A paged album has page, row and column positions, like a physical album. A box holds photos without fixed positions. The book icon marks an album; the box icon marks a box.

To use photos already on your computer, choose Connect folder and enter the full location of the folder. If you are unsure how to find that, ask someone to help you choose it. Photos stay where they are, and folders inside it are included.

Numbered filenames such as `card-2-3-1.jpg` mean page 2, row 3, column 1. A folder with no numbered photos becomes a box. A folder containing numbered photos becomes a paged album.

## Add photos

1. Choose Import photos. Enter a new album or box name, or choose one you already have.
2. Choose your photos, or drag individual image files into the import window. You can add more with another selection. Use Clear selection if you want to start over.
3. Check Organization. For a new name, unnumbered photos automatically select Box. If the selection contains numbered photos, it selects Paged album. You can change this choice before importing.
4. For a paged album, enter its page count, rows per page and columns per row. Check where the first photo will go. The preview shows the order and positions.
5. Choose Review import. Resolve any duplicates, then choose Import photos. Wait for the completion message before closing the app.

> Choose the type carefully. Once an album or box has been created, its type stays fixed. To use a different type later, move the photos into another album or box.

Imported photos are copies. The originals you selected stay in place. Boxes keep filenames; paged imports add position numbers to the saved filenames. You can import JPG, PNG, GIF, WebP, BMP, AVIF and ICO images. Each photo must be no larger than 50 MiB.

## Handle duplicates

If a filename or album position is already in use, the import pauses for your choice.

- Replace uses the new photo in place of the existing one and keeps its saved details. Check the photo and position before choosing this.
- Rename keeps both photos. In a box, the new filename gets an extra number. In a paged album, the new photo takes a free position.

If there is no room, increase Pages before reviewing again. If the folder changes while you are checking the preview, choose Review import again to get the latest information.

## Find a photo

Open an album from Browse albums or the sidebar. Use the main search box to look for a name, tag, description or filename. Starting a search clears the album filter, so it looks across the collection. Choose an album from the filter if you want to narrow the results.

Grid shows photo cards. Table puts the details into a list. Organized groups photos by their saved positions. Search shows up to 240 photos at a time; use Next to see more. Show thumbnails lets you turn the small previews on or off.

## Edit a photo’s details

1. Open the photo you want to change.
2. Edit Card name, Tags, Price or Description. Separate tags with commas, for example `Rare, Blue, Swedish`. Include the currency when entering a price, such as `25 SEK`.
3. Choose Save details and wait for Saved locally.

Changing the card name leaves its filename alone. The arrow buttons open nearby photos. If you try to leave with unsaved changes, the app asks whether you want to discard them.

## Move photos to another album or box

1. Open the source album and choose Select images.
2. Click the photos to select them, or choose Select all shown. Your selection stays while you browse other result pages. You can select up to 1,000 photos for one move.
3. Choose Move selected, then choose a destination.
4. Check the photo list and destination. Choose Confirm move to go ahead, or Cancel to leave everything where it is.

This moves the photos out of their current folders. Their names and saved details go with them. A paged destination uses its first free cells; a box removes position labels. A matching filename or a full album stops the move. Resolve that before trying again.

## Arrange photos inside an album

Open a paged album and choose Organized. As an admin, you can see its empty cells and pages. Drag a photo to an empty cell, or onto another photo to swap their positions.

For a move to another page in the same album, open the photo and choose Move photo. The detail window closes so you can choose the destination page and click a cell. Choose Cancel move if you change your mind.

Use Album layout to add pages or change the row and column counts. The layout must still fit the photos already there. Boxes have no page layout; dragging one box photo onto another swaps their display order.

## Delete an album or box

1. Open the album or box, then choose Delete album / box.
2. Check the name and photo count. Deleting a folder also includes any albums inside it.
3. Choose Confirm deletion to remove it from the library, or Cancel to keep it.

The files are kept in a hidden recovery folder called `.card-atlas-trash`. There is no undo button. If you delete the wrong album, stop and ask someone to help recover the files. The main collection folder itself cannot be deleted from the app.

## Export a photo list

Choose Export CSV, select the albums or boxes you want, then choose Download CSV. All albums are selected at first, so uncheck any you do not want.

The download is a list you can open in a spreadsheet app. It includes names, prices, descriptions and positions. It contains every photo in the selected albums, even if your current search shows fewer results. The image files are not included.

## Back up your collection and get help

Back up the whole collection folder, including the file called `catalog.json`. That file holds your saved details and arrangements. A CSV export is useful as a list, but it does not back up the photos or the whole collection.

If you add or remove files outside Card Atlas, choose Reload from folder so the app can find the changes. Avoid renaming or moving files outside the app if you want their saved details to stay linked to them.

When an error appears, follow the step in the message. If it keeps happening, note what you were doing and the message you saw. Someone helping you can find the technical details in the hidden `.log` folder beside your collection. Logs older than five days are cleaned automatically.

Choose Sign out when you finish on a shared computer. People can still browse and search, but they cannot change your collection without signing in.
