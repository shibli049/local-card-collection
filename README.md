# Soft Inventory

Browse image collections by album, page, row and column. Import photos in bulk, keep boxes without position numbers, and search filenames, folder names, positions and editable descriptions. Everything stays on your computer.

## Open the app

Unzip the download for your computer, then open `SoftInventory` on macOS or `SoftInventory.exe` on Windows. Your browser opens automatically. Keep the terminal window open while using the app; closing it stops the server.

The executable includes the server and browser interface. No Python, Go, database installation, or internet connection is needed to use it. It listens on `127.0.0.1`, so only the same computer can connect. Port 8765 is used when available; otherwise the app selects another port and prints the address.

Builds are unsigned. macOS or Windows may display a security prompt. The macOS downloads are standalone console executables, not `.app` bundles. ZIP files preserve the executable permission on macOS.

## Administrator and search-only access

The library opens with search-only access. Visitors can browse images, read descriptions, search, and filter by album or page. They cannot import photos, resolve import conflicts, reload the folder, connect another folder, or edit descriptions.

Click **Administrator sign in** to manage the library. The hardcoded credentials are:

- Username: `admin`
- Password: `Inventory@2026`

The server enforces these permissions for every catalog mutation. Signing in creates a session cookie with HttpOnly and SameSite protections. Sessions expire after eight hours, end on sign-out, and are invalidated when the server restarts. The credentials are fixed in `auth.go`; changing them requires rebuilding the executable.

## Import and organize photos

Sign in, click **Import photos**, enter a new album name or choose an existing one, and choose an organization type:

- **Paged album:** enter the number of pages, rows per page, and columns per row. Select multiple photos, choose the import order, and review the position preview. Photos fill from left to right, then down the rows, then onto the next page. Positions start at 1 by default; choose a starting page, row and column to import into a later part of an album. Imported copies get names such as `photo-2-3-1.jpg` for page 2, row 3, column 1.
- **Box:** select photos without page, row or column counts. Copies keep their filenames, except characters unsuitable for filenames are replaced and extensions are normalized. Box photos remain unpositioned even when their original filenames end in numbers.

Filename order uses natural sorting, so `photo2.jpg` comes before `photo10.jpg`. Reverse filename order and file picker order are also available. The preview shows the first 100 assignments; all selected files follow that order. Type a new album name or choose an existing album from the name field. When an album is selected in the library, **Import photos** starts with that album selected. Existing paged albums keep their row and column counts; increase **Pages** to add space.

Click **Review import** before uploading. Conflicts include existing filenames, occupied page/row/column positions, and duplicate filenames in the selection. Choose **Replace** or **Rename** for each conflict, or check several files and use **Replace selected** or **Rename selected**. **Select all conflicts** applies to every conflict, including those on other conflict-result pages.

**Replace** updates the existing photo and keeps its description. If a different filename replaces a physical position, the old photo is removed and its description moves to the replacement. **Rename** keeps existing photos: boxes add a suffix such as `photo (2).jpg`, while paged albums assign the new photo to the first free position outside the selected import range. The reviewed preview shows the final names and positions. If no positions are free, increase **Pages** or choose Replace. Duplicate filenames within the same selection must use Rename.

All conflicts must be resolved before importing. The app checks that destination names, sizes, and modification times still match the reviewed folder; if they changed, review again. Original photos selected for upload remain unchanged.

An album can have empty positions. Selecting more photos than its capacity is rejected. Limits: 10,000 photos, 50 MiB per photo, and 1 GiB total per import, including upload overhead. Supported extensions are JPG, JPEG, PNG, GIF, WebP, BMP, AVIF and ICO. Browser preview support varies by format. Files are accepted by extension; the app does not convert image formats.

The first import creates an `images` directory beside the local catalog. If you connected an existing image folder first, imported albums are created inside that folder. Originals selected for upload remain unchanged. Failed imports are rolled back during normal operation, including restoration of replaced photos; keep the app running until the import finishes. An unexpected process or power interruption can leave files that require a reload. Incomplete replacement backups may remain in a hidden `.soft-inventory-import-*` directory under the image root; keep these files for recovery rather than deleting them.

Choose an album from the sidebar or folder menu. **Organized** view groups images by folder, page and row, with columns shown in order. Wide rows scroll horizontally. **Grid** shows cards; **Table** shows filenames, positions and descriptions together. The page filter narrows results to a physical image page. Large collections use result pages of 60 images, so a physical page or row can continue on the next result page.

Click an image to see its full preview and edit its description, then click **Save description**. Search matches every space-separated term across the relative path, description and position numbers. Folder and physical-page filters combine with search.

## Connect existing folders and keep data

Instead of importing, click **Connect folder** and enter a full local folder path. Subfolders are included and images stay in place. Names ending in `-page-row-column` are interpreted as positions, for example `album/part-name-12-3-2.png`. Other filenames work without position labels. Symbolic-link directories are not followed; linked files outside the connected folder are excluded.

Click **Reload from folder** after adding, changing or removing files outside the app. This rescans the entire connected folder and refreshes image previews. Descriptions survive rescans and temporary image removal. Moving or renaming an image changes its identity, so descriptions do not follow a new path automatically.

The catalog is saved in `~/.soft-inventory/catalog.json`, under your user home folder on macOS or Windows. Album dimensions and box types are saved there too. Back up this file and your image folder together. One catalog connects to one root folder. Close one app instance before opening another instance that uses the same catalog.

Advanced launch options:

```sh
./SoftInventory --data ./another-catalog --folder "/path/to/images"
./SoftInventory --no-browser --port 9000
```

On Windows, run `SoftInventory.exe` with the same flags. `--data` chooses a separate catalog directory, `--folder` scans a folder at startup, `--no-browser` leaves browser opening to you, and `--port 0` chooses an available port.

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
| `SoftInventory-macOS-AppleSilicon.zip` | Macs with Apple Silicon |
| `SoftInventory-macOS-Intel.zip` | Intel Macs |
| `SoftInventory-Windows-x64.zip` | Windows x64 |
| `SoftInventory-Windows-ARM64.zip` | Windows ARM64 |

All four builds run on your Mac. A Windows VM and GitHub Actions are not required. The optional GitHub workflow runs the same build command. Windows executables are cross-compiled; launch behavior still needs validation on Windows. These builds are unsigned; distribution with signing or notarization requires the relevant developer credentials.

To run the source during development:

```sh
go run . --no-browser
```
