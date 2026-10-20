# Card Atlas

Browse image collections by album, page, row and column. Import photos in bulk, keep boxes without position numbers, and search filenames, folder names, positions and editable descriptions. Everything stays on your computer.

## Open the app

Unzip the download for your computer, then open `CardAtlas` on macOS or `CardAtlas.exe` on Windows. Your browser opens automatically. Keep the terminal window open while using the app; closing it stops the server.

The executable includes the server and browser interface. No Python, Go, database installation, or internet connection is needed to use it. It listens on `127.0.0.1`, so only the same computer can connect. The browser opens at `http://localhost:8765` by default. Port 8765 is used when available; otherwise the app selects another port and prints the address.

Builds are unsigned. macOS or Windows may display a security prompt. The macOS downloads are standalone console executables, not `.app` bundles. ZIP files preserve the executable permission on macOS.

## Administrator and search-only access

The library opens with search-only access. Visitors can browse images, read descriptions, search, and filter by album or page. They cannot import photos, resolve import conflicts, reload the folder, connect another folder, or edit descriptions and optional prices.

Click **Administrator sign in** to manage the library. The hardcoded credentials are:

- Username: `admin`
- Password: `Inventory@2026`

The server enforces these permissions for every catalog mutation. Signing in creates a session cookie with HttpOnly and SameSite protections. Sessions expire after eight hours, end on sign-out, and are invalidated when the server restarts. The credentials are fixed in `auth.go`; changing them requires rebuilding the executable.

## Import and organize photos

Sign in, click **Import photos**, enter a new album name or choose an existing one, and choose an organization type:

- **Paged album:** enter the number of pages, rows per page, and columns per row. Select multiple photos or drag and drop them into the import window, choose the import order, and review the position preview. Photos fill from left to right, then down the rows, then onto the next page. Positions start at 1 by default; choose a starting page, row and column to import into a later part of an album. Imported copies get names such as `photo-2-3-1.jpg` for page 2, row 3, column 1.
- **Box:** select photos without page, row or column counts. Copies keep their filenames, except characters unsuitable for filenames are replaced and extensions are normalized. Box photos remain unpositioned even when their original filenames end in numbers.

While signed in, you can also drop photos onto the library to open the import window. Each drop or file-picker selection adds photos to the current selection. Use **Clear selection** to start over. Drop individual photo files; folder drops are rejected. Dropped photos use the same validation, position preview, and conflict review as selected files.

Filename order uses natural sorting, so `photo2.jpg` comes before `photo10.jpg`. Reverse filename order and file picker order are also available. The preview shows the first 100 assignments; all selected files follow that order. Type a new album name or choose an existing album from the name field. When an album is selected in the library, **Import photos** starts with that album selected. Existing paged albums keep their row and column counts; increase **Pages** to add space.

Click **Review import** before uploading. Conflicts include existing filenames, occupied page/row/column positions, and duplicate filenames in the selection. Choose **Replace** or **Rename** for each conflict, or check several files and use **Replace selected** or **Rename selected**. **Select all conflicts** applies to every conflict, including those on other conflict-result pages.

**Replace** updates the existing photo and keeps its description. If a different filename replaces a physical position, the old photo is removed and its description moves to the replacement. **Rename** keeps existing photos: boxes add a suffix such as `photo (2).jpg`, while paged albums assign the new photo to the first free position outside the selected import range. The reviewed preview shows the final names and positions. If no positions are free, increase **Pages** or choose Replace. Duplicate filenames within the same selection must use Rename.

All conflicts must be resolved before importing. The app checks that destination names, sizes, and modification times still match the reviewed folder; if they changed, review again. Original photos selected for upload remain unchanged.

An album can have empty positions. Selecting more photos than its capacity is rejected. Limits: 10,000 photos, 50 MiB per photo, and 1 GiB total per import, including upload overhead. Supported extensions are JPG, JPEG, PNG, GIF, WebP, BMP, AVIF and ICO. Browser preview support varies by format. Files are accepted by extension; the app does not convert image formats.

If no folder is connected, the first import creates an `images` directory in the app settings folder, with its catalog stored inside that image root. If you connected an existing image folder first, imported albums are created inside that folder. Originals selected for upload remain unchanged. Failed imports are rolled back during normal operation, including restoration of replaced photos; keep the app running until the import finishes. An unexpected process or power interruption can leave files that require a reload. Incomplete replacement backups may remain in a hidden `.soft-inventory-import-*` directory under the image root; keep these files for recovery rather than deleting them.

The library opens with a paged album directory: 50 albums per view. Each album uses its first photo in position order (or saved box order) as a lazy-loaded cover. Use **Find an album**, page through the directory, or choose an album from the sidebar to open its cards. The sidebar is limited to 40 entries; **Browse all albums** opens the complete paged directory. The folder menu contains a small navigation subset, with metadata for albums in the current results. **Organized** view groups images by folder, page and row, with columns shown in order. Wide rows scroll horizontally. **Grid** shows cards; **Table** shows filenames, positions and descriptions together. The page filter narrows results to a physical image page. Large collections use result pages of 60 images, so a physical page or row can continue on the next result page.

Names are extracted from filenames on import and folder scans: the extension and page-row-column suffix are removed, and hyphens and underscores become spaces. Existing catalogs gain names when opened. Administrators can edit the name without renaming the image file.

Album covers use cached thumbnails and load as they approach the visible area. Thumbnails appear automatically when an album is selected or a search returns fewer than 100 photos. Use **Show thumbnails** to turn previews on or off; they load only as they approach the visible area. Click a card to load its full image; four preview requests and two thumbnail decodes can run at once. JPEG, PNG and GIF previews are resized to at most 320 pixels per side. Other formats, corrupt images, or images larger than 25 megapixels may require opening the original instead. Generated previews are cached in `.card-atlas-cache/thumbnails` inside the image root; reloads exclude that cache from the catalog. The cache is checked every 128 generated previews and pruned from over 256 MiB toward 192 MiB, so it may temporarily exceed the threshold.

Click an image to see its full preview and edit its name and description, optionally enter a price, then click **Save details**. Prices accept text such as `25.00 SEK` or `$10`, so the currency is explicit. Leave Price blank to remove it. Saved prices are visible on image cards, in the table, and in image details without signing in. Add multiple tags separated by commas, such as `Pokémon, Rare, Japanese`. Visitors can see names and tags, and search matches them too. Names and tags survive reloads and replacements; CSV exports join multiple tags with semicolons. Prices survive reloads, restarts, and replacements; a renamed copy starts without a price. Search matches every space-separated term across the relative path, description and position numbers. Typing a search resets album and physical-page filters to **All**, so it searches the whole collection by default. You can then choose an album or page to narrow the results.

### Rearrange photos and show empty positions

Sign in, open an album, and choose **Organized**. For folder imports, the app infers the page, row and column counts from positioned filenames. **Album layout** lets an administrator set the full dimensions, including blank trailing pages or rows. Photos without positions stay available under **Without positions / boxes** until placed.

Drag a photo into an empty cell to move it, or onto another photo in the same album to swap them. For a destination on another page, open the photo, choose **Move photo…**, navigate to the destination page, then click an empty cell or occupied photo. **Cancel move** leaves the arrangement untouched. In a box, dragging onto another photo swaps their display order.

The organized album view includes empty cells and completely empty pages, with at most 60 cells per result page. Positions and box order are saved in `catalog.json`; source filenames stay unchanged. Reloading the folder preserves these arrangements. Imports check the saved positions when detecting conflicts. A positional move fails without changing the catalog if its source or destination changed since the displayed view.

## Export albums as CSV

Sign in as administrator and click **Export CSV**. Select one or more albums or boxes, or use **Select all albums**, then click **Download CSV**. All albums are selected initially. The export selector also pages through 50 albums at a time and has its own name filter. Selecting all still selects every album, including albums on other pages. The export includes every present card in the selected folders, regardless of search filters or result pagination. Visitors without an administrator login cannot export.

The download is named `CardAtlas-export.csv` and contains Name, Filename, Album, Page, Row, Column, Price, Description, Tags, and Relative path columns. Missing positions and prices are blank. Files are ordered by album and numeric position. Images themselves are not included.

CSV fields are quoted as needed and encoded as UTF-8 with a byte-order mark for Excel. Text beginning with spreadsheet formula markers is prefixed with an apostrophe so spreadsheet applications treat it as text.

## Connect existing folders and keep data

Instead of importing, click **Connect folder** and enter a full local folder path. Subfolders are included and images stay in place. Names ending in `-page-row-column` are interpreted as positions, for example `album/part-name-12-3-2.png`. Other filenames work without position labels. Symbolic-link directories are not followed; linked files outside the connected folder are excluded.

Click **Reload from folder** after adding, changing or removing files outside the app. This rescans the entire connected folder and refreshes image previews. Descriptions survive rescans and temporary image removal. Moving or renaming an image changes its identity, so descriptions do not follow a new path automatically.

The catalog is saved as `catalog.json` directly inside the selected image root folder. It contains card names, descriptions, prices, tags, saved positions, box order, album dimensions and box types. Back up the image root folder to keep images and metadata together. A small `~/.soft-inventory/library-location.json` file remembers the selected root; it contains only the folder location. Reconnecting a folder loads its existing catalog. Moving the whole folder and reconnecting it preserves relative image paths and metadata. Older home-folder catalogs are copied into the image root on reload or startup; the original file remains as a backup. One catalog connects to one root folder. Close one app instance before opening another instance that uses the same catalog.

Advanced launch options:

```sh
./CardAtlas --data ./another-catalog --folder "/path/to/images"
./CardAtlas --no-browser --port 9000
```

On Windows, run `CardAtlas.exe` with the same flags. `--data` chooses a separate settings directory (or an image root already containing a catalog), `--folder` scans a folder at startup, `--no-browser` leaves browser opening to you, and `--port 0` chooses an available port.

If you used the earlier Python version, stop it and run the one-time migration before starting this version:

```sh
python3 migrate_catalog.py
```

The migration reads the old SQLite file without modifying it, imports descriptions and paths into `catalog.json`, and refuses to overwrite an existing JSON catalog. Use `--data /path/to/catalog-directory` for a nondefault location. Python is needed only for this migration.

## Build macOS and Windows downloads from a Mac

Building requires Go 1.24 or newer. The server uses the Go standard library with no external dependencies or C compiler. The interface is embedded into each executable.

Run from the project directory:

```sh
go test ./...
go run ./cmd/build
```

The build creates standalone executables and ZIP files in `dist`:

| Download | Target |
| --- | --- |
| `CardAtlas-macOS-AppleSilicon.zip` | Macs with Apple Silicon |
| `CardAtlas-macOS-Intel.zip` | Intel Macs |
| `CardAtlas-Windows-x64.zip` | Windows x64 |
| `CardAtlas-Windows-ARM64.zip` | Windows ARM64 |

All four builds run on your Mac. A Windows VM and GitHub Actions are not required. The optional GitHub workflow runs the same build command. Windows executables are cross-compiled; launch behavior still needs validation on Windows. These builds are unsigned; distribution with signing or notarization requires the relevant developer credentials.

To run the source during development:

```sh
go run . --no-browser
```

## Tests

Run the full suite with race detection:

```sh
go test -race ./...
```

Run the focused unit tests or real HTTP integration tests separately:

```sh
go test -run '^TestUnit' ./...
go test -run '^TestIntegration' .
```

Unit tests check position parsing, portable filenames, album dimensions, import starting positions, combined search filters, description limits, catalog format validation, and ZIP contents and executable permissions. The existing catalog and handler tests also cover authentication, rollback, symlink boundaries, and import conflict planning.

Integration tests start real HTTP servers on temporary localhost ports and use HTTP clients with cookie jars. They cover administrator login, public read-only search, multipart imports, image serving and conditional caching, mixed Replace/Rename choices, folder reloads, restart persistence, session invalidation, and interrupted-upload cleanup and retry. All catalogs and photos created by the tests use temporary directories that Go removes after each test.

The drag-and-drop selection tests use Node.js 22 or newer and need no npm packages:

```sh
node --test tests/frontend.test.cjs
```

They check adding photos across drops and file-picker selections, natural ordering, clearing the selection, invalidating conflict reviews, folder rejection, and access and upload guards. The Tests workflow runs these checks alongside the Go suite.

To collect statement coverage:

```sh
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

The Tests workflow runs the suite on macOS, Windows and Linux on pushes and pull requests. Its coverage reports are attached to each workflow run. Running the local tests does not require GitHub.

### Large-collection checks

The server caches sorted metadata, album counts, and normalized search text between edits. Album-specific queries use an index for that folder. Searches are debounced and obsolete browser requests are canceled. The scaling tests use 50,000 cards across 2,000 albums and verify bounded responses, index invalidation, thumbnail caching, and cache exclusion during scans.

Run the repeatable metadata benchmarks:

```sh
go test -run '^$' -bench '^BenchmarkLargeCatalog' -benchmem .
```

These benchmarks measure a warm in-memory catalog. They exclude startup, image decoding, disk scans, and full catalog saves. JSON metadata still loads into memory at startup and is rewritten on edits; the view optimizations do not change that storage model.
