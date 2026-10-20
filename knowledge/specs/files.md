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
