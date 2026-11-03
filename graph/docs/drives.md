---
name: Drives
resource: drives
order: 12
---
The folders and files on a drive. A drive is a document library of a site, the OneDrive of a user, or the drive of a group (its team site). Every way to a drive gives the same items:

- `/drives/{drive-id}` (the ID is `b!` and 64 characters),
- `/users/{id}/drive` and `/me/drive` (the OneDrive of a user),
- `/groups/{id}/drive` (the default drive of the team site of a Microsoft 365 group),
- `/sites/{id}/drive` and `/sites/{id}/drives/{drive-id}` (see the Sites page).

Below each of them are `/root`, `/root/children`, `/items/{item-id}`, `/items/{item-id}/children`, and `/items/{item-id}/content`. An item is also found by its path: `/root:/Reports/2026.docx`, with `:/children` and `:/content` after it. A file is put in a folder by name with `/items/{folder-id}:/name.txt:/content`, and with `/root:/folder/name.txt:/content`.

Make the drives of a site, and put folders and files on them, on the Sites page of the portal.

An item has an `id` of 34 characters (`01` and 32 letters and digits), a `name`, a `size`, an `eTag` (every change) and a `cTag` (only a change of the content), a `parentReference`, and either `folder` (with `childCount`) or `file` (with `mimeType`, from the end of the name). The size of a folder is the size of what is in it. A name cannot have `" * : < > ? / \ |`, and cannot start or end with a space, or end with a dot. Two items of a folder cannot have the same name (the case does not matter).

The content of a file is kept in `content/` in the data folder, by its hash (or in memory, when dollycli has no data folder). A file that the seed or the portal made with a size and no text has no content of its own: it reads as a sentence, again and again, as long as its size. A file can be up to 4 MB. dollycli has no upload session for a larger file.

## GET /v1.0/drives/{drive}/items/{item}/content
A `302` to the address in `@microsoft.graph.downloadUrl` (the Learn page "Download the contents of a DriveItem" says so). The address needs no token. It answers `Range`. When the content changes, the address changes, and the old one gives `404`.

## PUT /v1.0/drives/{drive}/root:/{rest...}
`/root:/Reports/2026.docx:/content` puts a file by its path. The folders of the path that are not there are made. The answer is `201` for a new file, and `200` when a file with that name was there (its content is replaced). `?@microsoft.graph.conflictBehavior=fail` answers `409 nameAlreadyExists` for a name that is there, and `rename` makes `2026 1.docx`.

## POST /v1.0/drives/{drive}/root/children
A folder: `{"name":"Reports","folder":{}}`. An empty file: `{"name":"a.txt","file":{}}`. `@microsoft.graph.conflictBehavior` is `fail` (the default, `409 nameAlreadyExists`), `rename` (`Reports 1`), or `replace` (the folder or file that has the name is deleted first).

## PATCH /v1.0/drives/{drive}/items/{item}
`name` renames, and `parentReference: {"id": "<folder-id>"}` moves. A name that the folder has fails with `409`. A folder cannot move into itself.

### What was checked against the real Graph, and what was not
**Checked** (the proxy of Graph Explorer, 2026-11-03): `GET /drives/b!nothing` and `GET /drives/b!nothing/root` give `400 invalidRequest`, `The provided drive id appears to be malformed, or does not represent a valid drive.` Items on it give `400 invalidRequest`, `Invalid request`. `GET /me/drive/root:/docs` gives `404 itemNotFound`, `Item not found`. `GET /users/{unknown}/drive` gives `404 ResourceNotFound`, `User not found`. The sample user has no drive, so no answer for a real item was seen.

**From the Learn pages** (not tried): the properties of an item, the calls above and their paths, `conflictBehavior`, the `302` of the content and the `Range` of the download address, rename and move with `PATCH`, and the 250 MB that a simple upload can take (dollycli takes 4 MB).

**A guess:** the form of a drive ID that counts as valid (an ID of that form that is not there gives `404 itemNotFound`), `nameAlreadyExists` and its text, the error for a name with a character that is not allowed, `413 requestEntityTooLarge` for a file above the limit, the children of a file (`400`), the content of a folder (`400`), deleting or changing the root folder (`400`), a file where a folder of a path should be (`409`), the folders that a put by path makes, `$skiptoken` in the paging (dollycli pages as the other resources do), and the quota, the owner, and the `createdBy` of an item.
