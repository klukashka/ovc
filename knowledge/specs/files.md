# Files specification

Behavior for FR-14 and every upload in OVC. Storage is S3-compatible object storage hosted in Russia ([ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md)). Integration tests use a local S3-compatible server; unit and contract tests may use the in-memory storage fake.

## Contexts and limits

| Context | Who uploads | Scope given at upload | Allowed kinds | Per-file limit | Per item |
|---|---|---|---|---|---|
| `avatar` | any user | — | JPEG, PNG, WebP | 5 MB | 1 |
| `prompt_image` | tutor | — (own library) | JPEG, PNG, WebP, GIF | 5 MB | 3 per prompt |
| `library` | tutor | — (own library) | images, PDF, office documents, audio, MP4 video | 100 MB | 1 per material |
| `chat` | channel members | the channel | images, PDF, office documents, audio, plain text | 25 MB | 10 per message |
| `answer` | student | the submission and question | the kinds allowed by the question | 25 MB | up to the question's maximum |
| `corrected` | tutor | the submission and question | images, PDF, office documents, audio | 25 MB | 5 per answer |

- **Kinds** map to an explicit allowlist of media types:
  - images: JPEG, PNG, WebP, GIF;
  - PDF;
  - office documents: DOCX, XLSX, PPTX, ODT, ODS, ODP;
  - audio: MP3, M4A, OGG, WAV;
  - video: MP4;
  - plain text.
- **Type detection** reads the content, not the client's claim:
  - ordinary types by magic bytes;
  - office documents by inspecting the container entries that identify each format; a generic ZIP is refused, and so are documents that contain macros (`vbaProject.bin`) or external relationships.

  The detected type must agree with the extension; a mismatch is refused with `unsupported_media_type`.
- **No archives and no executables.** Malware scanning is not in the MVP and is recorded as a threat-model limitation.

## Upload flow

1. **Request:** `POST /api/v1/uploads?context=<context>&scope=<scope>`.
   - The scope is required where the table names one: `channel:<channel id>` for chat, and `submission:<submission id>:<question id>` for answer and corrected uploads.
   - The caller must be allowed to write to it, and the scope decides which tutor's quota is charged.
   - The scope is stored on the attachment and checked again at binding.
2. **Streaming:**
   - the server parses the multipart body as a stream without spooling to disk, and writes it to storage as an S3 multipart upload with 8 MiB parts;
   - it counts bytes and computes SHA-256 as it goes, and aborts with `payload_too_large` the moment the limit is passed;
   - **type detection:** ordinary types are detected from the first bytes. For office formats the server keeps the last 1 MiB of the stream in a ring buffer and reads the container's central directory from it before completing the upload; a document whose central directory does not fit is refused;
   - **database sessions:** the request uses one short session before streaming (authorization, scope and quota pre-check) and another after it (creating the attachment row); none is held during the transfer;
   - **memory** is bounded by (part size + tail buffer) × concurrent uploads: at most 288 MiB per instance.
3. **Failure:** on rejection, error or client disconnect, the server aborts the multipart upload. A bucket lifecycle rule also aborts incomplete multipart uploads after one day.
4. **Concurrency:** at most 4 concurrent uploads per user and 32 per API instance; beyond that the server refuses with `rate_limited`.
5. **Response:** `{id, filename, media_type, size_bytes}`. The storage key is never returned.
6. **Pending state:** the attachment is `pending`, owned by the uploader and invisible to others, until a create or update request binds it to an item: a message, an answer, a review, a material, a prompt or an avatar.
7. **Binding** checks that:
   - the uploader is the caller;
   - the context and scope match the item;
   - the item's count limit holds;
   - the quota still holds.
8. **Unbound pending attachments** are deleted after 24 hours.

## Avatars

Slice 2 implements only the `avatar` context, and it departs from the general flow above in these ways:

- **Buffered, not streamed to storage:** the part is read into memory up to 5 MB plus one byte (more answers `payload_too_large`), because an avatar fits one S3 request; the request body is limited to 5 MB plus 64 KiB and has a 60-second read deadline.
- **Type:** JPEG, PNG or WebP by magic bytes, with an agreeing extension (`.jpg`/`.jpeg`, `.png`, `.webp`); `image.DecodeConfig` must report the same format and at most 4096 × 4096 pixels before anything is decoded.
- **Metadata removal:** the image is decoded and encoded again, which drops EXIF, XMP and text chunks: JPEG stays JPEG (quality 90), PNG and WebP become PNG. The stored file is named `avatar.jpg` or `avatar.png`, so no client file name is kept, and an encoded result above 10 MB is refused as `payload_too_large` the moment the encoder passes that size. 16-bit and CMYK images are refused as `unsupported_media_type`. Decoding memory has a budget of 256 MiB per instance: each decode reserves width × height × 8 bytes for PNG and WebP (the decoded image and the encoder's working copy) or × 16 bytes for JPEG (progressive coefficients for three components plus the image), and WebP reserves the larger of 8 bytes per pixel and 5 bytes per pixel plus 132 MiB, because a lossless image may declare up to 2600 Huffman table groups (about 131 MiB) whatever its size, so the largest JPEG fills the budget alone and only one WebP decodes at a time; a user decodes at most one image at a time, and an upload that does not fit answers `503 service_unavailable` with `Retry-After: 5`. Outside that budget, each of the at most 32 concurrent uploads holds its body (up to 5 MB) and, while encoding, its result (up to 10 MB).
- **Order:** the attachment row is created first, in a transaction that locks the user row and checks the pending limit again, and only then is the object stored; a failed store marks the row deleted and queues its deletion, and after a successful store the row is locked again: if a purge removed it meanwhile, a `storage_delete` row is queued for the object in that transaction and the upload fails, so no object exists without a row.
- **Limits per user:** at most 3 pending avatar uploads that have not passed their 24-hour expiry (an expired one stops counting before the hourly expiry job removes it), 20 avatar uploads per hour (`429 rate_limited` with `Retry-After`), and the general 4 concurrent uploads per user and 32 per instance.
- **Binding:** `PATCH /users/me` with `avatar_id` binds the caller's own pending avatar upload that has not passed its 24-hour expiry; the previous avatar loses its reference, becomes `deleted` and gets a `storage_delete` outbox row in the same transaction; `null` removes the avatar the same way. The reference is the only record of the avatar.
- **Serving:** `GET /api/v1/avatars/{id}` reads the object through the API (the CSP stays `'self'`) and answers only for an attachment with a live avatar reference whose owner is active or unverified, with `Content-Type` from detection, `X-Content-Type-Options: nosniff`, `Content-Disposition: inline` without a file name, `Cache-Control: private, max-age=86400`, `Content-Security-Policy: default-src 'none'; sandbox` and `Cross-Origin-Resource-Policy: same-origin`; everything else is `404`. The body is streamed from storage with `Content-Length` set from the stored size, never held in memory whole; an instance serves at most 32 avatars at once and answers `503 service_unavailable` with `Retry-After: 1` beyond that; the attachment lookup and the storage read share a 30-second deadline, and the response write is bounded by the request's write deadline; the last byte is sent only after the object is confirmed to end exactly at its recorded size, so an object shorter or longer than recorded always reaches the client short of its `Content-Length` before the connection is aborted, and is reported as `avatars.get.failed` with kind `size_mismatch`.
- **Deletion:** a `storage_delete` outbox row carries the attachment identifier and its storage key; the worker's lane deletes the object and then the attachment row, and skips the row if the attachment exists and is not `deleted`; an account purge deletes the rows first, so the key in the payload is what lets the object follow; the hourly `uploads.expire` job marks pending uploads older than 24 hours as `deleted` and queues their deletion, in batches of 200.

## References

- An attachment is shared through **references**: rows that say "this item uses this attachment", for a message, an answer, a review item, a material, a template, an issue snapshot, a lesson or an avatar.
- **Duplicating a template** or issuing a snapshot adds references; it never copies bytes.
- **Downloads** are authorized through any reference whose item the caller can see. Review-item references point to a specific review revision, which students can see only once published ([answer visibility](homework.md#answer-visibility)).
- **An attachment is deleted from storage**, through an outbox `storage_delete` row, when its last reference is removed.

## Downloads

- **Private files:** `GET /api/v1/attachments/{id}` authorizes the caller through the attachment's references; an unauthorized caller gets `not_found`.
- **On success** it returns `{url, expires_at}` with a signed storage URL valid for 5 minutes, which the client puts in `src` or `href`. Media elements cannot send an `Authorization` header, so the API never redirects. The storage object is served with:
  - `Content-Type` set from the detected type;
  - `X-Content-Type-Options: nosniff`;
  - `Content-Disposition: attachment` with the file name in RFC 6266 form: an ASCII fallback with CR, LF and quotes removed, plus `filename*` in UTF-8 encoding;
  - for images used as previews, `inline` instead of `attachment`.
- **Avatars** are public by design: `GET /api/v1/avatars/{id}` serves them without authentication, so the invitation page can show the tutor.
- **Previews** use the original image, constrained in size by the client. Generated thumbnails are a later optimisation.

## Quotas

- **Every tutor has two quotas:** 5 GB of files and 50 GB of recordings by default. The operator changes them with `ovc-admin tutors set-quota`.
- **The file quota counts each attachment once per tutor**, when it is referenced from:
  - the tutor's library, templates, prompts, issue snapshots and lessons;
  - the tutor's direct and group channels, whoever sent the file;
  - student answer files and corrected files in the tutor's issues.
- **Charging is incremental:** a charge row per tutor and attachment is created with the first reference and removed with the last one. Usage changes by the attachment's size only then, so no reference change rescans the tutor's files.
- **Avatars** count against no quota.
- **Over the file quota:** an upload or binding is refused with `quota_exceeded`, and a student sees "your tutor's storage is full".
- **Recording quota:** recording usage changes when a recording becomes available, expires or is deleted ([recordings](realtime-and-video.md#recordings)).
- **Usage** is shown in the tutor's settings and updated in the same transaction as each charge change.

## Deletion

- **Account purge** removes every reference the account's items held, which deletes files that are no longer referenced ([account deletion](accounts.md#account-deletion)).
- **Library materials** referenced by issued snapshots or lessons stay in storage until those references are gone ([library](homework.md#library)).
