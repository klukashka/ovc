const bytesPerMegabyte = 1_000_000;
const megabytesPerGigabyte = 1_000;
const smallestShownMegabytes = 0.1;

type StorageUnit = 'megabyte' | 'gigabyte';

interface StorageAmount {
  readonly value: number;
  readonly unit: StorageUnit;
}

function roundedToTenths(value: number): number {
  return Math.round(value * 10) / 10;
}

function storageAmount(bytes: number): StorageAmount {
  const megabytes = bytes / bytesPerMegabyte;
  if (roundedToTenths(megabytes) >= megabytesPerGigabyte) {
    return { value: megabytes / megabytesPerGigabyte, unit: 'gigabyte' };
  }
  if (bytes > 0 && megabytes < smallestShownMegabytes) {
    return { value: smallestShownMegabytes, unit: 'megabyte' };
  }
  return { value: megabytes, unit: 'megabyte' };
}

export function formatStorage(bytes: number, language: string): string {
  const amount = storageAmount(bytes);
  return new Intl.NumberFormat(language, {
    style: 'unit',
    unit: amount.unit,
    unitDisplay: 'short',
    maximumFractionDigits: 1,
  }).format(amount.value);
}
