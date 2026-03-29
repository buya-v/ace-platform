import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { buildCSVString, exportToCSV, CsvColumn } from '../utils/export';

describe('buildCSVString', () => {
  const columns: CsvColumn[] = [
    { key: 'name', header: 'Name' },
    { key: 'age', header: 'Age' },
    { key: 'city', header: 'City' },
  ];

  it('returns only header row for empty data', () => {
    const result = buildCSVString(columns, []);
    expect(result).toBe('Name,Age,City');
  });

  it('generates CSV for a single row', () => {
    const data = [{ name: 'Alice', age: 30, city: 'NYC' }];
    const result = buildCSVString(columns, data);
    expect(result).toBe('Name,Age,City\r\nAlice,30,NYC');
  });

  it('generates CSV for multiple rows', () => {
    const data = [
      { name: 'Alice', age: 30, city: 'NYC' },
      { name: 'Bob', age: 25, city: 'LA' },
    ];
    const result = buildCSVString(columns, data);
    const lines = result.split('\r\n');
    expect(lines).toHaveLength(3);
    expect(lines[0]).toBe('Name,Age,City');
    expect(lines[1]).toBe('Alice,30,NYC');
    expect(lines[2]).toBe('Bob,25,LA');
  });

  it('wraps values containing commas in double quotes', () => {
    const data = [{ name: 'Smith, John', age: 40, city: 'LA' }];
    const result = buildCSVString(columns, data);
    expect(result).toContain('"Smith, John"');
  });

  it('escapes double quotes inside values', () => {
    const data = [{ name: 'He said "hello"', age: 20, city: 'SF' }];
    const result = buildCSVString(columns, data);
    expect(result).toContain('"He said ""hello"""');
  });

  it('wraps values containing newlines', () => {
    const data = [{ name: 'Line1\nLine2', age: 1, city: 'X' }];
    const result = buildCSVString(columns, data);
    expect(result).toContain('"Line1\nLine2"');
  });

  it('handles null and undefined values as empty strings', () => {
    const data = [{ name: null, age: undefined, city: 'LA' }];
    const result = buildCSVString(columns, data as any);
    const row = result.split('\r\n')[1];
    expect(row).toBe(',,LA');
  });

  it('handles missing keys as empty strings', () => {
    const data = [{ name: 'Alice' }];
    const result = buildCSVString(columns, data as any);
    const row = result.split('\r\n')[1];
    expect(row).toBe('Alice,,');
  });

  it('escapes headers that contain commas', () => {
    const cols: CsvColumn[] = [{ key: 'x', header: 'A, B' }];
    const result = buildCSVString(cols, [{ x: 1 }]);
    expect(result.startsWith('"A, B"')).toBe(true);
  });
});

describe('exportToCSV', () => {
  let createObjectURLMock: ReturnType<typeof vi.fn>;
  let revokeObjectURLMock: ReturnType<typeof vi.fn>;
  let clickMock: ReturnType<typeof vi.fn>;
  let appendChildSpy: ReturnType<typeof vi.fn>;
  let removeChildSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    createObjectURLMock = vi.fn().mockReturnValue('blob:mock-url');
    revokeObjectURLMock = vi.fn();
    clickMock = vi.fn();

    globalThis.URL.createObjectURL = createObjectURLMock as typeof URL.createObjectURL;
    globalThis.URL.revokeObjectURL = revokeObjectURLMock;

    appendChildSpy = vi.fn().mockImplementation((node: Node) => node);
    removeChildSpy = vi.fn().mockImplementation((node: Node) => node);
    document.body.appendChild = appendChildSpy as unknown as typeof document.body.appendChild;
    document.body.removeChild = removeChildSpy as unknown as typeof document.body.removeChild;

    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      if (tag === 'a') {
        return {
          href: '',
          download: '',
          style: { display: '' },
          click: clickMock,
        } as unknown as HTMLAnchorElement;
      }
      return document.createElement(tag);
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('creates a Blob and passes it to createObjectURL', () => {
    const columns: CsvColumn[] = [{ key: 'id', header: 'ID' }];
    const data = [{ id: 1 }];

    exportToCSV(columns, data, 'test.csv');

    expect(createObjectURLMock).toHaveBeenCalledOnce();
    const blob = createObjectURLMock.mock.calls[0][0] as Blob;
    expect(blob).toBeInstanceOf(Blob);
    expect(blob.type).toBe('text/csv;charset=utf-8;');
  });

  it('creates a temporary link, clicks it, and cleans up', () => {
    const columns: CsvColumn[] = [{ key: 'id', header: 'ID' }];
    const data = [{ id: 1 }];

    exportToCSV(columns, data, 'export.csv');

    expect(createObjectURLMock).toHaveBeenCalledOnce();
    expect(appendChildSpy).toHaveBeenCalledOnce();
    expect(clickMock).toHaveBeenCalledOnce();
    expect(removeChildSpy).toHaveBeenCalledOnce();
    expect(revokeObjectURLMock).toHaveBeenCalledWith('blob:mock-url');
  });

  it('sets the download attribute to the given filename', () => {
    const columns: CsvColumn[] = [{ key: 'x', header: 'X' }];
    exportToCSV(columns, [], 'my-data.csv');

    const link = appendChildSpy.mock.calls[0][0] as any;
    expect(link.download).toBe('my-data.csv');
    expect(link.href).toBe('blob:mock-url');
  });
});
