const maxBufferedCharacters = 32 * 1024 * 1024;

class IncrementalUtf8Decoder {
  private pending = new Uint8Array(0);

  decode(chunk: Uint8Array): string {
    const bytes = new Uint8Array(this.pending.length + chunk.length);
    bytes.set(this.pending);
    bytes.set(chunk, this.pending.length);
    let output = "";
    let index = 0;
    while (index < bytes.length) {
      const first = bytes[index] ?? 0;
      if (first < 0x80) {
        output += String.fromCharCode(first);
        index += 1;
        continue;
      }
      const width = first >= 0xc2 && first <= 0xdf ? 2 : first >= 0xe0 && first <= 0xef ? 3 : first >= 0xf0 && first <= 0xf4 ? 4 : 0;
      if (width === 0) {
        output += "\uFFFD";
        index += 1;
        continue;
      }
      if (index + width > bytes.length) {
        break;
      }
      const continuation = Array.from(bytes.slice(index + 1, index + width));
      if (continuation.some((value) => value < 0x80 || value > 0xbf)) {
        output += "\uFFFD";
        index += 1;
        continue;
      }
      let codePoint = first & (0x7f >> width);
      for (const value of continuation) {
        codePoint = (codePoint << 6) | (value & 0x3f);
      }
      const minimum = width === 2 ? 0x80 : width === 3 ? 0x800 : 0x10000;
      const invalid = codePoint < minimum || codePoint > 0x10ffff || (codePoint >= 0xd800 && codePoint <= 0xdfff);
      output += invalid ? "\uFFFD" : String.fromCodePoint(codePoint);
      index += width;
    }
    this.pending = bytes.slice(index);
    return output;
  }

  finish(): string {
    if (this.pending.length > 0) {
      this.pending = new Uint8Array(0);
      throw new Error("stream ended with incomplete UTF-8 data");
    }
    return "";
  }
}

function extractJSONDocuments(source: string): { documents: string[]; remainder: string } {
  const documents: string[] = [];
  let startIndex = -1;
  let depth = 0;
  let inString = false;
  let escaped = false;
  let lastConsumedIndex = 0;
  for (let index = 0; index < source.length; index += 1) {
    const character = source[index] ?? "";
    if (startIndex < 0) {
      if (character === "{") {
        startIndex = index;
        depth = 1;
      } else if (/\s/u.test(character)) {
        lastConsumedIndex = index + 1;
      } else {
        throw new Error("stream contains data outside a JSON document");
      }
      continue;
    }
    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (character === "\\") {
        escaped = true;
      } else if (character === '"') {
        inString = false;
      }
      continue;
    }
    if (character === '"') {
      inString = true;
    } else if (character === "{") {
      depth += 1;
    } else if (character === "}") {
      depth -= 1;
      if (depth === 0) {
        documents.push(source.slice(startIndex, index + 1));
        startIndex = -1;
        lastConsumedIndex = index + 1;
      }
    }
  }
  return { documents, remainder: startIndex >= 0 ? source.slice(startIndex) : source.slice(lastConsumedIndex) };
}

export class ChunkedJSONParser {
  private readonly decoder = new IncrementalUtf8Decoder();
  private buffer = "";

  push(chunk: ArrayBuffer | Uint8Array): unknown[] {
    this.buffer += this.decoder.decode(chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk));
    return this.drain();
  }

  finish(): unknown[] {
    this.buffer += this.decoder.finish();
    const events = this.drain();
    if (this.buffer.trim()) {
      throw new Error("stream ended with incomplete JSON data");
    }
    this.buffer = "";
    return events;
  }

  private drain(): unknown[] {
    if (this.buffer.length > maxBufferedCharacters) {
      throw new Error("stream JSON buffer exceeded the 32 MiB safety limit");
    }
    const { documents, remainder } = extractJSONDocuments(this.buffer);
    this.buffer = remainder;
    return documents.map((document) => JSON.parse(document) as unknown);
  }
}
