import type { SpeechCapabilities, SpeechTranscript } from "@deeix/api-contract";
import type { ApiRequest } from "@/platform/transport";

export class SpeechClient {
  constructor(private readonly request: <T>(request: ApiRequest) => Promise<T>) {}

  capabilities(): Promise<SpeechCapabilities> {
    return this.request({ path: "/api/v1/speech/capabilities" });
  }

  transcribe(audio: string): Promise<SpeechTranscript> {
    return this.request({ path: "/api/v1/speech/transcriptions", method: "POST", body: { audio } });
  }
}
