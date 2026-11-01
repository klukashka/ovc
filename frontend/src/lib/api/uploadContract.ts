export interface ContractUploadFile {
  readonly name: string;
  readonly mediaType: string;
  readonly bytes: readonly number[];
}

export interface UploadContractSubject {
  readonly upload: (
    file: ContractUploadFile,
    scope: 'member' | 'outside',
    signal: AbortSignal,
    progress: (loaded: number, total: number | null) => void,
  ) => Promise<Response>;
}

export const contractUploadFile: ContractUploadFile = {
  name: 'lesson-notes.txt',
  mediaType: 'text/plain',
  bytes: [...new TextEncoder().encode('A lesson note.\nСтрока для проверки UTF-8.\n'.repeat(2_048))],
};

function check(condition: boolean, message: string): void {
  if (!condition) throw new Error(`upload contract: ${message}`);
}

async function multipart(subject: UploadContractSubject): Promise<void> {
  const progress: { loaded: number; total: number | null }[] = [];

  const response = await subject.upload(contractUploadFile, 'member', new AbortController().signal, (loaded, total) => {
    progress.push({ loaded, total });
  });
  const file: unknown = await response.json();

  check(response.status === 201, 'a valid multipart upload was refused');
  check(
    typeof file === 'object' && file !== null && 'id' in file && typeof file.id === 'string' && file.id.length > 0,
    'the uploaded file has no identifier',
  );
  check(
    typeof file === 'object' && file !== null && 'filename' in file && file.filename === contractUploadFile.name,
    'the uploaded filename changed',
  );
  check(
    typeof file === 'object' &&
      file !== null &&
      'media_type' in file &&
      file.media_type === contractUploadFile.mediaType,
    'the uploaded media type changed',
  );
  check(
    typeof file === 'object' &&
      file !== null &&
      'size_bytes' in file &&
      file.size_bytes === contractUploadFile.bytes.length,
    'the multipart payload size changed',
  );
  check(progress.length > 0, 'a completed upload reported no progress');
  check(
    progress.every(
      (sample, index) =>
        sample.loaded >= 0 &&
        sample.loaded >= (progress[index - 1]?.loaded ?? 0) &&
        (sample.total === null || (sample.total > 0 && sample.loaded <= sample.total)),
    ),
    'upload progress is not bounded and nondecreasing',
  );
}

async function preaborted(subject: UploadContractSubject): Promise<void> {
  const signal = AbortSignal.timeout(0);
  await new Promise<void>((resolve) => {
    signal.addEventListener(
      'abort',
      () => {
        resolve();
      },
      { once: true },
    );
  });
  const progress: number[] = [];

  const result = await subject
    .upload(contractUploadFile, 'member', signal, (loaded) => {
      progress.push(loaded);
    })
    .then(
      () => 'resolved',
      (error: unknown) => (error instanceof Error ? error.name : 'untyped'),
    );

  check(result === 'TimeoutError', 'a pre-aborted upload did not preserve the signal reason');
  check(progress.length === 0, 'a pre-aborted upload reported progress');
}

async function deniedScope(subject: UploadContractSubject): Promise<void> {
  const signal = new AbortController().signal;

  const response = await subject.upload(contractUploadFile, 'outside', signal, () => undefined);
  const body: unknown = await response.json();

  check(response.status === 404, 'another tutor channel did not answer not_found');
  check(
    typeof body === 'object' &&
      body !== null &&
      'error' in body &&
      typeof body.error === 'object' &&
      body.error !== null &&
      'code' in body.error &&
      body.error.code === 'not_found',
    'the scope refusal lost its structured code',
  );
}

export const uploadContract = { multipart, preaborted, deniedScope };
