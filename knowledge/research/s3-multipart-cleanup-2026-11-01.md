# Multipart completion and cleanup evidence

Checked: 2026-11-01. Scope: published Amazon S3 semantics and the limits of conclusions for an S3-compatible provider. No production-provider experiment was performed. Local integration tests prove named local interleavings, not every compatible provider's behavior.

## Published guarantees

[CompleteMultipartUpload](https://docs.aws.amazon.com/AmazonS3/latest/API/API_CompleteMultipartUpload.html) assembles parts after receipt. Processing may take minutes, and an initial successful HTTP status can contain an error in its body. SDKs interpret that body and can retry according to configuration. A client deadline is therefore not documented evidence that the remote operation stopped. OVC disables SDK retries for its single Create and Complete invocations and requires a confirmed durable transition before issuing Complete.

[AbortMultipartUpload](https://docs.aws.amazon.com/AmazonS3/latest/API/API_AbortMultipartUpload.html) warns that concurrent part requests may still succeed and another abort can be necessary. Its documentation does not provide a terminal fence for a previously submitted Complete. Treating Abort success or `NoSuchUpload` as proof against a late completed object would be an inference unsupported by this source.

[Incomplete multipart lifecycle cleanup](https://docs.aws.amazon.com/AmazonS3/latest/userguide/mpu-abort-incomplete-mpu-lifecycle-config.html) applies to incomplete uploads, including existing ones, once they become eligible by age. It does not delete completed objects. Eligibility after the configured day is not a precise physical-erasure deadline.

## Application consequence

A durable cleanup owner must precede storage work and survive account/attachment purge. An ambiguous Complete remains unresolved after a successful Delete, because a later object cannot be ruled out. Finite automatic cleanup and explicit local operator recovery preserve evidence; they cannot guarantee eventual physical erasure under permanent failure or unresolved remote work. No elapsed time, worker lease expiry, process shutdown or absent object is a provider fence.

The configured local S3 service remains the integration target. Its successful tests do not certify the deployment provider. Provider-specific reconciliation requires separate documented evidence and an approved operational decision; this change adds no provider-specific forget operation. See [ADR-0019](../../docs/decisions/0019-durable-upload-cleanup.md).

## Installed client options

The implementation inspected the locally installed AWS SDK for Go v2 S3 module at v1.114.3. Its `Options.RetryMaxAttempts` applies to a single operation when passed through the operation option function and wraps the configured retryer to that attempt limit. CreateMultipartUpload and CompleteMultipartUpload are called with `RetryMaxAttempts = 1`; other operations retain their existing configuration. This is a source-level configuration check, not a claim that a provider cannot continue a request after local cancellation.
